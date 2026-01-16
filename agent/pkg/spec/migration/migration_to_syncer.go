// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package migration

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloudevents/sdk-go/protocol/kafka_confluent/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	cecontext "github.com/cloudevents/sdk-go/v2/context"
	cetypes "github.com/cloudevents/sdk-go/v2/types"
	apiconstants "github.com/stolostron/cluster-lifecycle-api/constants"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

const (
	KlusterletManifestWorkSuffix = "-klusterlet"
	ClusterManagerName           = "cluster-manager"
	errMsgFailedToGet            = "failed to get %s from source resource: %w"

	// Bootstrap ClusterRole names for different environments
	DefaultACMBootstrapClusterRole = "open-cluster-management:managedcluster:bootstrap:agent-registration"
	DefaultOCMBootstrapClusterRole = "open-cluster-management:bootstrap"
)

var (
	log                = logger.DefaultZapLogger()
	registeringTimeout = 10 * time.Minute // the registering stage timeout should less than migration timeout
)

// shouldSkipMigrationEvent checks if a migration event should be skipped based on cached migration time
// Returns true if the event should be skipped, false otherwise
//
// Skip conditions:
//  1. If cached: skip if latestMigrationTime is after event time
//  2. If not cached: skip if event is older than 10 minutes
func shouldSkipMigrationEvent(ctx context.Context, client client.Client, evt *cloudevents.Event) (bool, error) {
	cached, latestMigrationTime, err := configs.GetSyncTimeState(ctx, client, migrationStateKey(evt))
	if err != nil {
		return false, fmt.Errorf("failed to get latest migration time from configmap: %w", err)
	}
	if cached {
		if latestMigrationTime.After(evt.Time()) {
			log.Infof("latest migration time %s is after event time %s, skip processing", latestMigrationTime, evt.Time())
			return true, nil
		}
	} else {
		if time.Since(evt.Time()) > 10*time.Minute {
			log.Infof("latest migration time is not cached, and the event time is 10 minutes ago, skip processing")
			return true, nil
		}
	}
	return false, nil
}

type MigrationTargetSyncer struct {
	client                client.Client
	transportClient       transport.TransportClient
	transportConfig       *transport.TransportInternalConfig
	bundleVersion         *eventversion.Version
	processingMigrationId string
	leafHubName           string
	// Batch tracking for deploying stage
	deployingTotalClusters     int             // Total clusters expected in deploying stage
	deployingProcessedClusters map[string]bool // Track which clusters have been processed
}

func NewMigrationTargetSyncer(client client.Client,
	transportClient transport.TransportClient,
	agentConfig *configs.AgentConfig,
) *MigrationTargetSyncer {
	return &MigrationTargetSyncer{
		client:          client,
		transportClient: transportClient,
		transportConfig: agentConfig.TransportConfig,
		bundleVersion:   eventversion.NewVersion(),
		leafHubName:     agentConfig.LeafHubName,
	}
}

func (s *MigrationTargetSyncer) Sync(ctx context.Context, evt *cloudevents.Event) error {
	log.Infof("received migration event from %s", evt.Source())

	// Check if this migration event should be skipped
	skip, err := shouldSkipMigrationEvent(ctx, s.client, evt)
	if err != nil {
		return err
	}
	if skip {
		return nil
	}

	// Use local variables instead of instance fields to avoid race conditions
	// when processing concurrent events (dispatcher uses async goroutines)
	var receivedMigrationId string
	var receivedStage string

	clusterErrors := map[string]string{}
	defer func() {
		if receivedStage == "" || receivedMigrationId == "" {
			log.Warnf("stage(%s) or migrationId(%s) is empty ", receivedStage, receivedMigrationId)
			return
		}

		migrationStatus := &migration.MigrationStatusBundle{
			MigrationId: receivedMigrationId,
			Stage:       receivedStage,
		}

		reportStatus := true
		if err != nil {
			migrationStatus.ErrMessage = err.Error()
			if len(clusterErrors) > 0 {
				migrationStatus.ClusterErrors = clusterErrors
			}
		} else {
			if receivedStage == migrationv1alpha1.PhaseDeploying {
				if s.deployingTotalClusters > 0 && len(s.deployingProcessedClusters) == s.deployingTotalClusters {
					log.Infof("deploying: all %d clusters have been processed successfully", s.deployingTotalClusters)
					// Reset batch tracking for next migration
					s.deployingTotalClusters = 0
					s.deployingProcessedClusters = nil
				} else {
					log.Infof("deploying: not finished, skip reporting status")
					reportStatus = false
				}
			}
		}

		if reportStatus {
			err = ReportMigrationStatus(cecontext.WithTopic(ctx, s.transportConfig.KafkaCredential.StatusTopic),
				s.transportClient, migrationStatus, s.bundleVersion)
			if err != nil {
				log.Errorf("failed to report migration status: %v", err)
			}
		}

		// update the latest migration time into configmap to avoid duplicate processing
		if err := configs.SetSyncTimeState(ctx, s.client, migrationStateKey(evt), evt.Time()); err != nil {
			log.Errorf("failed to update latest migration time: %w", err)
		}
	}()

	// Handle direct deploying events from source hub (not from global hub)
	if evt.Source() != constants.CloudEventGlobalHubClusterName {
		receivedStage = migrationv1alpha1.PhaseDeploying
		// Extract migrationId from deploying event for status reporting
		resourceEvent := &migration.MigrationResourceBundle{}
		if unmarshalErr := json.Unmarshal(evt.Data(), resourceEvent); unmarshalErr == nil {
			receivedMigrationId = resourceEvent.MigrationId
		}
		err = s.deploying(ctx, evt)
		if err != nil {
			return fmt.Errorf("failed to handle deploying event: %w", err)
		}
		return nil
	}

	// Parse migration event from global hub
	event := &migration.MigrationTargetBundle{}
	if err := json.Unmarshal(evt.Data(), event); err != nil {
		return fmt.Errorf("failed to unmarshal migration event: %w", err)
	}

	log.Debugf("received migration event: migrationId=%s, stage=%s", event.MigrationId, event.Stage)
	if event.MigrationId == "" {
		return fmt.Errorf("migrationId is required but not provided in event")
	}
	receivedMigrationId = event.MigrationId
	receivedStage = event.Stage

	// Set current migration ID and reset bundle version for initializing stage or rollbacking to initializing
	if s.processingMigrationId == "" ||
		event.Stage == migrationv1alpha1.PhaseValidating {
		s.processingMigrationId = event.MigrationId
		s.bundleVersion.Reset()
	}

	// Check if migration ID matches for all other stages, ignore the rollbacking status for the initializing
	if s.processingMigrationId != event.MigrationId {
		return fmt.Errorf("expected migrationId %s, but got  %s", s.processingMigrationId, event.MigrationId)
	}

	err = s.handleStage(ctx, event, clusterErrors)
	if err != nil {
		return fmt.Errorf("failed to handle migration stage: %w", err)
	}

	return nil
}

// use "kafkaTopic--eventSource" as key
func migrationStateKey(evt *cloudevents.Event) string {
	topic, err := cetypes.ToString(evt.Extensions()[kafka_confluent.KafkaTopicKey])
	if err != nil {
		log.Info("failed to parse topic from event, use source as key", "error", err)
		topic = "topic"
	}
	return fmt.Sprintf("%s--%s", topic, evt.Source())
}

// handleStage processes different migration stages using switch statement
func (s *MigrationTargetSyncer) handleStage(ctx context.Context, target *migration.MigrationTargetBundle,
	clusterErrors map[string]string,
) error {
	switch target.Stage {
	case migrationv1alpha1.PhaseValidating:
		return s.executeStage(ctx, target, s.validating, clusterErrors)
	case migrationv1alpha1.PhaseInitializing:
		return s.executeStage(ctx, target, s.initializing, clusterErrors)
	case migrationv1alpha1.PhaseRegistering:
		return s.executeStage(ctx, target, s.registering, clusterErrors)
	case migrationv1alpha1.PhaseCleaning:
		return s.executeStage(ctx, target, s.cleaning, clusterErrors)
	case migrationv1alpha1.PhaseRollbacking:
		return s.executeStage(ctx, target, s.rollbacking, clusterErrors)
	default:
		log.Warnf("unknown migration stage: %s", target.Stage)
		return fmt.Errorf("unknown migration stage: %s", target.Stage)
	}
}

// executeStage executes a migration stage with consistent logging
func (s *MigrationTargetSyncer) executeStage(
	ctx context.Context,
	event *migration.MigrationTargetBundle,
	stageFunc func(context.Context, *migration.MigrationTargetBundle, map[string]string) error,
	clusterErrors map[string]string,
) error {
	log.Infof("migration %s started: migrationId=%s, clusters=%v", event.Stage, event.MigrationId, event.ManagedClusters)

	if err := stageFunc(ctx, event, clusterErrors); err != nil {
		log.Errorf("migration %s failed: migrationId=%s, error=%v",
			event.Stage, event.MigrationId, err)
		return err
	}

	log.Infof("migration %s completed: migrationId=%s", event.Stage, event.MigrationId)
	return nil
}

func (s *MigrationTargetSyncer) cleaning(ctx context.Context,
	event *migration.MigrationTargetBundle,
	clusterErrors map[string]string,
) error {
	msaName := event.ManagedServiceAccountName
	msaNamespace := event.ManagedServiceAccountInstallNamespace

	// Clean up the auto-import disable annotation from the managed clusters
	for _, clusterName := range event.ManagedClusters {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			mc := &clusterv1.ManagedCluster{}
			err := s.client.Get(ctx, types.NamespacedName{Name: clusterName}, mc)
			if err != nil {
				return err
			}
			annotations := mc.GetAnnotations()
			_, ok := annotations[apiconstants.DisableAutoImportAnnotation]
			if !ok {
				return nil
			}
			delete(annotations, apiconstants.DisableAutoImportAnnotation)
			mc.SetAnnotations(annotations)
			return s.client.Update(ctx, mc)
		})
		if err != nil {
			return fmt.Errorf("failed to remove auto-import disable annotation from cluster %s: %v",
				clusterName, err)
		}
	}

	// Remove MSA user from ClusterManager AutoApproveUsers list
	if err := s.removeAutoApproveUser(ctx, msaName, msaNamespace); err != nil {
		log.Errorf("failed to remove auto approve user from ClusterManager: %v", err)
		return err
	}

	// Clean up RBAC resources created for migration
	if err := s.cleanupMigrationRBAC(ctx, msaName); err != nil {
		log.Errorf("failed to cleanup migration RBAC resources: %v", err)
		return err
	}
	return nil
}

// validating handles the validating phase
func (s *MigrationTargetSyncer) validating(
	ctx context.Context, source *migration.MigrationTargetBundle,
	clusterErrors map[string]string,
) error {
	// Validate the clusters
	if len(source.ManagedClusters) > 0 {
		if err := s.validateManagedClusters(ctx, source.ManagedClusters, clusterErrors); err != nil {
			return err
		}
		log.Infof("validated %d clusters for migration %s", len(source.ManagedClusters), source.MigrationId)
	} else {
		log.Infof("no clusters to validate for migration %s", source.MigrationId)
	}

	return nil
}

// validateManagedClusters validates managed clusters for migration
func (s *MigrationTargetSyncer) validateManagedClusters(
	ctx context.Context, clusterNames []string, clusterErrors map[string]string,
) error {
	for _, clusterName := range clusterNames {
		mc := &clusterv1.ManagedCluster{}
		if err := s.client.Get(ctx, types.NamespacedName{Name: clusterName}, mc); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			clusterErrors[clusterName] = err.Error()
			continue
		}
		clusterErrors[clusterName] = fmt.Sprintf("managed cluster %v already exists in target hub", clusterName)
	}

	if len(clusterErrors) > 0 {
		return fmt.Errorf("%v clusters validation failed, get more details in events", len(clusterErrors))
	}

	return nil
}

// registering watches the migrated managed clusters
func (s *MigrationTargetSyncer) registering(ctx context.Context,
	event *migration.MigrationTargetBundle, clusterErrors map[string]string,
) error {
	if len(event.ManagedClusters) == 0 {
		return fmt.Errorf("no managed clusters found in migration event: %s", event.MigrationId)
	}

	// Use the timeout from the manager if provided, otherwise fall back to the default
	timeout := registeringTimeout
	if event.RegisteringTimeoutMinutes > 0 {
		timeout = time.Duration(event.RegisteringTimeoutMinutes) * time.Minute
	}

	err := wait.PollUntilContextTimeout(ctx, 5*time.Second, timeout, true,
		func(context.Context) (done bool, err error) {
			clear(clusterErrors)
			for _, clusterName := range event.ManagedClusters {
				if err := s.checkClusterManifestWork(ctx, clusterName); err != nil {
					log.Debugf("cluster %s not ready: %v", clusterName, err)
					clusterErrors[clusterName] = err.Error()
				}
			}
			if len(clusterErrors) > 0 {
				return false, nil
			}
			clear(clusterErrors)
			return true, nil
		},
	)
	if err != nil {
		return fmt.Errorf("failed to wait for %d managed clusters to be ready, get more details in events: %w",
			len(clusterErrors), err)
	}
	log.Infof("all %d managed clusters are ready for migration", len(event.ManagedClusters))
	return nil
}

// initializing create the permission for the migration service account, and enable auto-approval for registration
func (s *MigrationTargetSyncer) initializing(ctx context.Context,
	event *migration.MigrationTargetBundle,
	clusterErrors map[string]string,
) error {
	msaName := event.ManagedServiceAccountName
	msaNamespace := event.ManagedServiceAccountInstallNamespace
	if err := s.ensureClusterManagerAutoApproval(ctx, msaName, msaNamespace); err != nil {
		return err
	}
	if err := s.ensureSubjectAccessReviewRole(ctx, msaName); err != nil {
		return err
	}
	if err := s.ensureSubjectAccessReviewRoleBinding(ctx, msaName, msaNamespace); err != nil {
		return err
	}
	// bind migration sa with "open-cluster-management:managedcluster:bootstrap:agent-registration"
	if err := s.ensureRegistrationClusterRoleBinding(ctx, msaName, msaNamespace); err != nil {
		return err
	}

	// In OCM environment, delay 1 minute after all resources are created to allow manual testing
	if s.isOCMEnvironment(ctx) {
		log.Infof("OCM environment detected, delaying 1 minute after initializing to allow manual resource mocking")
		time.Sleep(1 * time.Minute)
	}

	return nil
}

func (s *MigrationTargetSyncer) deploying(ctx context.Context, evt *cloudevents.Event) error {
	log.Infof("deploying: received migration event from sourceHub=%s", evt.Source())

	// Extract migrationId from deploying event payload
	resourceEvent := &migration.MigrationResourceBundle{}
	if unmarshalErr := json.Unmarshal(evt.Data(), resourceEvent); unmarshalErr != nil {
		return fmt.Errorf("failed to unmarshal deploying event: %w", unmarshalErr)
	}

	log.Debugf("deploying: received migrationId=%s with %d cluster resources",
		resourceEvent.MigrationId, len(resourceEvent.MigrationClusterResources))

	// Initialize or validate migration tracking
	if s.processingMigrationId == "" {
		s.processingMigrationId = resourceEvent.MigrationId
		s.bundleVersion.Reset()
		log.Infof("deploying: initialized processing migrationId=%s", s.processingMigrationId)
	}

	// only handle the current migration event, ignore the previous ones
	if s.processingMigrationId != resourceEvent.MigrationId {
		return fmt.Errorf("expected migrationId %s, but got %s", s.processingMigrationId,
			resourceEvent.MigrationId)
	}

	// Initialize batch tracking
	if s.deployingProcessedClusters == nil {
		totalClusters := 0
		if val, err := cetypes.ToInteger(evt.Extensions()[migration.ExtTotalClusters]); err == nil {
			totalClusters = int(val)
		} else {
			log.Errorf("deploying: failed to convert totalclusters extension to integer: %v", err)
			return fmt.Errorf("failed to convert totalclusters extension to integer: %v", err)
		}
		if totalClusters == 0 {
			log.Error("deploying: totalclusters extension is 0, expecting at least 1 cluster")
			return fmt.Errorf("totalclusters extension is 0, expecting at least 1 cluster")
		}
		s.deployingTotalClusters = totalClusters
		s.deployingProcessedClusters = make(map[string]bool)
		log.Infof("deploying: initialized batch tracking for migration %s, expecting %d total clusters",
			resourceEvent.MigrationId, totalClusters)
	}

	// Process the resources in this batch
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		return s.syncMigrationResources(ctx, resourceEvent)
	}); err != nil {
		return err
	}

	// Track processed clusters
	for _, clusterResource := range resourceEvent.MigrationClusterResources {
		s.deployingProcessedClusters[clusterResource.ClusterName] = true
	}

	log.Infof("deploying: processed batch with %d clusters, total processed: %d/%d",
		len(resourceEvent.MigrationClusterResources), len(s.deployingProcessedClusters), s.deployingTotalClusters)
	return nil
}

func (s *MigrationTargetSyncer) ensureNamespace(ctx context.Context, namespace string) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		operation, err := controllerutil.CreateOrUpdate(ctx, s.client, ns, func() error {
			return nil
		})
		log.Debugf("namespace %s is %s", ns.Name, operation)
		return err
	})
	if err != nil {
		return fmt.Errorf("failed to create or update namespace %s: %w", ns.Name, err)
	}
	return nil
}

func (s *MigrationTargetSyncer) syncMigrationResources(ctx context.Context,
	migrationResources *migration.MigrationResourceBundle,
) error {
	log.Infof("deploying: processing %d clusters", len(migrationResources.MigrationClusterResources))

	totalResources := 0
	totalClusters := len(migrationResources.MigrationClusterResources)
	// Process each cluster's resources
	for idx, clusterResource := range migrationResources.MigrationClusterResources {
		clusterName := clusterResource.ClusterName
		log.Infof("deploying: [%d/%d] processing cluster: %s with %d resources",
			idx+1, totalClusters, clusterName, len(clusterResource.ResourceList))

		log.Debugf("deploying: ensuring namespace exists for cluster %s", clusterName)
		if err := s.ensureNamespace(ctx, clusterName); err != nil {
			return err
		}

		// Process each resource in the cluster
		for resIdx, resource := range clusterResource.ResourceList {
			log.Debugf("deploying: [cluster %s] [%d/%d] syncing resource kind=%s, name=%s, namespace=%s",
				clusterName, resIdx+1, len(clusterResource.ResourceList),
				resource.GetKind(), resource.GetName(), resource.GetNamespace())

			if err := s.syncResource(ctx, &resource); err != nil {
				return fmt.Errorf("failed to sync resource %s/%s for cluster %s: %w",
					resource.GetKind(), resource.GetName(), clusterName, err)
			}

			log.Debugf("deploying: [cluster %s] [%d/%d] successfully synced resource kind=%s, name=%s",
				clusterName, resIdx+1, len(clusterResource.ResourceList),
				resource.GetKind(), resource.GetName())
		}

		totalResources += len(clusterResource.ResourceList)
		log.Infof("deploying: [%d/%d] completed processing cluster %s",
			idx+1, totalClusters, clusterName)
	}

	log.Infof("deploying: successfully synced all %d resources across %d clusters for migrationId=%s",
		totalResources, totalClusters, migrationResources.MigrationId)
	return nil
}

// syncResource handles syncing individual resources from the migration bundle
// Works directly with unstructured resources without type conversion
func (s *MigrationTargetSyncer) syncResource(ctx context.Context, resource *unstructured.Unstructured) error {
	resourceKey := fmt.Sprintf("%s/%s/%s", resource.GetKind(), resource.GetNamespace(), resource.GetName())
	log.Debugf("deploying: syncResource started for resource=%s", resourceKey)

	// Store the source resource data that needs to be applied
	sourceLabels := resource.GetLabels()
	sourceAnnotations := resource.GetAnnotations()

	// Handle ConfigMap and Secret data field
	sourceData, hasData, err := unstructured.NestedFieldCopy(resource.Object, "data")
	if err != nil {
		return fmt.Errorf(errMsgFailedToGet, "data", err)
	}

	// Handle spec field
	sourceSpec, hasSpec, err := unstructured.NestedFieldCopy(resource.Object, "spec")
	if err != nil {
		return fmt.Errorf(errMsgFailedToGet, "spec", err)
	}

	// Handle status field (will be applied separately if exists)
	sourceStatus, hasStatus, err := unstructured.NestedFieldCopy(resource.Object, "status")
	if err != nil {
		return fmt.Errorf(errMsgFailedToGet, "status", err)
	}

	log.Debugf("deploying: creating or updating resource=%s", resourceKey)
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		operation, err := controllerutil.CreateOrUpdate(ctx, s.client, resource,
			func() error {
				// Apply labels and annotations from source
				if sourceLabels != nil {
					resource.SetLabels(sourceLabels)
				}
				if sourceAnnotations != nil {
					resource.SetAnnotations(sourceAnnotations)
				}

				// Apply data from source if it exists (for ConfigMap/Secret)
				if hasData {
					if err := unstructured.SetNestedField(resource.Object, sourceData, "data"); err != nil {
						return fmt.Errorf("failed to set data: %w", err)
					}
				}

				// Apply spec from source if it exists
				if hasSpec {
					if err := unstructured.SetNestedField(resource.Object, sourceSpec, "spec"); err != nil {
						return fmt.Errorf("failed to set spec: %w", err)
					}
				}
				return nil
			})
		log.Debugf("deploying: resource=%s operation=%s", resourceKey, operation)
		return err
	})
	if err != nil {
		log.Errorf("deploying: failed to create or update resource=%s: %v", resourceKey, err)

		// Skip Invalid error.
		if !apierrors.IsInvalid(err) {
			return fmt.Errorf("failed to create or update %s %s: %w", resource.GetKind(), resource.GetName(), err)
		}
	}

	// Apply status separately using Status().Update() if it exists
	if hasStatus {
		log.Debugf("deploying: updating status for resource=%s", resourceKey)
		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			// Get the latest version of the resource to update status
			latestResource := &unstructured.Unstructured{}
			latestResource.SetGroupVersionKind(resource.GroupVersionKind())
			if err := s.client.Get(ctx, client.ObjectKeyFromObject(resource), latestResource); err != nil {
				return fmt.Errorf("failed to get latest resource: %w", err)
			}

			// Set the status from source
			if err := unstructured.SetNestedField(latestResource.Object, sourceStatus, "status"); err != nil {
				return fmt.Errorf("failed to set status: %w", err)
			}

			// Update only the status subresource
			return s.client.Status().Update(ctx, latestResource)
		})
		if err != nil {
			log.Errorf("deploying: failed to update status for resource=%s: %v", resourceKey, err)
			return fmt.Errorf("failed to update status for %s %s: %w", resource.GetKind(), resource.GetName(), err)
		}
		log.Debugf("deploying: successfully updated status for resource=%s", resourceKey)
	}

	log.Debugf("deploying: syncResource completed successfully for resource=%s", resourceKey)
	return nil
}

func (s *MigrationTargetSyncer) ensureClusterManagerAutoApproval(ctx context.Context,
	saName, saNamespace string,
) error {
	foundClusterManager := &operatorv1.ClusterManager{}
	if err := s.client.Get(ctx,
		types.NamespacedName{Name: ClusterManagerName}, foundClusterManager); err != nil {
		return err
	}

	clusterManager := foundClusterManager.DeepCopy()
	// check if the ManagedClusterAutoApproval feature is enabled and
	// the service account is added to the auto-approve list
	autoApproveUser := fmt.Sprintf("system:serviceaccount:%s:%s", saNamespace, saName)
	autoApproveFeatureExists := false
	autoApproveUserAdded := false
	clusterManagerChanged := false
	if clusterManager.Spec.RegistrationConfiguration != nil {
		registrationConfiguration := clusterManager.Spec.RegistrationConfiguration
		for i, featureGate := range registrationConfiguration.FeatureGates {
			if featureGate.Feature == "ManagedClusterAutoApproval" {
				autoApproveFeatureExists = true
				if featureGate.Mode == operatorv1.FeatureGateModeTypeEnable {
					break
				} else {
					registrationConfiguration.FeatureGates[i].Mode = operatorv1.FeatureGateModeTypeEnable
					clusterManagerChanged = true
					break
				}
			}
		}
		if !autoApproveFeatureExists {
			registrationConfiguration.FeatureGates = append(
				registrationConfiguration.FeatureGates,
				operatorv1.FeatureGate{
					Feature: "ManagedClusterAutoApproval",
					Mode:    operatorv1.FeatureGateModeTypeEnable,
				})
			clusterManagerChanged = true
		}

		for _, user := range registrationConfiguration.AutoApproveUsers {
			if user == autoApproveUser {
				autoApproveUserAdded = true
				break
			}
		}
		if !autoApproveUserAdded {
			registrationConfiguration.AutoApproveUsers = append(
				registrationConfiguration.AutoApproveUsers,
				autoApproveUser)
			clusterManagerChanged = true
		}
	} else {
		clusterManager.Spec.RegistrationConfiguration = &operatorv1.RegistrationHubConfiguration{
			FeatureGates: []operatorv1.FeatureGate{
				{
					Feature: "ManagedClusterAutoApproval",
					Mode:    operatorv1.FeatureGateModeTypeEnable,
				},
			},
			AutoApproveUsers: []string{
				autoApproveUser,
			},
		}
		clusterManagerChanged = true
	}

	// patch cluster-manager only if it has changed
	if clusterManagerChanged {
		if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			// Get the latest ClusterManager to avoid conflicts
			if err := s.client.Get(ctx, types.NamespacedName{Name: ClusterManagerName}, foundClusterManager); err != nil {
				return err
			}
			foundClusterManager.Spec.RegistrationConfiguration = clusterManager.Spec.RegistrationConfiguration
			return s.client.Update(ctx, foundClusterManager)
		}); err != nil {
			log.Errorw("failed to update clusterManager", "error", err)
		}
	}

	return nil
}

func (s *MigrationTargetSyncer) ensureSubjectAccessReviewRole(ctx context.Context, msaName string) error {
	// create or update clusterrole for the migration service account
	subjectAccessReviewClusterRoleName := GetSubjectAccessReviewClusterRoleName(msaName)
	subjectAccessReview := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: subjectAccessReviewClusterRoleName,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"authorization.k8s.io"},
				Resources: []string{"subjectaccessreviews"},
				Verbs:     []string{"create"},
			},
		},
	}

	foundMigrationClusterRole := &rbacv1.ClusterRole{}
	if err := s.client.Get(ctx,
		types.NamespacedName{
			Name: subjectAccessReviewClusterRoleName,
		}, foundMigrationClusterRole); err != nil {
		if apierrors.IsNotFound(err) {
			log.Infof("creating migration clusterrole %s", foundMigrationClusterRole.GetName())
			log.Debugf("creating migration clusterrole %v", foundMigrationClusterRole)
			if err := s.client.Create(ctx, subjectAccessReview); err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		if !apiequality.Semantic.DeepDerivative(subjectAccessReview, foundMigrationClusterRole) {
			log.Infof("updating migration clusterrole %s", subjectAccessReview.GetName())
			log.Debugf("updating migration clusterrole %v", subjectAccessReview)
			if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
				// Get the latest ClusterRole to avoid conflicts
				latestClusterRole := &rbacv1.ClusterRole{}
				if err := s.client.Get(ctx, client.ObjectKeyFromObject(subjectAccessReview), latestClusterRole); err != nil {
					return err
				}
				latestClusterRole.Rules = subjectAccessReview.Rules
				return s.client.Update(ctx, latestClusterRole)
			}); err != nil {
				log.Errorw("failed to update migration ClusterRole", "error", err)
			}
		}
	}

	return nil
}

// getBootstrapClusterRoleName dynamically detects the bootstrap ClusterRole name.
// It first checks for ACM/MCE ClusterRole, then falls back to OCM ClusterRole.
func (s *MigrationTargetSyncer) getBootstrapClusterRoleName(ctx context.Context) (string, error) {
	// Try ACM/MCE ClusterRole first
	cr := &rbacv1.ClusterRole{}
	if err := s.client.Get(ctx, types.NamespacedName{Name: DefaultACMBootstrapClusterRole}, cr); err == nil {
		return DefaultACMBootstrapClusterRole, nil
	}

	// Fallback to OCM ClusterRole
	if err := s.client.Get(ctx, types.NamespacedName{Name: DefaultOCMBootstrapClusterRole}, cr); err == nil {
		return DefaultOCMBootstrapClusterRole, nil
	}

	return "", fmt.Errorf("no bootstrap ClusterRole found (tried %s and %s)",
		DefaultACMBootstrapClusterRole, DefaultOCMBootstrapClusterRole)
}

// isOCMEnvironment checks if running in OCM environment (not ACM/MCE)
// Returns true if only OCM ClusterRole exists, false if ACM ClusterRole exists
func (s *MigrationTargetSyncer) isOCMEnvironment(ctx context.Context) bool {
	cr := &rbacv1.ClusterRole{}
	// If ACM ClusterRole exists, it's not OCM environment
	if err := s.client.Get(ctx, types.NamespacedName{Name: DefaultACMBootstrapClusterRole}, cr); err == nil {
		return false
	}
	// If only OCM ClusterRole exists, it's OCM environment
	if err := s.client.Get(ctx, types.NamespacedName{Name: DefaultOCMBootstrapClusterRole}, cr); err == nil {
		return true
	}
	return false
}

func (s *MigrationTargetSyncer) ensureRegistrationClusterRoleBinding(ctx context.Context,
	msaName, msaNamespace string,
) error {
	registrationClusterRoleName, err := s.getBootstrapClusterRoleName(ctx)
	if err != nil {
		return fmt.Errorf("failed to get bootstrap ClusterRole name: %w", err)
	}
	log.Infof("using bootstrap ClusterRole: %s", registrationClusterRoleName)
	registrationClusterRoleBindingName := GetAgentRegistrationClusterRoleBindingName(msaName)
	registrationClusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: registrationClusterRoleBindingName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      msaName,
				Namespace: msaNamespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			Name:     registrationClusterRoleName,
			APIGroup: "rbac.authorization.k8s.io",
		},
	}

	foundRegistrationClusterRoleBinding := &rbacv1.ClusterRoleBinding{}
	if err := s.client.Get(ctx,
		types.NamespacedName{
			Name: registrationClusterRoleBindingName,
		}, foundRegistrationClusterRoleBinding); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("creating agent registration clusterrolebinding",
				"clusterrolebinding", registrationClusterRoleBindingName)
			if err := s.client.Create(ctx, registrationClusterRoleBinding); err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		if !apiequality.Semantic.DeepDerivative(registrationClusterRoleBinding, foundRegistrationClusterRoleBinding) {
			log.Info("updating agent registration clusterrolebinding",
				"clusterrolebinding", registrationClusterRoleBindingName)
			if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
				// Get the latest ClusterRoleBinding to avoid conflicts
				latestClusterRoleBinding := &rbacv1.ClusterRoleBinding{}
				if err := s.client.Get(ctx, client.ObjectKeyFromObject(registrationClusterRoleBinding),
					latestClusterRoleBinding); err != nil {
					return err
				}
				latestClusterRoleBinding.Subjects = registrationClusterRoleBinding.Subjects
				latestClusterRoleBinding.RoleRef = registrationClusterRoleBinding.RoleRef
				return s.client.Update(ctx, latestClusterRoleBinding)
			}); err != nil {
				log.Errorw("failed to update migration ClusterRoleBinding", "error", err)
			}
		}
	}

	return nil
}

func (s *MigrationTargetSyncer) ensureSubjectAccessReviewRoleBinding(ctx context.Context,
	msaName, msaNamespace string,
) error {
	accessReviewClusterRoleName := GetSubjectAccessReviewClusterRoleName(msaName)
	accessReviewClusterRoleBindingName := GetSubjectAccessReviewClusterRoleBindingName(msaName)
	accessReviewClusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: accessReviewClusterRoleBindingName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      msaName,
				Namespace: msaNamespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			Name:     accessReviewClusterRoleName,
			APIGroup: "rbac.authorization.k8s.io",
		},
	}

	foundAccessReviewClusterRoleBinding := &rbacv1.ClusterRoleBinding{}
	if err := s.client.Get(ctx, client.ObjectKeyFromObject(accessReviewClusterRoleBinding),
		foundAccessReviewClusterRoleBinding); err != nil {
		if apierrors.IsNotFound(err) {
			log.Infof("creating subjectaccessreviews clusterrolebinding %s",
				foundAccessReviewClusterRoleBinding.GetName())
			log.Debugf("creating subjectaccessreviews clusterrolebinding %v",
				foundAccessReviewClusterRoleBinding)
			if err := s.client.Create(ctx, accessReviewClusterRoleBinding); err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		if !apiequality.Semantic.DeepDerivative(accessReviewClusterRoleBinding, foundAccessReviewClusterRoleBinding) {
			log.Infof("updating subjectaccessreviews clusterrolebinding %v",
				accessReviewClusterRoleBinding.GetName())
			log.Debugf("updating subjectaccessreviews clusterrolebinding %v",
				accessReviewClusterRoleBinding)
			if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
				// Get the latest ClusterRoleBinding to avoid conflicts
				latestClusterRoleBinding := &rbacv1.ClusterRoleBinding{}
				if err := s.client.Get(ctx, client.ObjectKeyFromObject(accessReviewClusterRoleBinding),
					latestClusterRoleBinding); err != nil {
					return err
				}
				latestClusterRoleBinding.Subjects = accessReviewClusterRoleBinding.Subjects
				latestClusterRoleBinding.RoleRef = accessReviewClusterRoleBinding.RoleRef
				return s.client.Update(ctx, latestClusterRoleBinding)
			}); err != nil {
				log.Errorw("failed to update subjectaccessreviews ClusterRoleBinding", "error", err)
			}

		}
	}

	return nil
}

func GetSubjectAccessReviewClusterRoleName(managedServiceAccountName string) string {
	return fmt.Sprintf("global-hub-migration-%s-sar", managedServiceAccountName)
}

func GetSubjectAccessReviewClusterRoleBindingName(managedServiceAccountName string) string {
	return fmt.Sprintf("global-hub-migration-%s-sar", managedServiceAccountName)
}

func GetAgentRegistrationClusterRoleBindingName(managedServiceAccountName string) string {
	return fmt.Sprintf("global-hub-migration-%s-registration", managedServiceAccountName)
}

// removeAutoApproveUser removes the managed service account user from ClusterManager AutoApproveUsers list
func (s *MigrationTargetSyncer) removeAutoApproveUser(ctx context.Context, saName, saNamespace string) error {
	foundClusterManager := &operatorv1.ClusterManager{}
	if err := s.client.Get(ctx, types.NamespacedName{Name: ClusterManagerName}, foundClusterManager); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("cluster-manager not found, skipping auto approve user removal")
			return nil
		}
		return fmt.Errorf("failed to get cluster-manager: %w", err)
	}

	clusterManager := foundClusterManager.DeepCopy()
	autoApproveUser := fmt.Sprintf("system:serviceaccount:%s:%s", saNamespace, saName)
	clusterManagerChanged := false

	// Remove the user from AutoApproveUsers list
	newAutoApproveUsers := []string{}
	if clusterManager.Spec.RegistrationConfiguration != nil {
		registrationConfiguration := clusterManager.Spec.RegistrationConfiguration

		for _, user := range registrationConfiguration.AutoApproveUsers {
			if user != autoApproveUser {
				newAutoApproveUsers = append(newAutoApproveUsers, user)
			} else {
				clusterManagerChanged = true
				log.Infof("removing auto approve user: %s", autoApproveUser)
			}
		}
	}

	// Update cluster-manager only if it has changed
	if clusterManagerChanged {
		if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			// Get the latest ClusterManager to avoid conflicts
			if err := s.client.Get(ctx, types.NamespacedName{Name: ClusterManagerName}, clusterManager); err != nil {
				return err
			}
			clusterManager.Spec.RegistrationConfiguration.AutoApproveUsers = newAutoApproveUsers
			return s.client.Update(ctx, clusterManager)
		}); err != nil {
			return fmt.Errorf("failed to update ClusterManager to remove auto approve user: %w", err)
		}
		log.Infof("successfully removed auto approve user %s from ClusterManager", autoApproveUser)
	} else {
		log.Infof("auto approve user %s not found in ClusterManager, no removal needed", autoApproveUser)
	}

	return nil
}

// rollbacking handles rollback operations for different stages on the target hub
func (s *MigrationTargetSyncer) rollbacking(ctx context.Context,
	target *migration.MigrationTargetBundle,
	clusterErrors map[string]string,
) error {
	log.Infof("performing rollback for stage: %s", target.RollbackStage)
	switch target.RollbackStage {
	case migrationv1alpha1.PhaseInitializing:
		return s.rollbackInitializing(ctx, target)
	case migrationv1alpha1.PhaseDeploying:
		return s.rollbackDeploying(ctx, target)
	case migrationv1alpha1.PhaseRegistering:
		return s.rollbackRegistering(ctx, target)
	default:
		return fmt.Errorf("no specific rollback action needed for stage: %s", target.RollbackStage)
	}
}

// rollbackInitializing handles rollback of initializing phase on target hub
func (s *MigrationTargetSyncer) rollbackInitializing(ctx context.Context, spec *migration.MigrationTargetBundle) error {
	// For initializing rollback on target hub, we need to:
	// 1. Remove the managed service account user from ClusterManager AutoApproveUsers list
	// 2. Clean up RBAC resources created for the managed service account
	log.Infof("rollback initializing stage for managed service account: %s", spec.ManagedServiceAccountName)
	if spec.ManagedServiceAccountName == "" {
		log.Info("no managed service account name provided, skipping rollback cleanup")
		return nil
	}

	// Whether to set the autoapprove feature gate available?

	// Remove MSA user from ClusterManager AutoApproveUsers list
	if err := s.removeAutoApproveUser(ctx, spec.ManagedServiceAccountName,
		spec.ManagedServiceAccountInstallNamespace); err != nil {
		log.Errorf("failed to remove auto approve user from ClusterManager: %v", err)
		return err
	}

	// Clean up RBAC resources
	return s.cleanupMigrationRBAC(ctx, spec.ManagedServiceAccountName)
}

// rollbackDeploying handles rollback of deploying phase on target hub
// This is the main rollback operation that removes all migration resources and clusters
func (s *MigrationTargetSyncer) rollbackDeploying(ctx context.Context, spec *migration.MigrationTargetBundle) error {
	log.Infof("rollback deploying stage for clusters: %v", spec.ManagedClusters)

	// 1. Remove all migration resources (including ManagedClusters and KlusterletAddonConfigs)
	for _, clusterName := range spec.ManagedClusters {
		if err := s.removeMigrationResources(ctx, clusterName); err != nil {
			log.Errorf("failed to remove migration resources for cluster %s: %v", clusterName, err)
			return fmt.Errorf("failed to remove migration resources for cluster %s: %v", clusterName, err)
		}
	}

	// 2. Remove cluster namespace
	for _, clusterName := range spec.ManagedClusters {
		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: clusterName,
			},
		}
		if err := deleteResourceIfExists(ctx, s.client, namespace, false); err != nil {
			log.Warnf("failed to remove namespace %s: %v", clusterName, err)
		}
	}

	// 3. Roll back initializing
	if err := s.rollbackInitializing(ctx, spec); err != nil {
		return fmt.Errorf("failed to rollback initializing stage: %v", err)
	}

	log.Info("completed deploying stage rollback")
	return nil
}

// rollbackRegistering handles rollback of registering phase on target hub
func (s *MigrationTargetSyncer) rollbackRegistering(ctx context.Context, spec *migration.MigrationTargetBundle) error {
	log.Infof("rollback registering stage for clusters: %v", spec.ManagedClusters)
	return s.rollbackDeploying(ctx, spec)
}

// addPauseAnnotationBeforeDeletion adds pause annotation to ZTP resources and verifies it was added
func (s *MigrationTargetSyncer) addPauseAnnotationBeforeDeletion(
	ctx context.Context, resource *unstructured.Unstructured,
) error {
	resourceKey := fmt.Sprintf("%s/%s/%s", resource.GetKind(), resource.GetNamespace(), resource.GetName())
	log.Infof("adding pause annotation to %s before deletion", resourceKey)

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Get the latest version to avoid conflicts
		latestResource := &unstructured.Unstructured{}
		latestResource.SetGroupVersionKind(resource.GroupVersionKind())
		if err := s.client.Get(ctx, client.ObjectKeyFromObject(resource), latestResource); err != nil {
			return fmt.Errorf("failed to get latest resource: %w", err)
		}

		// Add pause annotation
		annotations := latestResource.GetAnnotations()
		if annotations == nil {
			annotations = make(map[string]string)
		}
		annotations[PauseAnnotation] = "true"
		latestResource.SetAnnotations(annotations)

		// Update the resource
		if err := s.client.Update(ctx, latestResource); err != nil {
			return err
		}

		// Copy back to the original resource for later use
		resource.SetAnnotations(annotations)
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to add pause annotation: %w", err)
	}

	// Verify the annotation was added
	verifyResource := &unstructured.Unstructured{}
	verifyResource.SetGroupVersionKind(resource.GroupVersionKind())
	if err := s.client.Get(ctx, client.ObjectKeyFromObject(resource), verifyResource); err != nil {
		return fmt.Errorf("failed to verify pause annotation: %w", err)
	}

	annotations := verifyResource.GetAnnotations()
	if annotations == nil || annotations[PauseAnnotation] != "true" {
		return fmt.Errorf("pause annotation verification failed for %s", resourceKey)
	}

	log.Infof("successfully added and verified pause annotation for %s", resourceKey)
	return nil
}

// removeMigrationResources removes all migration resources for a cluster based on the migrateResources list
// Deletes all resources defined in resources.go from the cluster namespace
// Uses collectMigrationResources to collect resources consistently
func (s *MigrationTargetSyncer) removeMigrationResources(ctx context.Context, clusterName string) error {
	log.Infof("removing all migration resources for cluster namespace: %s", clusterName)

	// Collect all migration resources for this cluster
	// Use no-op functions for cleaning since we're just deleting the resources
	resources, err := collectMigrationResources(
		ctx, s.client, clusterName, migrateResources,
		func(obj client.Object) {}, // no-op clean function
		func(resource *unstructured.Unstructured, migrateResource MigrationResource) {}, // no-op process function
	)
	if err != nil {
		return fmt.Errorf("failed to collect migration resources for deletion: %w", err)
	}

	if len(resources) == 0 {
		log.Infof("no migration resources found in cluster namespace: %s", clusterName)
		return nil
	}

	log.Infof("found %d total migration resources to delete in namespace %s", len(resources), clusterName)

	// Delete all collected resources
	for _, obj := range resources {
		objCopy := obj

		// For ClusterDeployment,
		// add pause annotation and remove deprovision finalizers before deletion
		if objCopy.GetKind() == "ClusterDeployment" {
			if err := s.addPauseAnnotationBeforeDeletion(ctx, &objCopy); err != nil {
				log.Errorf("failed to add pause annotation to %s/%s of kind %s: %v",
					objCopy.GetNamespace(), objCopy.GetName(), objCopy.GetKind(), err)
				return fmt.Errorf("failed to add pause annotation to %s/%s: %w",
					objCopy.GetNamespace(), objCopy.GetName(), err)
			}

			// Remove deprovision finalizers to prevent blocking deletion
			if err := removeDeprovisionFinalizers(ctx, s.client, &objCopy); err != nil {
				log.Errorf("failed to remove deprovision finalizers from %s/%s of kind %s: %v",
					objCopy.GetNamespace(), objCopy.GetName(), objCopy.GetKind(), err)
				return fmt.Errorf("failed to remove deprovision finalizers from %s/%s: %w",
					objCopy.GetNamespace(), objCopy.GetName(), err)
			}
		}

		if err := s.client.Delete(ctx, &objCopy); err != nil {
			if !apierrors.IsNotFound(err) {
				return fmt.Errorf("failed to delete resource: %w", err)
			}
			log.Debugf("resource %s/%s not found during deletion, may have been already deleted",
				obj.GetNamespace(), obj.GetName())
		}

		log.Infof("successfully removed resource %s/%s of kind %s",
			objCopy.GetNamespace(), objCopy.GetName(), objCopy.GetKind())
	}

	log.Infof("completed removing all %d migration resources from cluster namespace: %s", len(resources), clusterName)
	return nil
}

// cleanupMigrationRBAC removes RBAC resources created for the managed service account
func (s *MigrationTargetSyncer) cleanupMigrationRBAC(ctx context.Context, managedServiceAccountName string) error {
	// Remove ClusterRole for SubjectAccessReview: global-hub-migration-<msa-name>-sar
	clusterRoleName := GetSubjectAccessReviewClusterRoleName(managedServiceAccountName)
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterRoleName,
		},
	}
	if err := deleteResourceIfExists(ctx, s.client, clusterRole, false); err != nil {
		log.Errorf("failed to delete cluster role %s: %v", clusterRoleName, err)
		return err
	}

	// Remove ClusterRoleBinding for SubjectAccessReview: global-hub-migration-<msa-name>-sar
	sarClusterRoleBindingName := GetSubjectAccessReviewClusterRoleBindingName(managedServiceAccountName)
	sarClusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: sarClusterRoleBindingName,
		},
	}
	if err := deleteResourceIfExists(ctx, s.client, sarClusterRoleBinding, false); err != nil {
		log.Errorf("failed to delete subjectaccessreviews cluster role binding %s: %v", sarClusterRoleBindingName, err)
		return err
	}

	// Remove ClusterRoleBinding for Agent Registration: global-hub-migration-<msa-name>-registration
	registrationClusterRoleBindingName := GetAgentRegistrationClusterRoleBindingName(managedServiceAccountName)
	registrationClusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: registrationClusterRoleBindingName,
		},
	}
	if err := deleteResourceIfExists(ctx, s.client, registrationClusterRoleBinding, false); err != nil {
		log.Errorf("failed to delete agent registration cluster role binding %s: %v", registrationClusterRoleBindingName, err)
		return err
	}

	return nil
}

// checkClusterManifestWork checks if a cluster's ManifestWork is ready
func (s *MigrationTargetSyncer) checkClusterManifestWork(ctx context.Context, clusterName string) error {
	// ManifestWork name pattern: <cluster-name>-klusterlet
	workName := fmt.Sprintf("%s%s", clusterName, KlusterletManifestWorkSuffix)
	work := &workv1.ManifestWork{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: clusterName,
			Name:      workName,
		},
	}

	if err := s.client.Get(ctx, client.ObjectKeyFromObject(work), work); err != nil {
		return fmt.Errorf("failed to get manifestwork %s: %w", workName, err)
	}

	if !meta.IsStatusConditionTrue(work.Status.Conditions, workv1.WorkApplied) {
		return fmt.Errorf("manifestwork %s is not applied", workName)
	}

	return nil
}

// deleteResourceIfExists deletes a resource if it exists, ignoring NotFound errors
// If forceDelete is true, removes all finalizers before deletion
func deleteResourceIfExists(ctx context.Context, c client.Client, obj client.Object, forceDelete bool) error {
	if obj == nil {
		return nil
	}

	if err := c.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
		if apierrors.IsNotFound(err) {
			log.Debugf("resource %s/%s already deleted", obj.GetNamespace(), obj.GetName())
			return nil
		}
		return fmt.Errorf("failed to get resource: %w", err)
	}

	// Remove finalizers if forceDelete is enabled
	if forceDelete && len(obj.GetFinalizers()) != 0 {
		if err := removeFinalizers(ctx, c, obj); err != nil {
			log.Errorf("failed to remove finalizers from %s/%s: %v", obj.GetNamespace(), obj.GetName(), err)
			return fmt.Errorf("failed to remove finalizers: %w", err)
		}
	}

	log.Infof("deleting resource %s/%s", obj.GetNamespace(), obj.GetName())
	if err := c.Delete(ctx, obj); err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete resource: %w", err)
		}
		log.Debugf("resource %s/%s not found during deletion, may have been already deleted",
			obj.GetNamespace(), obj.GetName())
	}

	return nil
}

// SetMigrationID sets the processing migration ID for testing purposes
func (s *MigrationTargetSyncer) SetMigrationID(migrationID string) {
	s.processingMigrationId = migrationID
}
