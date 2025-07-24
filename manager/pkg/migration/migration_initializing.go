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
	"k8s.io/client-go/util/retry"
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
	ConditionReasonTokenSecretMissing     = "TokenSecretMissing"
	ConditionReasonResourceInitialized    = "ResourceInitialized"
	ConditionReasonResourceNotInitialized = "ResourceNotInitialized"
	ConditionReasonTimeout                = "Timeout"
)

var migrationStageTimeout = 5 * time.Minute

// Initializing:
//  1. Destination Hub: create managedserviceaccount, Set autoApprove for the SA by initializing event
//  2. Source Hub: attach the klusterletconfig with bootstrapsecret for the migrating clusters by initializing event
//  3. Confirmation: report initialized event by the agent, processed by the manager handler
func (m *ClusterMigrationController) initializing(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
) (bool, error) {
	if !mcm.DeletionTimestamp.IsZero() {
		return false, nil
	}

	// skip if the MigrationResourceInitialized condition is True
	if meta.IsStatusConditionTrue(mcm.Status.Conditions, migrationv1alpha1.ConditionTypeInitialized) ||
		mcm.Status.Phase != migrationv1alpha1.PhaseInitializing {
		return false, nil
	}
	log.Info("migration initializing started")

	condType := migrationv1alpha1.ConditionTypeInitialized
	condStatus := metav1.ConditionTrue
	condReason := ConditionReasonResourceInitialized
	condMessage := "All source and target hubs have been successfully initialized"
	var err error

	defer func() {
		if err != nil {
			condMessage = err.Error()
			condStatus = metav1.ConditionFalse
			condReason = ConditionReasonResourceNotInitialized
		}
		log.Debugf("initializing condition %s(%s): %s", condType, condReason, condMessage)
		e := m.UpdateConditionWithRetry(ctx, mcm, condType, condStatus, condReason, condMessage, migrationStageTimeout)
		if e != nil {
			log.Errorf("failed to update the %s condition: %v", condType, e)
		}
	}()

	sourceHubToClusters, err := getSourceClusters(mcm)
	if err != nil {
		return false, err
	}
	AddSourceClusters(string(mcm.GetUID()), sourceHubToClusters)

	// 1. Create the managedserviceaccount -> generate bootstrap secret
	if err := m.ensureManagedServiceAccount(ctx, mcm); err != nil {
		return false, err
	}
	tokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcm.Name,
			Namespace: mcm.Spec.To, // hub
		},
	}
	err = m.Client.Get(ctx, client.ObjectKeyFromObject(tokenSecret), tokenSecret)
	if err != nil && !apierrors.IsNotFound(err) {
		return false, err
	} else if apierrors.IsNotFound(err) {
		condStatus = metav1.ConditionFalse
		condReason = ConditionReasonTokenSecretMissing
		condMessage = fmt.Sprintf("token secret(%s/%s) is not found!", tokenSecret.Namespace, tokenSecret.Name)
		err = nil // An unready token should not be considered an error
		return true, nil
	}
	// bootstrap secret for source hub
	bootstrapSecret, err := m.generateBootstrapSecret(ctx, mcm)
	if err != nil {
		return false, err
	}

	// 2. Send event to destination hub, TODO: ensure the produce event 'exactly-once'
	// Important: Registration must occur only after autoApprove is successfully set.
	// Thinking - Ensure that autoApprove is properly configured before proceeding.
	if !GetStarted(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseInitializing) {
		if err := m.sendEventToDestinationHub(ctx, mcm, migrationv1alpha1.PhaseInitializing, nil); err != nil {
			return false, err
		}
		log.Infof("sent initializing event to target hub %s", mcm.Spec.To)
		SetStarted(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseInitializing)
	}

	// 3. Send event to Source Hub
	for fromHubName, clusters := range sourceHubToClusters {
		if !GetStarted(string(mcm.GetUID()), fromHubName, migrationv1alpha1.PhaseInitializing) {
			err := m.sendEventToSourceHub(ctx, fromHubName, mcm, migrationv1alpha1.PhaseInitializing, clusters,
				nil, bootstrapSecret)
			if err != nil {
				return false, err
			}
			log.Infof("sent initialing events to source hubs: %s", fromHubName)
			SetStarted(string(mcm.GetUID()), fromHubName, migrationv1alpha1.PhaseInitializing)
		}
	}

	// 4. Check the status from hubs: if the source and target hub are initialized, if not, waiting for the initialization
	errMsg := GetErrorMessage(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseInitializing)
	if errMsg != "" {
		err = fmt.Errorf("initializing target hub %s with err :%s", mcm.Spec.To, errMsg) // the err will be updated into cr
		return true, nil
	}
	for fromHubName := range sourceHubToClusters {
		errMsg := GetErrorMessage(string(mcm.GetUID()), fromHubName, migrationv1alpha1.PhaseInitializing)
		if errMsg != "" {
			err = fmt.Errorf("initializing source hub %s with err :%s", fromHubName, errMsg)
			return true, nil
		}
	}

	if !GetFinished(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseInitializing) {
		condMessage = fmt.Sprintf("The target hub %s is initializing", mcm.Spec.To)
		condStatus = metav1.ConditionFalse
		return true, nil
	}

	for fromHubName := range sourceHubToClusters {
		if !GetFinished(string(mcm.GetUID()), fromHubName, migrationv1alpha1.PhaseInitializing) {
			condMessage = fmt.Sprintf("The source hub %s is initializing", fromHubName)
			condStatus = metav1.ConditionFalse
			return true, nil
		}
	}

	log.Info("migration initializing finished")
	return false, nil
}

// update with conflict error, and also add timeout validating in the conditions
func (m *ClusterMigrationController) UpdateConditionWithRetry(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
	conditionType string,
	status metav1.ConditionStatus,
	reason, message string,
	timeout time.Duration,
) error {
	// The MigrationStarted type should not have timeout
	if conditionType != migrationv1alpha1.ConditionTypeStarted &&
		status == metav1.ConditionFalse && time.Since(mcm.CreationTimestamp.Time) > timeout {
		reason = ConditionReasonTimeout
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := m.Client.Get(ctx, client.ObjectKeyFromObject(mcm), mcm); err != nil {
			return err
		}
		if err := m.UpdateCondition(ctx, mcm, conditionType, status, reason, message); err != nil {
			return err
		}
		return nil
	})
}

func (m *ClusterMigrationController) UpdateCondition(
	ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
	conditionType string,
	status metav1.ConditionStatus,
	reason, message string,
) error {
	newCondition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	}

	update := meta.SetStatusCondition(&mcm.Status.Conditions, newCondition)

	// Handle phase updates based on condition type and status
	if status == metav1.ConditionFalse {
		// migration not start, means the migration is pending
		if conditionType == migrationv1alpha1.ConditionTypeStarted && mcm.Status.Phase != migrationv1alpha1.PhaseCompleted {
			mcm.Status.Phase = migrationv1alpha1.PhasePending
			update = true
		}

		// 1. timeout -> failed
		// 2. validated error, then status switch to failed immediately
		if reason == ConditionReasonTimeout || conditionType == migrationv1alpha1.ConditionTypeValidated {
			mcm.Status.Phase = migrationv1alpha1.PhaseFailed
			update = true
		}
	} else {
		switch conditionType {
		case migrationv1alpha1.ConditionTypeValidated:
			if mcm.Status.Phase != migrationv1alpha1.PhaseCompleted {
				mcm.Status.Phase = migrationv1alpha1.PhaseInitializing
			}
		case migrationv1alpha1.ConditionTypeInitialized:
			if mcm.Status.Phase != migrationv1alpha1.PhaseCompleted {
				mcm.Status.Phase = migrationv1alpha1.PhaseDeploying
			}
		case migrationv1alpha1.ConditionTypeDeployed:
			if mcm.Status.Phase != migrationv1alpha1.PhaseCompleted {
				mcm.Status.Phase = migrationv1alpha1.PhaseRegistering
			}
		case migrationv1alpha1.ConditionTypeRegistered:
			if mcm.Status.Phase != migrationv1alpha1.PhaseCompleted {
				mcm.Status.Phase = migrationv1alpha1.PhaseCleaning
			}
		case migrationv1alpha1.ConditionTypeCleaned:
			if mcm.Status.Phase != migrationv1alpha1.PhaseCompleted && mcm.Status.Phase != migrationv1alpha1.PhaseFailed {
				mcm.Status.Phase = migrationv1alpha1.PhaseCompleted
			}
		}
		update = true
	}

	// Update the resource in Kubernetes only if there was a change
	if update {
		return m.Status().Update(ctx, mcm)
	}
	return nil
}

// sendEventToSourceHub specifies the manager send the message into "From Hub" via spec path(or topic)
// Stage is the expected state for migration, it algin with the condition states
// 1. ResourceInitialized: request sync the klusterletconfig from source hub
// 2. ClusterRegistered: forward bootstrap kubeconfig secret into source hub, to register into the destination hub
// 3. ResourceDeployed: delete the resourcesailed to mark the stage ResourceDeploye from the source hub
// 4. MigrationCompleted: delete the items from the database
func (m *ClusterMigrationController) sendEventToSourceHub(ctx context.Context, fromHub string,
	migration *migrationv1alpha1.ManagedClusterMigration, stage string, managedClusters []string,
	resources []string, bootstrapSecret *corev1.Secret,
) error {
	managedClusterMigrationFromEvent := &migrationbundle.ManagedClusterMigrationFromEvent{
		MigrationId:     string(migration.GetUID()),
		Stage:           stage,
		ToHub:           migration.Spec.To,
		ManagedClusters: managedClusters,
		BootstrapSecret: bootstrapSecret,
		Resources:       resources,
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

// getSourceClusters return a map, key the is the from hub name, and value is the clusters of the hub
func getSourceClusters(mcm *migrationv1alpha1.ManagedClusterMigration) (map[string][]string, error) {
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
