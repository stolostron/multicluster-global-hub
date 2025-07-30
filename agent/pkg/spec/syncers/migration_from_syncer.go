// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package syncers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	cecontext "github.com/cloudevents/sdk-go/v2/context"
	klusterletv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	addonv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const (
	errFailedToSendEvent       = "failed to send event(%s) from %s to %s: %v"
	klusterletConfigNamePrefix = "migration-"
	bootstrapSecretNamePrefix  = "bootstrap-"
	KlusterletConfigAnnotation = "agent.open-cluster-management.io/klusterlet-config"
)

type MigrationSourceSyncer struct {
	client             client.Client
	restConfig         *rest.Config // for init no-cached client of the runtime manager
	transportClient    transport.TransportClient
	transportConfig    *transport.TransportInternalConfig
	bundleVersion      *eventversion.Version
	currentMigrationId string
}

func NewMigrationSourceSyncer(client client.Client, restConfig *rest.Config,
	transportClient transport.TransportClient, transportConfig *transport.TransportInternalConfig,
) *MigrationSourceSyncer {
	return &MigrationSourceSyncer{
		client:          client,
		restConfig:      restConfig,
		transportClient: transportClient,
		transportConfig: transportConfig,
		bundleVersion:   eventversion.NewVersion(),
	}
}

func (s *MigrationSourceSyncer) Sync(ctx context.Context, evt *cloudevents.Event) error {
	payload := evt.Data()
	// handle migration.from cloud event
	migrationSourceHubEvent := &migration.ManagedClusterMigrationFromEvent{}
	if err := json.Unmarshal(payload, migrationSourceHubEvent); err != nil {
		return err
	}
	log.Debugf("received managed cluster migration event %s", string(payload))

	if migrationSourceHubEvent.MigrationId == "" {
		return fmt.Errorf("migrationId is required but not provided in event")
	}

	var err error
	defer func() {
		// don't report the status if migration instance is not matched
		if s.currentMigrationId != migrationSourceHubEvent.MigrationId {
			return
		}
		errMessage := ""
		if err != nil {
			errMessage = err.Error()
		}
		err := ReportMigrationStatus(cecontext.WithTopic(ctx, s.transportConfig.KafkaCredential.StatusTopic),
			s.transportClient,
			&migration.ManagedClusterMigrationBundle{
				MigrationId: migrationSourceHubEvent.MigrationId,
				Stage:       migrationSourceHubEvent.Stage,
				ErrMessage:  errMessage,
			},
			s.bundleVersion)
		if err != nil {
			log.Errorf("failed to send the %s confirmation: %v", migrationSourceHubEvent.Stage, err)
		}
	}()

	if migrationSourceHubEvent.Stage == migrationv1alpha1.PhaseInitializing {
		s.currentMigrationId = migrationSourceHubEvent.MigrationId
		// reset the bundle version for the new migration
		s.bundleVersion.Reset()
		log.Infof("migration initializing started: migrationId=%s, toHub=%s, clusters=%v",
			migrationSourceHubEvent.MigrationId, migrationSourceHubEvent.ToHub, migrationSourceHubEvent.ManagedClusters)
		if err := s.initializing(ctx, migrationSourceHubEvent); err != nil {
			log.Errorf("migration initializing failed: migrationId=%s, error=%v", migrationSourceHubEvent.MigrationId, err)
			return err
		}
		log.Infof("migration initializing completed: migrationId=%s", migrationSourceHubEvent.MigrationId)
	}

	if s.currentMigrationId != migrationSourceHubEvent.MigrationId {
		log.Infof("ignore the received migration event %s, current migrationId is %s", migrationSourceHubEvent.MigrationId,
			s.currentMigrationId)
		return nil
	}

	if migrationSourceHubEvent.Stage == migrationv1alpha1.PhaseDeploying {
		log.Infof("migration deploying started: migrationId=%s, clusters=%v",
			migrationSourceHubEvent.MigrationId, migrationSourceHubEvent.ManagedClusters)
		if err := s.deploying(ctx, migrationSourceHubEvent); err != nil {
			log.Errorf("migration deploying failed: migrationId=%s, error=%v", migrationSourceHubEvent.MigrationId, err)
			return err
		}
		log.Infof("migration deploying completed: migrationId=%s", migrationSourceHubEvent.MigrationId)
	}

	if migrationSourceHubEvent.Stage == migrationv1alpha1.PhaseRegistering {
		log.Infof("migration registering started: migrationId=%s, clusters=%v",
			migrationSourceHubEvent.MigrationId, migrationSourceHubEvent.ManagedClusters)
		if err := s.registering(ctx, migrationSourceHubEvent); err != nil {
			log.Errorf("migration registering failed: migrationId=%s, error=%v", migrationSourceHubEvent.MigrationId, err)
			return err
		}
		log.Infof("migration registering completed: migrationId=%s", migrationSourceHubEvent.MigrationId)
	}

	if migrationSourceHubEvent.Stage == migrationv1alpha1.PhaseCleaning {
		log.Infof("migration cleaning started: migrationId=%s, clusters=%v",
			migrationSourceHubEvent.MigrationId, migrationSourceHubEvent.ManagedClusters)
		if err := s.cleaning(ctx, migrationSourceHubEvent); err != nil {
			log.Errorf("migration cleaning failed: migrationId=%s, error=%v", migrationSourceHubEvent.MigrationId, err)
			return err
		}
		log.Infof("migration cleaning completed: migrationId=%s", migrationSourceHubEvent.MigrationId)
	}

	if migrationSourceHubEvent.Stage == migrationv1alpha1.PhaseRollbacking {
		log.Infof("migration rollbacking: %s - %s", migrationSourceHubEvent.MigrationId,
			migrationSourceHubEvent.RollbackStage)
		err = s.rollbacking(ctx, migrationSourceHubEvent)
		log.Infof("migration rollbacking is finished: %s", migrationSourceHubEvent.MigrationId)
	}
	return err
}

func (m *MigrationSourceSyncer) cleaning(
	ctx context.Context, migratingEvt *migration.ManagedClusterMigrationFromEvent,
) error {
	bootstrapSecret := migratingEvt.BootstrapSecret
	// delete bootstrap kubeconfig secret
	foundBootstrapSecret := &corev1.Secret{}
	if err := m.client.Get(ctx,
		types.NamespacedName{
			Name:      bootstrapSecret.Name,
			Namespace: bootstrapSecret.Namespace,
		}, foundBootstrapSecret); err != nil {
		if apierrors.IsNotFound(err) {
			log.Infof("bootstrap secret %s is removed", bootstrapSecret.GetName())
		} else {
			return err
		}
	} else {
		log.Infof("delete bootstrap secret %s", bootstrapSecret.GetName())
		if err := m.client.Delete(ctx, bootstrapSecret); err != nil {
			return err
		}
	}

	// delete klusterletconfig
	klusterletConfig := &klusterletv1alpha1.KlusterletConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: klusterletConfigNamePrefix + migratingEvt.ToHub,
		},
	}
	if err := m.client.Get(ctx, client.ObjectKeyFromObject(klusterletConfig), klusterletConfig); err != nil {
		if apierrors.IsNotFound(err) {
			log.Infof("klusterletConfig %s is removed", klusterletConfig.GetName())
		} else {
			return err
		}
	} else {
		log.Infof("delete klusterletconfig secret %s", klusterletConfig.GetName())
		if err := m.client.Delete(ctx, klusterletConfig); err != nil {
			return err
		}
	}

	log.Infof("cleaning up clusters %v", migratingEvt.ManagedClusters)
	if err := m.cleaningClusters(ctx, migratingEvt.ManagedClusters, migratingEvt.Stage); err != nil {
		log.Errorf("failed to clean up managed clusters: %v", err)
		return err
	}
	return nil
}

// deploying: send clusters and addon config into target hub
func (s *MigrationSourceSyncer) deploying(
	ctx context.Context, migratingEvt *migration.ManagedClusterMigrationFromEvent,
) error {
	migrationResources := &migration.SourceClusterMigrationResources{
		MigrationId:           migratingEvt.MigrationId,
		ManagedClusters:       []clusterv1.ManagedCluster{},
		KlusterletAddonConfig: []addonv1.KlusterletAddonConfig{},
	}

	// collect clusters and klusterletAddonConfig for migration
	for _, managedCluster := range migratingEvt.ManagedClusters {
		// add cluster
		cluster, err := s.prepareManagedClusterForMigration(ctx, managedCluster)
		if err != nil {
			return fmt.Errorf("failed to prepare managed cluster %s for migration: %w", managedCluster, err)
		}
		migrationResources.ManagedClusters = append(migrationResources.ManagedClusters, *cluster)

		// add addonConfig
		addonConfig, err := s.prepareAddonConfigForMigration(ctx, managedCluster)
		if err != nil {
			return fmt.Errorf("failed to prepare addon config %s for migration: %w", managedCluster, err)
		}
		migrationResources.KlusterletAddonConfig = append(migrationResources.KlusterletAddonConfig, *addonConfig)
	}
	log.Info("deploying: attach clusters and addonConfigs into the event")

	payloadBytes, err := json.Marshal(migrationResources)
	if err != nil {
		return fmt.Errorf("failed to marshal SourceClusterMigrationResources (%v) - %w", migrationResources, err)
	}

	fromHub := configs.GetLeafHubName()
	toHub := migratingEvt.ToHub

	e := utils.ToCloudEvent(constants.MigrationTargetMsgKey, fromHub, toHub, payloadBytes)
	if err := s.transportClient.GetProducer().SendEvent(
		cecontext.WithTopic(ctx, s.transportConfig.KafkaCredential.SpecTopic), e); err != nil {
		return fmt.Errorf(errFailedToSendEvent, constants.MigrationTargetMsgKey, fromHub, toHub, err)
	}
	return nil
}

// initializing: attach klusterletconfig(with bootstrap kubeconfig secret) to managed clusters
// Note: Add the "global-hub.open-cluster-management.io/migrating" to avoid the race condition of the cluster
// reported by both target and source hub
func (m *MigrationSourceSyncer) initializing(
	ctx context.Context, migratingEvt *migration.ManagedClusterMigrationFromEvent,
) error {
	if migratingEvt.BootstrapSecret == nil {
		return fmt.Errorf("bootstrap secret is nil when initializing")
	}
	bootstrapSecret := migratingEvt.BootstrapSecret
	// ensure secret
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		operation, err := controllerutil.CreateOrUpdate(ctx, m.client, bootstrapSecret, func() error { return nil })
		log.Infof("bootstrap secret %s is %s", bootstrapSecret.GetName(), operation)
		return err
	})
	if err != nil {
		return fmt.Errorf("failed to create/update bootstrap secret: %w", err)
	}

	// ensure klusterletconfig
	klusterletConfig, err := generateKlusterletConfig(m.client, migratingEvt.ToHub, bootstrapSecret.Name)
	if err != nil {
		return err
	}
	if err := m.client.Get(ctx, client.ObjectKeyFromObject(klusterletConfig), klusterletConfig); err != nil {
		if apierrors.IsNotFound(err) {
			if err := m.client.Create(ctx, klusterletConfig); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	// update managed cluster annotations to point to the new klusterletconfig
	managedClusters := migratingEvt.ManagedClusters
	for _, managedCluster := range managedClusters {
		mc := &clusterv1.ManagedCluster{}
		if err := m.client.Get(ctx, types.NamespacedName{
			Name: managedCluster,
		}, mc); err != nil {
			return err
		}
		annotations := mc.Annotations
		if annotations == nil {
			annotations = make(map[string]string)
		}

		_, migrating := annotations[constants.ManagedClusterMigrating]
		if migrating && annotations[KlusterletConfigAnnotation] == klusterletConfig.GetName() {
			continue
		}
		if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			operation, err := controllerutil.CreateOrUpdate(ctx, m.client, mc, func() error {
				// Update annotations within the CreateOrUpdate function
				currentAnnotations := mc.GetAnnotations()
				if currentAnnotations == nil {
					currentAnnotations = make(map[string]string)
				}
				currentAnnotations[KlusterletConfigAnnotation] = klusterletConfig.GetName()
				currentAnnotations[constants.ManagedClusterMigrating] = ""
				mc.SetAnnotations(currentAnnotations)
				return nil
			})
			log.Infof("managed clusters %s is %s", mc.GetName(), operation)
			return err
		}); err != nil {
			return err
		}
	}
	return nil
}

// generateKlusterletConfig generate the klusterletconfig for migration
func generateKlusterletConfig(client client.Client, targetHub, bootstrapSecretName string) (
	*unstructured.Unstructured, error,
) {
	mch, err := utils.ListMCH(context.Background(), client)
	if err != nil {
		return nil, err
	}
	if mch == nil {
		return nil, fmt.Errorf("no MCH found")
	}

	klusterletConfig213 := fmt.Sprintf(`
apiVersion: config.open-cluster-management.io/v1alpha1
kind: KlusterletConfig
metadata:
  name: %s
spec:
  bootstrapKubeConfigs:
    type: "LocalSecrets"
    localSecretsConfig:
      kubeConfigSecrets:
      - name: "%s"`, klusterletConfigNamePrefix+targetHub, bootstrapSecretName)

	klusterletConfig214 := fmt.Sprintf(`
apiVersion: config.open-cluster-management.io/v1alpha1
kind: KlusterletConfig
metadata:
  name: %s
spec:
  multipleHubsConfig:
    genBootstrapKubeConfigStrategy: "IncludeCurrentHub"
    bootstrapKubeConfigs:
      type: "LocalSecrets"
      localSecretsConfig:
        kubeConfigSecrets:
        - name: "%s"`, klusterletConfigNamePrefix+targetHub, bootstrapSecretName)

	klusterletConfig := klusterletConfig214
	if strings.Contains(mch.Status.CurrentVersion, "2.13") {
		klusterletConfig = klusterletConfig213
	}

	// Decode YAML into Unstructured object using runtime.Decoder
	dec := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	obj := &unstructured.Unstructured{}
	_, _, err = dec.Decode([]byte(klusterletConfig), nil, obj)
	if err != nil {
		return nil, err
	}

	return obj, err
}

func (m *MigrationSourceSyncer) registering(
	ctx context.Context, migratingEvt *migration.ManagedClusterMigrationFromEvent,
) error {
	managedClusters := migratingEvt.ManagedClusters
	// set the hub accept client into false to trigger the re-registering
	for _, managedCluster := range managedClusters {
		mc := &clusterv1.ManagedCluster{}
		if err := m.client.Get(ctx, types.NamespacedName{
			Name: managedCluster,
		}, mc); err != nil {
			return err
		}
		mc.Spec.HubAcceptsClient = false
		log.Infof("updating managedcluster %s to set HubAcceptsClient as false", mc.Name)
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			return m.client.Update(ctx, mc)
		})
		if err != nil {
			return fmt.Errorf("failed to update managedcluster %s: %w", mc.Name, err)
		}
	}
	return nil
}

func ReportMigrationStatus(
	ctx context.Context,
	transportClient transport.TransportClient,
	migrationBundle *migration.ManagedClusterMigrationBundle,
	version *eventversion.Version,
) error {
	source := configs.GetLeafHubName()
	clusterName := constants.CloudEventGlobalHubClusterName
	payloadBytes, err := json.Marshal(migrationBundle)
	if err != nil {
		return fmt.Errorf("failed to marshal ManagedClusterMigrationBundle %w", err)
	}

	version.Incr()
	eventType := string(enum.ManagedClusterMigrationType)
	e := utils.ToCloudEvent(eventType, source, clusterName, payloadBytes)
	e.SetExtension(eventversion.ExtVersion, version.String())
	if transportClient != nil {
		if err := transportClient.GetProducer().SendEvent(ctx, e); err != nil {
			return fmt.Errorf(errFailedToSendEvent, eventType, source, clusterName, err)
		}
		version.Next()
		return nil
	}
	return errors.New("transport client must not be nil")
}

func SendEvent(
	ctx context.Context,
	transportClient transport.TransportClient,
	eventType string,
	source string,
	clusterName string,
	payloadBytes []byte,
	version *eventversion.Version,
) error {
	version.Incr()
	e := utils.ToCloudEvent(eventType, source, clusterName, payloadBytes)
	e.SetExtension(eventversion.ExtVersion, version.String())
	if transportClient != nil {
		if err := transportClient.GetProducer().SendEvent(ctx, e); err != nil {
			return fmt.Errorf(errFailedToSendEvent, eventType, source, clusterName, err)
		}
		version.Next()
		return nil
	}
	return errors.New("transport client must not be nil")
}

// cleaningClusters handle the following two cases
//  1. stage = failed: remove the added klusterletconfig/migrating, set the hubAccepted with true to rollback
//  2. stage = cleaning: detach the clusters after the migrating finshed
func (s *MigrationSourceSyncer) cleaningClusters(ctx context.Context, managedClusters []string, stage string) error {
	for _, managedCluster := range managedClusters {
		log.Debugf("cleaning up managed cluster %s", managedCluster)
		mc := &clusterv1.ManagedCluster{}
		if err := s.client.Get(ctx, types.NamespacedName{
			Name: managedCluster,
		}, mc); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			} else {
				return err
			}
		}
		if stage == migrationv1alpha1.PhaseCleaning {
			if mc.Spec.HubAcceptsClient {
				continue
			}
			err := s.client.Delete(ctx, mc)
			if err != nil && !apierrors.IsNotFound(err) {
				return err
			}
			continue
		} else {
			if !mc.Spec.HubAcceptsClient {
				mc.Spec.HubAcceptsClient = true
			}
			annotations := mc.GetAnnotations()
			if annotations != nil {
				delete(annotations, KlusterletConfigAnnotation)
				delete(annotations, constants.ManagedClusterMigrating)
				mc.SetAnnotations(annotations)
				if err := s.client.Update(ctx, mc); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// prepareManagedClusterForMigration prepares a managed cluster for migration by cleaning metadata
func (s *MigrationSourceSyncer) prepareManagedClusterForMigration(ctx context.Context, clusterName string) (
	*clusterv1.ManagedCluster, error,
) {
	cluster := &clusterv1.ManagedCluster{}
	if err := s.client.Get(ctx, types.NamespacedName{Name: clusterName}, cluster); err != nil {
		return nil, fmt.Errorf("failed to get managed cluster %s: %w", clusterName, err)
	}

	// Clean metadata for migration
	s.cleanObjectMetadata(cluster)
	cluster.Spec.ManagedClusterClientConfigs = nil
	cluster.Status = clusterv1.ManagedClusterStatus{}

	// remove migrating and klusterletconfig annotations from managedcluster
	annotations := cluster.GetAnnotations()
	if annotations != nil {
		delete(annotations, constants.ManagedClusterMigrating)
		delete(annotations, KlusterletConfigAnnotation)
		cluster.SetAnnotations(annotations)
	}

	return cluster, nil
}

// prepareAddonConfigForMigration prepares addon config for migration by cleaning metadata
func (s *MigrationSourceSyncer) prepareAddonConfigForMigration(ctx context.Context, clusterName string) (
	*addonv1.KlusterletAddonConfig, error,
) {
	addonConfig := &addonv1.KlusterletAddonConfig{}
	if err := s.client.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: clusterName}, addonConfig); err != nil {
		return nil, fmt.Errorf("failed to get addon config %s: %w", clusterName, err)
	}

	// Clean metadata for migration
	s.cleanObjectMetadata(addonConfig)
	addonConfig.Status = addonv1.KlusterletAddonConfigStatus{}

	return addonConfig, nil
}

// cleanObjectMetadata removes metadata fields that should not be migrated
func (s *MigrationSourceSyncer) cleanObjectMetadata(obj client.Object) {
	obj.SetManagedFields(nil)
	obj.SetFinalizers(nil)
	obj.SetOwnerReferences(nil)
	obj.SetSelfLink("")
	obj.SetResourceVersion("")
	obj.SetGeneration(0)
}

// rollbacking handles rollback operations for different stages
// Based on RollbackStage field, it performs appropriate cleanup actions
func (s *MigrationSourceSyncer) rollbacking(ctx context.Context, migrationSourceHubEvent 
	*migration.ManagedClusterMigrationFromEvent) error {
	log.Infof("performing rollback for stage: %s", migrationSourceHubEvent.RollbackStage)

	var err error
	var reportMessage string

	switch migrationSourceHubEvent.RollbackStage {
	case migrationv1alpha1.PhaseInitializing:
		err = s.rollbackInitializing(ctx, migrationSourceHubEvent)
		if err != nil {
			reportMessage = fmt.Sprintf("Initializing rollback on source hub completed with issues: %v", err)
		} else {
			reportMessage = "Migration annotations successfully removed from all managed clusters on source hub"
		}
	case migrationv1alpha1.PhaseDeploying:
		err = s.rollbackDeploying(ctx, migrationSourceHubEvent)
		if err != nil {
			reportMessage = fmt.Sprintf("Deploying rollback on source hub completed with issues: %v", err)
		} else {
			reportMessage = "Migration annotations successfully removed from all managed clusters on source hub"
		}
	case migrationv1alpha1.PhaseRegistering:
		err = s.rollbackRegistering(ctx, migrationSourceHubEvent)
		if err != nil {
			reportMessage = fmt.Sprintf("Registering rollback on source hub completed with issues: %v", err)
		} else {
			reportMessage = "Migration annotations successfully removed from all managed clusters on source hub"
		}
	default:
		log.Infof("no specific rollback action needed for stage: %s", migrationSourceHubEvent.RollbackStage)
		reportMessage = fmt.Sprintf("No specific rollback action needed for stage: %s",
			migrationSourceHubEvent.RollbackStage)
	}

	// Report rollback status to global hub, including detailed message
	reportErr := ReportMigrationStatus(cecontext.WithTopic(ctx, s.transportConfig.KafkaCredential.StatusTopic),
		s.transportClient,
		&migration.ManagedClusterMigrationBundle{
			MigrationId: migrationSourceHubEvent.MigrationId,
			Stage:       migrationv1alpha1.ConditionTypeRolledBack,
			ErrMessage:  reportMessage, // Use detailed message instead of just error
		},
		s.bundleVersion)

	if reportErr != nil {
		log.Errorf("failed to report rollback status: %v", reportErr)
	}

	// Return the original error if any
	return err
}

// rollbackInitializing removes migration-related annotations from managed clusters
// This is used when initializing phase fails
func (s *MigrationSourceSyncer) rollbackInitializing(ctx context.Context, migrationSourceHubEvent *migration.ManagedClusterMigrationFromEvent) error {
	var successfulClusters []string
	var failedClusters []string
	var errorMessages []string

	for _, managedCluster := range migrationSourceHubEvent.ManagedClusters {
		log.Infof("cleaning up annotations for managed cluster: %s", managedCluster)

		mc := &clusterv1.ManagedCluster{}
		err := s.client.Get(ctx, types.NamespacedName{Name: managedCluster}, mc)
		if err != nil {
			if apierrors.IsNotFound(err) {
				log.Infof("managed cluster %s not found, skipping annotation cleanup", managedCluster)
				continue
			}
			failedClusters = append(failedClusters, managedCluster)
			errorMessages = append(errorMessages, fmt.Sprintf("failed to get managed cluster %s: %v", managedCluster, err))
			continue
		}

		annotations := mc.GetAnnotations()
		if annotations == nil {
			log.Infof("no annotations found on managed cluster %s, skipping cleanup", managedCluster)
			continue
		}

		// Check if migration annotations exist
		_, hasMigrating := annotations[constants.ManagedClusterMigrating]
		_, hasKlusterletConfig := annotations[KlusterletConfigAnnotation]

		if !hasMigrating && !hasKlusterletConfig {
			log.Infof("no migration annotations found on managed cluster %s, skipping cleanup", managedCluster)
			continue
		}

		// Remove migration-related annotations
		delete(annotations, constants.ManagedClusterMigrating)
		delete(annotations, KlusterletConfigAnnotation)
		mc.SetAnnotations(annotations)

		if err := s.client.Update(ctx, mc); err != nil {
			failedClusters = append(failedClusters, managedCluster)
			errorMessages = append(errorMessages, fmt.Sprintf("failed to remove migration annotations from managed cluster %s: %v", managedCluster, err))
			continue
		}

		successfulClusters = append(successfulClusters, managedCluster)
		log.Infof("successfully removed migration annotations from managed cluster: %s", managedCluster)
	}

	// Prepare detailed result message
	if len(errorMessages) > 0 {
		var resultMessage strings.Builder
		if len(successfulClusters) > 0 {
			resultMessage.WriteString(fmt.Sprintf("Migration annotations successfully removed from clusters: %v. ", successfulClusters))
		}
		resultMessage.WriteString(fmt.Sprintf("Failed to remove annotations from clusters: %v. Errors: %v", failedClusters, strings.Join(errorMessages, "; ")))
		return errors.New(resultMessage.String())
	}

	return nil
}

// rollbackDeploying handles rollback operations for deploying stage
func (s *MigrationSourceSyncer) rollbackDeploying(ctx context.Context, migrationSourceHubEvent *migration.ManagedClusterMigrationFromEvent) error {
	log.Infof("rollback deploying stage for clusters: %v", migrationSourceHubEvent.ManagedClusters)

	// For deploying stage rollback, we need to:
	// 1. Clean up migration annotations from managed clusters on source hub
	// 2. The target hub will handle removing the deployed addonConfig and clusters

	// Clean up annotations on source hub - use the enhanced error handling
	err := s.rollbackInitializing(ctx, migrationSourceHubEvent)
	if err != nil {
		// Return error with deploying stage context
		return fmt.Errorf("deploying stage rollback failed: %v", err)
	}
	return nil
}

// rollbackRegistering handles rollback operations for registering stage
func (s *MigrationSourceSyncer) rollbackRegistering(ctx context.Context, migrationSourceHubEvent *migration.ManagedClusterMigrationFromEvent) error {
	log.Infof("rollback registering stage for clusters: %v", migrationSourceHubEvent.ManagedClusters)

	// For registering stage rollback, we may need to:
	// 1. Restore original cluster registration configuration
	// 2. Remove bootstrap secrets
	// 3. Clean up migration annotations

	// For now, clean up annotations as the main rollback action
	err := s.rollbackInitializing(ctx, migrationSourceHubEvent)
	if err != nil {
		// Return error with registering stage context
		return fmt.Errorf("registering stage rollback failed: %v", err)
	}
	return nil
}
