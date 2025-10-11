// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package migration

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
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
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
	// Error message templates
	errFailedToSendEvent = "failed to send event(%s) from %s to %s: %v"

	// Resource naming
	klusterletConfigNamePrefix = "migration-"
	bootstrapSecretNamePrefix  = "bootstrap-"

	// Annotations
	KlusterletConfigAnnotation = "agent.open-cluster-management.io/klusterlet-config"
	kubectlConfigAnnotation    = "kubectl.kubernetes.io/last-applied-configuration"
)

type MigrationSourceSyncer struct {
	client                client.Client
	restConfig            *rest.Config // for init no-cached client of the runtime manager
	transportClient       transport.TransportClient
	transportConfig       *transport.TransportInternalConfig
	bundleVersion         *eventversion.Version
	processingMigrationId string
	clusterErrors         map[string]string
	leafHubName           string
}

func NewMigrationSourceSyncer(client client.Client, restConfig *rest.Config,
	transportClient transport.TransportClient,
	agentConfig *configs.AgentConfig,
) *MigrationSourceSyncer {
	return &MigrationSourceSyncer{
		client:          client,
		restConfig:      restConfig,
		transportClient: transportClient,
		transportConfig: agentConfig.TransportConfig,
		bundleVersion:   eventversion.NewVersion(),
		leafHubName:     agentConfig.LeafHubName,
	}
}

func (s *MigrationSourceSyncer) Sync(ctx context.Context, evt *cloudevents.Event) error {
	// Check if this migration event should be skipped
	skip, err := shouldSkipMigrationEvent(ctx, s.client, evt)
	if err != nil {
		return err
	}
	if skip {
		return nil
	}

	// Parse migration event
	migrationEvent := &migration.MigrationSourceBundle{}
	if err := json.Unmarshal(evt.Data(), migrationEvent); err != nil {
		return fmt.Errorf("failed to unmarshal migration event: %w", err)
	}
	log.Debugf("received migration event: migrationId=%s, stage=%s", migrationEvent.MigrationId, migrationEvent.Stage)
	defer func() {
		s.reportStatus(ctx, migrationEvent, err)
	}()

	err = s.handleStage(ctx, migrationEvent)
	if err != nil {
		return fmt.Errorf("failed to handle migration stage: %w", err)
	}

	// update the latest migration time into configmap to avoid duplicate processing
	if err := configs.SetSyncTimeState(ctx, s.client, configs.LatestMigrationTimeKey); err != nil {
		return fmt.Errorf("failed to update latest migration time: %w", err)
	}
	return nil
}

// handleStage processes different migration stages
func (s *MigrationSourceSyncer) handleStage(ctx context.Context, event *migration.MigrationSourceBundle) error {
	if event.MigrationId == "" {
		return fmt.Errorf("migrationId is required but not provided in stage %s", event.Stage)
	}

	s.clusterErrors = make(map[string]string)

	// Set current migration ID for stages that need cluster identification:
	// - processingMigrationId is empty: always set, to handle restart case
	// - Validating phase: always set (uses placement for cluster selection)
	if s.processingMigrationId == "" ||
		event.Stage == migrationv1alpha1.PhaseValidating {
		s.processingMigrationId = event.MigrationId
		s.bundleVersion.Reset()
	}

	// Check if migration ID matches for all other stages
	if s.processingMigrationId != event.MigrationId {
		return fmt.Errorf("expected migrationId %s, but got  %s", s.processingMigrationId,
			event.MigrationId)
	}

	switch event.Stage {
	case migrationv1alpha1.PhaseValidating:
		return s.executeStage(ctx, event, s.validating)
	case migrationv1alpha1.PhaseInitializing:
		return s.executeStage(ctx, event, s.initializing)
	case migrationv1alpha1.PhaseDeploying:
		return s.executeStage(ctx, event, s.deploying)
	case migrationv1alpha1.PhaseRegistering:
		return s.executeStage(ctx, event, s.registering)
	case migrationv1alpha1.PhaseCleaning:
		return s.executeStage(ctx, event, s.cleaning)
	case migrationv1alpha1.PhaseRollbacking:
		return s.executeStage(ctx, event, s.rollbacking)
	default:
		log.Warnf("unknown migration stage: %s", event.Stage)
		return nil
	}
}

// executeStage executes a migration stage with consistent logging
func (s *MigrationSourceSyncer) executeStage(ctx context.Context, source *migration.MigrationSourceBundle,
	stageFunc func(context.Context, *migration.MigrationSourceBundle) error,
) error {
	log.Infof("migration %s started: migrationId=%s, clusters=%v", source.Stage, source.MigrationId,
		source.ManagedClusters)

	if err := stageFunc(ctx, source); err != nil {
		log.Errorf("migration %s failed: migrationId=%s, error=%v",
			source.Stage, source.MigrationId, err)
		return err
	}

	log.Infof("migration %s completed: migrationId=%s", source.Stage, source.MigrationId)
	return nil
}

func (s *MigrationSourceSyncer) cleaning(ctx context.Context, source *migration.MigrationSourceBundle) error {
	// Delete bootstrap secret
	if err := deleteResourceIfExists(ctx, s.client, source.BootstrapSecret); err != nil {
		return fmt.Errorf("failed to delete bootstrap secret: %w", err)
	}

	// Delete klusterlet config
	klusterletConfig := &klusterletv1alpha1.KlusterletConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: klusterletConfigNamePrefix + source.ToHub,
		},
	}
	if err := deleteResourceIfExists(ctx, s.client, klusterletConfig); err != nil {
		return fmt.Errorf("failed to delete klusterletconfig: %w", err)
	}

	// Clean up managed clusters
	log.Infof("cleaning up %d managed clusters", len(source.ManagedClusters))
	return s.deleteClusterIfExists(ctx, source.ManagedClusters)
}

// deploying: send clusters and addon config into target hub
func (s *MigrationSourceSyncer) deploying(ctx context.Context, source *migration.MigrationSourceBundle) error {
	migrationResources := &migration.MigrationResourceBundle{
		MigrationId:           source.MigrationId,
		ManagedClusters:       []clusterv1.ManagedCluster{},
		KlusterletAddonConfig: []addonv1.KlusterletAddonConfig{},
	}

	// collect clusters and klusterletAddonConfig for migration
	for _, managedCluster := range source.ManagedClusters {
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
	toHub := source.ToHub

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
func (m *MigrationSourceSyncer) initializing(ctx context.Context, source *migration.MigrationSourceBundle) error {
	if source.BootstrapSecret == nil {
		return fmt.Errorf("bootstrap secret is nil when initializing")
	}
	bootstrapSecret := source.BootstrapSecret
	// ensure secret
	currentBootstrapSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name:      bootstrapSecret.Name,
		Namespace: bootstrapSecret.Namespace,
	}}
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		operation, err := controllerutil.CreateOrUpdate(ctx, m.client, currentBootstrapSecret, func() error {
			currentBootstrapSecret.Data = bootstrapSecret.Data
			return nil
		})
		log.Infof("bootstrap secret %s is %s", bootstrapSecret.GetName(), operation)
		return err
	})
	if err != nil {
		return fmt.Errorf("failed to create/update bootstrap secret: %w", err)
	}

	// ensure klusterletconfig
	klusterletConfig, err := generateKlusterletConfig(m.client, source.ToHub, bootstrapSecret.Name)
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
	managedClusters := source.ManagedClusters
	for _, managedCluster := range managedClusters {
		mc := &clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: managedCluster,
			},
		}
		if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := m.client.Get(ctx, client.ObjectKeyFromObject(mc), mc); err != nil {
				return err
			}
			currentAnnotations := mc.GetAnnotations()
			if currentAnnotations == nil {
				currentAnnotations = make(map[string]string)
			}
			currentAnnotations[KlusterletConfigAnnotation] = klusterletConfig.GetName()
			currentAnnotations[constants.ManagedClusterMigrating] = ""
			mc.SetAnnotations(currentAnnotations)

			err := m.client.Update(ctx, mc)
			if err != nil {
				return err
			}
			log.Infof("managed clusters %s is updated", mc.GetName())
			return nil
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
	ctx context.Context, migratingEvt *migration.MigrationSourceBundle,
) error {
	managedClusters := migratingEvt.ManagedClusters
	// set the hub accept client into false to trigger the re-registering
	for _, managedCluster := range managedClusters {

		log.Infof("updating managed cluster %s to set HubAcceptsClient as false", managedCluster)
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			mc := &clusterv1.ManagedCluster{}
			if err := m.client.Get(ctx, types.NamespacedName{
				Name: managedCluster,
			}, mc); err != nil {
				return err
			}
			if !mc.Spec.HubAcceptsClient {
				return nil
			}
			mc.Spec.HubAcceptsClient = false
			return m.client.Update(ctx, mc)
		})
		if err != nil {
			return fmt.Errorf("failed to set HubAcceptsClient to false for managed cluster %s: %w", managedCluster, err)
		}
	}
	return nil
}

func ReportMigrationStatus(
	ctx context.Context,
	transportClient transport.TransportClient,
	migrationBundle *migration.MigrationStatusBundle,
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

	// remove migrating and klusterletconfig annotations from managed cluster
	annotations := cluster.GetAnnotations()
	if annotations != nil {
		delete(annotations, constants.ManagedClusterMigrating)
		delete(annotations, KlusterletConfigAnnotation)
		delete(annotations, kubectlConfigAnnotation)
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
	annotations := addonConfig.GetAnnotations()
	if annotations != nil {
		delete(annotations, kubectlConfigAnnotation)
		addonConfig.SetAnnotations(annotations)
	}

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
func (s *MigrationSourceSyncer) rollbacking(ctx context.Context, spec *migration.MigrationSourceBundle) error {
	log.Infof("performing rollback for stage: %s", spec.RollbackStage)

	switch spec.RollbackStage {
	case migrationv1alpha1.PhaseInitializing:
		return s.rollbackInitializing(ctx, spec)
	case migrationv1alpha1.PhaseDeploying:
		return s.rollbackDeploying(ctx, spec)
	case migrationv1alpha1.PhaseRegistering:
		return s.rollbackRegistering(ctx, spec)
	default:
		return fmt.Errorf("no specific rollback action needed for stage: %s", spec.RollbackStage)
	}
}

// rollbackInitializing removes migration-related annotations from managed clusters
// and cleans up bootstrap secret and KlusterletConfig created during initializing phase
func (s *MigrationSourceSyncer) rollbackInitializing(ctx context.Context,
	migrationSourceHubEvent *migration.MigrationSourceBundle,
) error {
	// 1. Clean up bootstrap secret if it exists
	if migrationSourceHubEvent.BootstrapSecret != nil {
		log.Infof("cleaning up bootstrap secret: %s", migrationSourceHubEvent.BootstrapSecret.Name)
		if err := deleteResourceIfExists(ctx, s.client, migrationSourceHubEvent.BootstrapSecret); err != nil {
			return fmt.Errorf("failed to delete bootstrap secret %s: %v", migrationSourceHubEvent.BootstrapSecret.Name, err)
		}
		log.Infof("successfully deleted bootstrap secret: %s", migrationSourceHubEvent.BootstrapSecret.Name)
	}

	// 2. Clean up KlusterletConfig
	klusterletConfig := &klusterletv1alpha1.KlusterletConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: klusterletConfigNamePrefix + migrationSourceHubEvent.ToHub,
		},
	}
	log.Infof("cleaning up KlusterletConfig: %s", klusterletConfig.Name)
	if err := deleteResourceIfExists(ctx, s.client, klusterletConfig); err != nil {
		return fmt.Errorf("failed to delete KlusterletConfig %s: %v", klusterletConfig.Name, err)
	}
	log.Infof("successfully deleted KlusterletConfig: %s", klusterletConfig.Name)

	// 3. Clean up managed cluster annotations
	for _, managedCluster := range migrationSourceHubEvent.ManagedClusters {
		log.Infof("cleaning up annotations for managed cluster: %s", managedCluster)

		mc := &clusterv1.ManagedCluster{}
		err := s.client.Get(ctx, types.NamespacedName{Name: managedCluster}, mc)
		if err != nil {
			if apierrors.IsNotFound(err) {
				log.Infof("managed cluster %s not found, skipping annotation cleanup", managedCluster)
				continue
			}
			s.clusterErrors[managedCluster] = fmt.Sprintf("failed to get managed cluster %s: %v", managedCluster, err)
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
		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			lastestCluster := &clusterv1.ManagedCluster{}
			err := s.client.Get(ctx, types.NamespacedName{Name: managedCluster}, lastestCluster)
			if err != nil {
				return err
			}
			delete(annotations, constants.ManagedClusterMigrating)
			delete(annotations, KlusterletConfigAnnotation)
			lastestCluster.SetAnnotations(annotations)
			return s.client.Update(ctx, lastestCluster)
		})
		if err != nil {
			s.clusterErrors[managedCluster] = fmt.Sprintf("failed to remove annotations from cluster %s: %v",
				managedCluster, err)
			continue
		}

		log.Infof("successfully removed migration annotations from managed cluster: %s", managedCluster)
	}

	// Prepare detailed result message
	if len(s.clusterErrors) > 0 {
		return fmt.Errorf("failed to rollback %v managed clusters, get more details in events", len(s.clusterErrors))
	}
	return nil
}

// rollbackDeploying handles rollback operations for deploying stage
func (s *MigrationSourceSyncer) rollbackDeploying(ctx context.Context, source *migration.MigrationSourceBundle) error {
	log.Infof("rollback deploying stage for clusters: %v", source.ManagedClusters)

	// For deploying stage rollback, we need to:
	// 1. Clean up migration annotations from managed clusters on source hub
	// 2. The target hub will handle removing the deployed addonConfig and clusters

	// Clean up annotations on source hub - use the enhanced error handling
	if err := s.rollbackInitializing(ctx, source); err != nil {
		// Return error with deploying stage context
		return fmt.Errorf("deploying stage rollback failed: %v", err)
	}

	log.Info("completed deploying stage rollback")
	return nil
}

// rollbackRegistering handles rollback operations for registering stage
func (s *MigrationSourceSyncer) rollbackRegistering(ctx context.Context, spec *migration.MigrationSourceBundle) error {
	log.Infof("rollback registering stage for clusters: %v", spec.ManagedClusters)

	// For registering stage rollback, we may need to:
	// 1. Restore original cluster registration configuration
	// 2. Remove bootstrap secrets
	// 3. Clean up migration annotations
	// 4. Set HubAcceptsClient to true
	if err := s.rollbackDeploying(ctx, spec); err != nil {
		return fmt.Errorf("deploying stage rollback failed: %v", err)
	}

	for _, managedCluster := range spec.ManagedClusters {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			mc := &clusterv1.ManagedCluster{}
			if err := s.client.Get(ctx, types.NamespacedName{
				Name: managedCluster,
			}, mc); err != nil {
				return err
			}
			if mc.Spec.HubAcceptsClient {
				return nil
			}
			mc.Spec.HubAcceptsClient = true
			return s.client.Update(ctx, mc)
		})
		if err != nil {
			return fmt.Errorf("failed to set HubAcceptsClient to true for managed cluster %s: %w", managedCluster, err)
		}
	}

	return nil
}

// reportStatus reports the migration status back to global hub
func (s *MigrationSourceSyncer) reportStatus(ctx context.Context, spec *migration.MigrationSourceBundle, err error) {
	// Don't report if migration ID doesn't match current one(expect the rollbacking status for the initilzing)
	if s.processingMigrationId != spec.MigrationId &&
		(spec.Stage != migrationv1alpha1.PhaseRollbacking && spec.RollbackStage != migrationv1alpha1.PhaseInitializing) {
		return
	}

	errMessage := ""
	if err != nil {
		errMessage = err.Error()
	}

	reportErr := ReportMigrationStatus(
		cecontext.WithTopic(ctx, s.transportConfig.KafkaCredential.StatusTopic),
		s.transportClient,
		&migration.MigrationStatusBundle{
			MigrationId:     spec.MigrationId,
			Stage:           spec.Stage,
			ErrMessage:      errMessage,
			ManagedClusters: spec.ManagedClusters,
			ClusterErrors:   s.clusterErrors,
		},
		s.bundleVersion)

	if reportErr != nil {
		log.Errorf("failed to report migration status for stage %s: %v", spec.Stage, reportErr)
	}
}

// deleteClusterIfExists handles cleaning up multiple clusters
func (s *MigrationSourceSyncer) deleteClusterIfExists(ctx context.Context, clusterNames []string) error {
	var errors []string

	for _, clusterName := range clusterNames {
		log.Debugf("cleaning up managed cluster %s", clusterName)
		if err := s.cleanupSingleCluster(ctx, clusterName); err != nil {
			errors = append(errors, fmt.Sprintf("cluster %s: %v", clusterName, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to clean up some clusters: %s", strings.Join(errors, "; "))
	}

	return nil
}

// cleanupSingleCluster handles cleanup logic for a single managed cluster
func (s *MigrationSourceSyncer) cleanupSingleCluster(ctx context.Context, clusterName string) error {
	mc := &clusterv1.ManagedCluster{}
	if err := s.client.Get(ctx, types.NamespacedName{Name: clusterName}, mc); err != nil {
		if apierrors.IsNotFound(err) {
			log.Debugf("managed cluster %s not found, skipping cleanup", clusterName)
			return nil
		}
		return fmt.Errorf("failed to get managed cluster: %w", err)
	}

	// For cleaning stage, delete the cluster if HubAcceptsClient is false
	if !mc.Spec.HubAcceptsClient {
		if err := s.client.Delete(ctx, mc); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete managed cluster: %w", err)
		}
		log.Infof("deleted managed cluster %s", clusterName)
	}
	return nil
}

func ResyncMigrationEvent(ctx context.Context, transportClient transport.TransportClient,
	transportConfig *transport.TransportInternalConfig,
) error {
	return ReportMigrationStatus(
		cecontext.WithTopic(ctx, transportConfig.KafkaCredential.StatusTopic),
		transportClient,
		&migration.MigrationStatusBundle{
			Resync: true,
		}, eventversion.NewVersion(),
	)
}

// validating handles the validating phase - get clusters from placement decisions and validate them
func (s *MigrationSourceSyncer) validating(ctx context.Context, source *migration.MigrationSourceBundle) error {
	// If placement name is provided, get clusters from placement decisions
	if source.PlacementName != "" {
		clusters, err := s.getClustersFromPlacementDecisions(ctx, source.PlacementName)
		if err != nil {
			return fmt.Errorf("failed to get managed clusters from placement decisions: %w", err)
		}
		if len(clusters) == 0 {
			return fmt.Errorf("no managed clusters found in placement %q", source.PlacementName)
		}
		source.ManagedClusters = clusters
	}

	// Validate the clusters
	if len(source.ManagedClusters) > 0 {
		if err := s.validateManagedClusters(ctx, source.ManagedClusters); err != nil {
			return err
		}
		log.Infof("validated %d managed clusters for migration %s", len(source.ManagedClusters), source.MigrationId)
	} else {
		log.Infof("no managed clusters to validate for migration %s", source.MigrationId)
	}

	return nil
}

// getClustersFromPlacementDecisions retrieves cluster list from placement decisions
func (s *MigrationSourceSyncer) getClustersFromPlacementDecisions(
	ctx context.Context,
	placementName string,
) ([]string, error) {
	// List placement decisions that match the placement name
	placementDecisions := &clusterv1beta1.PlacementDecisionList{}
	err := s.client.List(ctx, placementDecisions,
		client.MatchingLabels{clusterv1beta1.PlacementLabel: placementName})
	if err != nil {
		return nil, fmt.Errorf("failed to list placement decisions for placement %s: %w", placementName, err)
	}

	var clusters []string
	for _, decision := range placementDecisions.Items {
		for _, clusterDecision := range decision.Status.Decisions {
			if clusterDecision.ClusterName != "" {
				clusters = append(clusters, clusterDecision.ClusterName)
			}
		}
	}

	log.Infof("found %d managed clusters from placement %s", len(clusters), placementName)
	return clusters, nil
}

// validateManagedClusters validates managed clusters for migration
func (s *MigrationSourceSyncer) validateManagedClusters(ctx context.Context, clusterNames []string) error {
	for _, clusterName := range clusterNames {
		if err := s.validateSingleCluster(ctx, clusterName); err != nil {
			s.clusterErrors[clusterName] = err.Error()
		}
	}

	if len(s.clusterErrors) > 0 {
		return fmt.Errorf("%v managed clusters validation failed, get more details in events", len(s.clusterErrors))
	}

	return nil
}

// validateSingleCluster validates a single managed cluster for migration
func (s *MigrationSourceSyncer) validateSingleCluster(ctx context.Context, clusterName string) error {
	mc := &clusterv1.ManagedCluster{}
	if err := s.client.Get(ctx, types.NamespacedName{Name: clusterName}, mc); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("managed cluster %v is not found in managed hub %v", clusterName, s.leafHubName)
		}
		return fmt.Errorf("failed to get managed cluster %v from managed hub %v: %w", clusterName, s.leafHubName, err)
	}

	// Check if cluster is hosted
	if s.isHostedCluster(mc) {
		return fmt.Errorf(
			"managed cluster %v is imported as hosted mode in managed hub %v, it cannot be migrated",
			clusterName, s.leafHubName)
	}

	// Check if cluster is local cluster
	if mc.Labels != nil && mc.Labels[constants.LocalClusterName] == "true" {
		return fmt.Errorf(
			"managed cluster %v is local cluster in managed hub %v, it cannot be migrated",
			clusterName, s.leafHubName)
	}

	// Check if cluster is available
	if !s.isManagedClusterAvailable(mc) {
		return fmt.Errorf(
			"managed cluster %v is not available in managed hub %v, it cannot be migrated",
			clusterName, s.leafHubName)
	}

	// Check if cluster is a managed hub
	if s.isManagedHub(mc) {
		return fmt.Errorf(
			"managed cluster %v is a managed hub cluster in managed hub %v, it cannot be migrated",
			clusterName, s.leafHubName)
	}

	return nil
}

// isManagedHub checks if the cluster is a managed hub cluster
func (s *MigrationSourceSyncer) isManagedHub(cluster *clusterv1.ManagedCluster) bool {
	if cluster.Annotations == nil {
		return false
	}
	if cluster.Annotations[constants.AnnotationONMulticlusterHub] == "true" {
		return true
	}

	return false
}

// isHostedCluster checks if a managed cluster is hosted
func (s *MigrationSourceSyncer) isHostedCluster(mc *clusterv1.ManagedCluster) bool {
	return mc.Annotations != nil &&
		mc.Annotations[constants.AnnotationClusterDeployMode] == constants.ClusterDeployModeHosted
}

// isManagedClusterAvailable returns true if the ManagedCluster is available
func (s *MigrationSourceSyncer) isManagedClusterAvailable(mc *clusterv1.ManagedCluster) bool {
	for _, cond := range mc.Status.Conditions {
		if cond.Type == clusterv1.ManagedClusterConditionAvailable && cond.Status == "True" {
			return true
		}
	}
	return false
}
