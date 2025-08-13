package migration

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"open-cluster-management.io/managed-serviceaccount/apis/authentication/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	migrationbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const (
	ConditionReasonResourceInitialized = "ResourceInitialized"
	ConditionReasonError               = "Error"
	ConditionReasonTimeout             = "Timeout"
	ManagedServiceAddonName            = "managed-serviceaccount"
	DefaultAddOnInstallationNamespace  = "open-cluster-management-agent-addon"
)

var (
	migrationStageTimeout = 5 * time.Minute  // Default timeout for most migration stages
	registeringTimeout    = 12 * time.Minute // Extended timeout for registering phase
)

// Initializing:
//  1. Destination Hub: create managedserviceaccount, Set autoApprove for the SA by initializing event
//  2. Source Hub: attach the klusterletconfig with bootstrapsecret for the migrating clusters by initializing event
//  3. Confirmation: report initialized event by the agent, processed by the manager handler
func (m *ClusterMigrationController) initializing(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
) (bool, error) {
	if mcm.DeletionTimestamp != nil {
		return false, nil
	}

	if meta.IsStatusConditionTrue(mcm.Status.Conditions, migrationv1alpha1.ConditionTypeInitialized) ||
		mcm.Status.Phase != migrationv1alpha1.PhaseInitializing {
		return false, nil
	}
	log.Info("migration initializing started")

	condition := metav1.Condition{
		Type:    migrationv1alpha1.ConditionTypeInitialized,
		Status:  metav1.ConditionFalse,
		Reason:  ConditionReasonWaiting,
		Message: "Waiting for the source and target hubs to be initialized",
	}
	nextPhase := migrationv1alpha1.PhaseInitializing

	defer m.handleStatusWithRollback(ctx, mcm, &condition, &nextPhase, migrationStageTimeout)

	// check the source hub to see is there any error message reported
	sourceHubToClusters := GetSourceClusters(string(mcm.GetUID()))
	if sourceHubToClusters == nil {
		sourceHubToClustersFromDB, err := getSourceHubToClustersFromDB(mcm)
		if err != nil {
			condition.Message = err.Error()
			condition.Reason = ConditionReasonError
			return false, err
		}
		if len(sourceHubToClustersFromDB) == 0 {
			condition.Message = fmt.Sprintf("not found the clusters %s", mcm.Spec.IncludedManagedClusters)
			condition.Reason = ConditionReasonError
			return false, nil
		}
		sourceHubToClusters = sourceHubToClustersFromDB
		AddSourceClusters(string(mcm.GetUID()), sourceHubToClustersFromDB)
	}

	// 1. Create the managedserviceaccount -> generate bootstrap secret
	if err := m.ensureManagedServiceAccount(ctx, mcm); err != nil {
		return false, err
	}
	token := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: mcm.Name, Namespace: mcm.Spec.To}}
	err := m.Client.Get(ctx, client.ObjectKeyFromObject(token), token)
	if err != nil && !apierrors.IsNotFound(err) {
		condition.Message = err.Error()
		condition.Reason = ConditionReasonError
		return false, err
	} else if apierrors.IsNotFound(err) {
		condition.Message = fmt.Sprintf("waiting for token secret (%s/%s) to be created", token.Namespace, token.Name)
		log.Info(condition.Message)
		return true, nil
	}
	// bootstrap secret for source hub
	bootstrapSecret, err := m.generateBootstrapSecret(ctx, mcm)
	if err != nil {
		condition.Message = err.Error()
		condition.Reason = ConditionReasonError
		return false, err
	}

	// 2. Send event to destination hub, TODO: ensure the produce event 'exactly-once'
	// Important: Registration must occur only after autoApprove is successfully set.
	// Thinking - Ensure that autoApprove is properly configured before proceeding.
	if !GetStarted(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseInitializing) {
		if err := m.sendEventToTargetHub(ctx, mcm, migrationv1alpha1.PhaseInitializing, nil, ""); err != nil {
			condition.Message = err.Error()
			condition.Reason = ConditionReasonError
			return false, err
		}
		log.Infof("sent initializing event to target hub %s", mcm.Spec.To)
		SetStarted(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseInitializing)
	}

	// 3. Send event to Source Hub
	for fromHubName, clusters := range sourceHubToClusters {
		if !GetStarted(string(mcm.GetUID()), fromHubName, migrationv1alpha1.PhaseInitializing) {
			err := m.sendEventToSourceHub(ctx, fromHubName, mcm, migrationv1alpha1.PhaseInitializing, clusters,
				bootstrapSecret, "")
			if err != nil {
				condition.Message = err.Error()
				condition.Reason = ConditionReasonError
				return false, err
			}
			log.Infof("sent initialing events to source hubs: %s", fromHubName)
			SetStarted(string(mcm.GetUID()), fromHubName, migrationv1alpha1.PhaseInitializing)
		}
	}

	// 4. Check the status from hubs: if the source and target hub are initialized, if not, waiting for the initialization
	if errMsg := GetErrorMessage(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseInitializing); errMsg != "" {
		condition.Message = fmt.Sprintf("initializing target hub %s with err :%s", mcm.Spec.To, errMsg)
		condition.Reason = ConditionReasonError
		return false, nil
	}
	for fromHubName := range sourceHubToClusters {
		if errMsg := GetErrorMessage(string(mcm.GetUID()), fromHubName, migrationv1alpha1.PhaseInitializing); errMsg != "" {
			condition.Message = fmt.Sprintf("initializing source hub %s with err :%s", fromHubName, errMsg)
			condition.Reason = ConditionReasonError
			return false, nil
		}
	}

	if !GetFinished(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseInitializing) {
		condition.Message = fmt.Sprintf("waiting for target hub %s to complete initialization", mcm.Spec.To)
		return true, nil
	}

	for fromHubName := range sourceHubToClusters {
		if !GetFinished(string(mcm.GetUID()), fromHubName, migrationv1alpha1.PhaseInitializing) {
			condition.Message = fmt.Sprintf("waiting for source hub %s to complete initialization", fromHubName)
			return true, nil
		}
	}

	condition.Status = metav1.ConditionTrue
	condition.Reason = ConditionReasonResourceInitialized
	condition.Message = "All source and target hubs have been successfully initialized"
	nextPhase = migrationv1alpha1.PhaseDeploying

	log.Info("migration initializing finished")
	return false, nil
}

// handleStatusWithRollback updates the condition and phase, transitioning to Rollbacking for most phases
// Note: Cleaning phase has its own handleCleaningStatus function and should not use this function
func (m *ClusterMigrationController) handleStatusWithRollback(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
	condition *metav1.Condition,
	nextPhase *string,
	stageTimeout time.Duration,
) {
	// Cleaning phase should use handleCleaningStatus instead
	if condition.Type == migrationv1alpha1.ConditionTypeCleaned {
		log.Errorf("handleStatusWithRollback should not be called for cleaning phase, use handleCleaningStatus instead")
		return
	}

	startedCond := meta.FindStatusCondition(mcm.Status.Conditions, migrationv1alpha1.ConditionTypeStarted)
	if condition.Reason == ConditionReasonWaiting && startedCond != nil &&
		time.Since(startedCond.LastTransitionTime.Time) > stageTimeout {
		condition.Reason = ConditionReasonTimeout
		condition.Message = fmt.Sprintf("[Timeout] %s", condition.Message)
		*nextPhase = migrationv1alpha1.PhaseRollbacking
	}

	if condition.Reason == ConditionReasonError {
		*nextPhase = migrationv1alpha1.PhaseRollbacking
	}

	err := m.UpdateStatusWithRetry(ctx, mcm, *condition, *nextPhase)
	if err != nil {
		log.Errorf("failed to update the %s condition: %v", condition.Type, err)
	}
}

// sendEventToSourceHub specifies the manager send the message into "From Hub" via spec path(or topic)
// Stage is the expected state for migration, it algin with the condition states
// 1. ResourceInitialized: request sync the klusterletconfig from source hub
// 2. ClusterRegistered: forward bootstrap kubeconfig secret into source hub, to register into the destination hub
// 3. ResourceDeployed: delete the resourcesailed to mark the stage ResourceDeploye from the source hub
// 4. MigrationCompleted: delete the items from the database
func (m *ClusterMigrationController) sendEventToSourceHub(ctx context.Context, fromHub string,
	migration *migrationv1alpha1.ManagedClusterMigration, stage string, managedClusters []string,
	bootstrapSecret *corev1.Secret, rollbackStage string,
) error {
	managedClusterMigrationFromEvent := &migrationbundle.MigrationSourceBundle{
		MigrationId:     string(migration.GetUID()),
		Stage:           stage,
		ToHub:           migration.Spec.To,
		ManagedClusters: managedClusters,
		BootstrapSecret: bootstrapSecret,
		RollbackStage:   rollbackStage,
	}

	payloadBytes, err := json.Marshal(managedClusterMigrationFromEvent)
	if err != nil {
		return fmt.Errorf("failed to marshal managed cluster migration from event(%v) - %w",
			managedClusterMigrationFromEvent, err)
	}

	eventType := constants.MigrationSourceMsgKey
	evt := utils.ToCloudEvent(eventType, constants.CloudEventGlobalHubClusterName, fromHub, payloadBytes)
	if err := m.Producer.SendEvent(ctx, evt); err != nil {
		return fmt.Errorf("failed to sync managedclustermigration event(%s) from source(%s) to destination(%s) - %w",
			eventType, constants.CloudEventGlobalHubClusterName, fromHub, err)
	}
	return nil
}

// getSourceHubToClustersFromDB return a map, key the is the from hub name, and value is the clusters of the hub
func getSourceHubToClustersFromDB(mcm *migrationv1alpha1.ManagedClusterMigration) (map[string][]string, error) {
	db := database.GetGorm()
	managedClusterMap := make(map[string][]string)
	if mcm.Spec.From != "" {
		managedClusterMap[mcm.Spec.From] = mcm.Spec.IncludedManagedClusters
		return managedClusterMap, nil
	}

	rows, err := db.Raw(`SELECT leaf_hub_name, cluster_name FROM status.managed_clusters
			WHERE cluster_name IN (?)`,
		mcm.Spec.IncludedManagedClusters).Rows()
	if err != nil {
		return nil, fmt.Errorf("failed to get leaf hub name and managed clusters - %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var leafHubName, managedClusterName string
		if err := rows.Scan(&leafHubName, &managedClusterName); err != nil {
			return nil, fmt.Errorf("failed to scan leaf hub name and managed cluster name - %w", err)
		}
		// If the cluster is synced into the to hub, ignore it.
		if leafHubName == mcm.Spec.To {
			continue
		}
		managedClusterMap[leafHubName] = append(managedClusterMap[leafHubName], managedClusterName)
	}
	return managedClusterMap, nil
}

func (m *ClusterMigrationController) ensureManagedServiceAccount(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
) error {
	// create a desired msa
	desiredMSA := &v1beta1.ManagedServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcm.GetName(),
			Namespace: mcm.Spec.To,
			Labels: map[string]string{
				"owner": strings.ToLower(constants.ManagedClusterMigrationKind),
			},
		},
		Spec: v1beta1.ManagedServiceAccountSpec{
			Rotation: v1beta1.ManagedServiceAccountRotation{
				Enabled: true,
				Validity: metav1.Duration{
					Duration: 86400 * time.Hour,
				},
			},
		},
	}

	existingMSA := &v1beta1.ManagedServiceAccount{}
	err := m.Client.Get(ctx, types.NamespacedName{
		Name:      mcm.GetName(),
		Namespace: mcm.Spec.To,
	}, existingMSA)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return m.Client.Create(ctx, desiredMSA)
		}
		return err
	}

	return nil
}

// https://github.com/open-cluster-management-io/ocm/blob/main/pkg/registration/spoke/addon/configuration.go#L63
func (m *ClusterMigrationController) getManagedServiceAccountAddonInstallNamespace(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
) (string, error) {
	addOn := addonapiv1alpha1.ManagedClusterAddOn{ObjectMeta: metav1.ObjectMeta{
		Name:      "managed-serviceaccount",
		Namespace: mcm.Spec.To, // target hub
	}}
	err := m.Client.Get(ctx, client.ObjectKeyFromObject(&addOn), &addOn)
	if err != nil {
		return "", err
	}

	installationNamespace := addOn.Status.Namespace
	if installationNamespace == "" {
		installationNamespace = addOn.Spec.InstallNamespace
	}

	msaInstallNamespaceAnnotation := "global-hub.open-cluster-management.io/managed-serviceaccount-install-namespace"
	// if user specifies the managedserviceaccount addon namespace, then use it
	if val, ok := mcm.Annotations[msaInstallNamespaceAnnotation]; ok {
		installationNamespace = val
	}

	if installationNamespace == "" {
		installationNamespace = DefaultAddOnInstallationNamespace
	}
	return installationNamespace, nil
}

func (m *ClusterMigrationController) generateBootstrapSecret(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
) (*corev1.Secret, error) {
	toHubCluster := mcm.Spec.To
	// get the secret which is generated by msa
	desiredSecret := &corev1.Secret{}
	if err := m.Client.Get(ctx, types.NamespacedName{
		Name:      mcm.GetName(),
		Namespace: toHubCluster,
	}, desiredSecret); err != nil {
		return nil, err
	}
	// fetch the managed cluster to get url
	managedCluster := &clusterv1.ManagedCluster{}
	if err := m.Client.Get(ctx, types.NamespacedName{
		Name: toHubCluster,
	}, managedCluster); err != nil {
		return nil, err
	}

	if len(managedCluster.Spec.ManagedClusterClientConfigs) == 0 {
		return nil, fmt.Errorf("no ManagedClusterClientConfigs found for hub %s", toHubCluster)
	}

	config := clientcmdapi.NewConfig()
	config.Clusters[toHubCluster] = &clientcmdapi.Cluster{
		Server:                   managedCluster.Spec.ManagedClusterClientConfigs[0].URL,
		CertificateAuthorityData: desiredSecret.Data["ca.crt"],
	}
	config.AuthInfos["user"] = &clientcmdapi.AuthInfo{
		Token: string(desiredSecret.Data["token"]),
	}
	config.Contexts["default-context"] = &clientcmdapi.Context{
		Cluster:  toHubCluster,
		AuthInfo: "user",
	}
	config.CurrentContext = "default-context"

	// load it into secret
	kubeconfigBytes, err := clientcmd.Write(*config)
	if err != nil {
		return nil, err
	}
	bootstrapSecret := getBootstrapSecret(toHubCluster, kubeconfigBytes)
	return bootstrapSecret, nil
}

func getBootstrapSecret(sourceHubCluster string, kubeconfigBytes []byte) *corev1.Secret {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      bootstrapSecretNamePrefix + sourceHubCluster,
			Namespace: "multicluster-engine",
		},
		Data: map[string][]byte{
			"kubeconfig": kubeconfigBytes,
		},
	}
	if kubeconfigBytes != nil {
		secret.Data = map[string][]byte{
			"kubeconfig": kubeconfigBytes,
		}
	}
	return secret
}
