package migration

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	addonv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"open-cluster-management.io/managed-serviceaccount/apis/authentication/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	migrationbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/config"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const (
	ConditionReasonTokenSecretMissing = "TokenSecretMissing"
	ConditionReasonClusterNotSynced   = "ClusterNotSynced"
	ConditionReasonResourcePrepared   = "ResourcePrepared"
	ConditionReasonTimeout            = "Timeout"
)

var (
	migrationStageTimeout    = 5 * time.Minute
	sendInitToSourceHub      = false
	sendInitToDestinationHub = false
)

// Initializing:
//  1. Grant the proper permission to the source clusters and the target cluster for the migration topic.
//  2. Send the event to the source clusters and the target cluster to create producer and consumer
//     for the migration topic.
//  3. Destination Hub: create managedserviceaccount, Set autoApprove for the SA
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
	log.Info("migration initializing")

	condType := migrationv1alpha1.ConditionTypeInitialized
	condStatus := metav1.ConditionTrue
	condReason := ConditionReasonResourcePrepared
	condMessage := "All required resources initialized"
	var err error

	defer func() {
		if err != nil {
			condMessage = err.Error()
			condStatus = metav1.ConditionFalse
		}
		log.Infof("initializing condition %s(%s): %s", condType, condReason, condMessage)
		e := m.UpdateConditionWithRetry(ctx, mcm, condType, condStatus, condReason, condMessage)
		if e != nil {
			log.Errorf("failed to update the %s condition: %v", condType, e)
		}
	}()

	// To Hub
	// check if the secret is created by managedserviceaccount, if not, ensure the managedserviceaccount
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

	// set destination hub to autoApprove for the sa
	// Important: Registration must occur only after autoApprove is successfully set.
	// Thinking - Ensure that autoApprove is properly configured before proceeding.
	if !sendInitToDestinationHub {
		if err := m.sendEventToDestinationHub(ctx, mcm, migrationv1alpha1.ConditionTypeInitialized, nil); err != nil {
			return false, err
		}
		sendInitToDestinationHub = true
	}

	log.Info("migration bootstrap kubeconfig secret token is ready")

	// check the migration clusters in the databases, if not, send it again
	sourceHubToClusters, err := getSourceClusters(mcm)
	if err != nil {
		return false, err
	}

	// ensure grant the proper permission to the KafkaUser for the gh-migration topic
	if err := m.ensurePermission(ctx, sourceHubToClusters, mcm.Spec.To); err != nil {
		log.Errorf("failed to grant permission to the kafkauser due to %v", err)
		return false, err
	}

	// From Hub
	// send the migration event to migration.from managed hub(s)
	initializingClusters := []string{}
	db := database.GetGorm()
	for fromHubName, clusters := range sourceHubToClusters {
		clusterResourcesSynced := true
		var initialized []models.ManagedClusterMigration
		if err = db.Where("from_hub = ? AND to_hub = ?", fromHubName, mcm.Spec.To).Find(&initialized).Error; err != nil {
			return false, err
		}
		initializedClusters := make([]string, len(initialized))
		for i, cluster := range initialized {
			initializedClusters[i] = cluster.ClusterName
		}
		// assert whether synced, if synced, change the cluster status into migrating
		for _, cluster := range clusters {
			if !utils.ContainsString(initializedClusters, cluster) {
				initializingClusters = append(initializingClusters, cluster)
				clusterResourcesSynced = false
			}
		}
		// confirmation in status
		if !clusterResourcesSynced && !sendInitToSourceHub {
			err := m.sendEventToSourceHub(ctx, fromHubName, mcm.Spec.To,
				migrationv1alpha1.ConditionTypeInitialized, clusters, nil)
			if err != nil {
				return false, err
			}
			sendInitToSourceHub = true
		}
	}

	if len(initializingClusters) > 0 {
		condStatus = metav1.ConditionFalse
		condReason = ConditionReasonClusterNotSynced
		condMessage = fmt.Sprintf("cluster addonConfigs %v are not synced to the database", initializingClusters)
		return true, nil
	}

	// finished initializing
	sendInitToDestinationHub = false
	sendInitToSourceHub = false

	log.Info("migration klusterletconfigs have been synced into the database")
	return false, nil
}

func (m *ClusterMigrationController) ensurePermission(ctx context.Context,
	sourceHubToClusters map[string][]string, toHub string,
) error {
	for fromHubName := range sourceHubToClusters {
		// Get the KafkaUser
		kafkaUser := &kafkav1beta2.KafkaUser{}
		err := m.Client.Get(ctx, types.NamespacedName{
			Name:      config.GetKafkaUserName(fromHubName),
			Namespace: utils.GetDefaultNamespace(),
		}, kafkaUser)
		if err != nil {
			return err
		}

		migrationACL := utils.WriteTopicACL(m.migrationTopic)
		if err := m.updateKafkaUser(ctx, kafkaUser, migrationACL); err != nil {
			return err
		}
	}

	// Get the KafkaUser
	kafkaUser := &kafkav1beta2.KafkaUser{}
	err := m.Client.Get(ctx, types.NamespacedName{
		Name:      config.GetKafkaUserName(toHub),
		Namespace: utils.GetDefaultNamespace(),
	}, kafkaUser)
	if err != nil {
		return err
	}
	migrationACL := utils.ReadTopicACL(m.migrationTopic, false)
	if err := m.updateKafkaUser(ctx, kafkaUser, migrationACL); err != nil {
		return err
	}
	return nil
}

func (m *ClusterMigrationController) updateKafkaUser(ctx context.Context, kafkaUser *kafkav1beta2.KafkaUser,
	migrationACL kafkav1beta2.KafkaUserSpecAuthorizationAclsElem,
) error {
	found := false
	for _, existingAcl := range kafkaUser.Spec.Authorization.Acls {
		if utils.GenerateACLKey(existingAcl) == utils.GenerateACLKey(migrationACL) {
			found = true
			break
		}
	}
	if !found {
		// Patch the gh-migration Permission
		kafkaUser.Spec.Authorization.Acls = append(kafkaUser.Spec.Authorization.Acls,
			[]kafkav1beta2.KafkaUserSpecAuthorizationAclsElem{
				migrationACL,
			}...,
		)
		if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			if err := m.Client.Update(ctx, kafkaUser); err != nil {
				return err
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

// update with conflict error, and also add timeout validating in the conditions
func (m *ClusterMigrationController) UpdateConditionWithRetry(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
	conditionType string,
	status metav1.ConditionStatus,
	reason, message string,
) error {
	// timeout
	if status == metav1.ConditionFalse && time.Since(mcm.CreationTimestamp.Time) > migrationStageTimeout {
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

// sendEventToDestinationHub:
// 1. only send the msa info to allow auto approve if KlusterletAddonConfig is nil -> registering
// 2. if KlusterletAddonConfig is not nil -> deploying
func (m *ClusterMigrationController) sendEventToDestinationHub(ctx context.Context,
	migration *migrationv1alpha1.ManagedClusterMigration, stage string, addonConfig *addonv1.KlusterletAddonConfig,
) error {
	// default managedserviceaccount addon namespace
	msaNamespace := "open-cluster-management-agent-addon"
	if m.importClusterInHosted {
		// hosted mode, the  managedserviceaccount addon namespace
		msaNamespace = "open-cluster-management-global-hub-agent-addon"
	}
	msaInstallNamespaceAnnotation := "global-hub.open-cluster-management.io/managed-serviceaccount-install-namespace"
	// if user specifies the managedserviceaccount addon namespace, then use it
	if val, ok := migration.Annotations[msaInstallNamespaceAnnotation]; ok {
		msaNamespace = val
	}
	managedClusterMigrationToEvent := &migrationbundle.ManagedClusterMigrationToEvent{
		Stage:                                 stage,
		ManagedServiceAccountName:             migration.Name,
		ManagedServiceAccountInstallNamespace: msaNamespace,
	}
	if addonConfig != nil {
		managedClusterMigrationToEvent.KlusterletAddonConfig = addonConfig
	}

	payloadToBytes, err := json.Marshal(managedClusterMigrationToEvent)
	if err != nil {
		return fmt.Errorf("failed to marshal managed cluster migration to event(%v) - %w",
			managedClusterMigrationToEvent, err)
	}

	eventType := constants.CloudEventTypeMigrationTo
	evt := utils.ToCloudEvent(eventType, constants.CloudEventGlobalHubClusterName, migration.Spec.To, payloadToBytes)
	if err := m.Producer.SendEvent(ctx, evt); err != nil {
		return fmt.Errorf("failed to sync managedclustermigration event(%s) from source(%s) to destination(%s) - %w",
			eventType, constants.CloudEventGlobalHubClusterName, migration.Spec.To, err)
	}
	return nil
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
		if reason == ConditionReasonTimeout && mcm.Status.Phase != migrationv1alpha1.PhaseFailed {
			mcm.Status.Phase = migrationv1alpha1.PhaseFailed
			update = true
		}
	} else {
		switch conditionType {
		case migrationv1alpha1.ConditionTypeValidated:
			if mcm.Status.Phase != migrationv1alpha1.PhaseCompleted {
				mcm.Status.Phase = migrationv1alpha1.PhaseInitializing
			}
		case migrationv1alpha1.ConditionTypeInitialized, migrationv1alpha1.ConditionTypeRegistered:
			if mcm.Status.Phase != migrationv1alpha1.PhaseMigrating {
				mcm.Status.Phase = migrationv1alpha1.PhaseMigrating
			}
		case migrationv1alpha1.ConditionTypeDeployed:
			if mcm.Status.Phase != migrationv1alpha1.PhaseCleaning {
				mcm.Status.Phase = migrationv1alpha1.PhaseCleaning
			}
		case migrationv1alpha1.ConditionTypeCleaned:
			if mcm.Status.Phase != migrationv1alpha1.PhaseCompleted {
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
func (m *ClusterMigrationController) sendEventToSourceHub(ctx context.Context, fromHub string, toHub string,
	stage string, managedClusters []string, bootstrapSecret *corev1.Secret,
) error {
	managedClusterMigrationFromEvent := &migrationbundle.ManagedClusterMigrationFromEvent{
		Stage:           stage,
		ToHub:           toHub,
		ManagedClusters: managedClusters,
		BootstrapSecret: bootstrapSecret,
	}

	payloadBytes, err := json.Marshal(managedClusterMigrationFromEvent)
	if err != nil {
		return fmt.Errorf("failed to marshal managed cluster migration from event(%v) - %w",
			managedClusterMigrationFromEvent, err)
	}

	eventType := constants.CloudEventTypeMigrationFrom
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
