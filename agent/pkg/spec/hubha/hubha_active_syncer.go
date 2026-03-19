// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubha

import (
	"context"
	"fmt"
	"time"

	cecontext "github.com/cloudevents/sdk-go/v2/context"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var (
	log            = logger.DefaultZapLogger()
	resourceFilter = utils.NewHubHAResourceFilter()
)

const (
	// Send Hub HA bundles every 30 seconds
	hubhaSyncInterval = 30 * time.Second
)

// HubHAActiveSyncer periodically collects Hub HA resources and sends them to standby hub via spec topic
type HubHAActiveSyncer struct {
	client          client.Client
	producer        transport.Producer
	transportConfig *transport.TransportInternalConfig
	resourcesToSync []schema.GroupVersionKind
	activeHubName   string
	standbyHubName  string
}

// StartHubHAActiveSyncer starts the active hub syncer (only on active hubs)
func StartHubHAActiveSyncer(ctx context.Context, mgr ctrl.Manager, producer transport.Producer) error {
	// Only start if this is an active hub with a configured standby
	agentConfig := configs.GetAgentConfig()
	if agentConfig == nil {
		log.Info("Hub HA active syncer not started - agent config is nil")
		return nil
	}

	// Get hub role and standby hub atomically
	hubRole, standbyHub := agentConfig.GetHubRoleAndStandbyHub()
	if hubRole != constants.GHHubRoleActive {
		log.Info("Hub HA active syncer not started - this is not an active hub")
		return nil
	}

	if standbyHub == "" {
		log.Warn("Hub HA active syncer not started - no standby hub configured")
		return nil
	}

	syncer := &HubHAActiveSyncer{
		client:          mgr.GetClient(),
		producer:        producer,
		transportConfig: agentConfig.TransportConfig,
		activeHubName:   agentConfig.LeafHubName,
		standbyHubName:  standbyHub,
	}

	log.Infof("starting Hub HA active syncer: %s (active) -> %s (standby)",
		syncer.activeHubName, syncer.standbyHubName)

	// Initialize resources to sync
	if err := syncer.discoverResources(); err != nil {
		return fmt.Errorf("failed to initialize Hub HA resources: %w", err)
	}

	// Start periodic sync goroutine
	go syncer.periodicSync(ctx)

	return nil
}

func (s *HubHAActiveSyncer) discoverResources() error {
	// Use hardcoded list of resources to sync (not all hubs have all CRDs installed)
	s.resourcesToSync = getHubHAResourcesToSync()
	log.Infof("configured %d resource types to sync to standby hub", len(s.resourcesToSync))
	return nil
}

// getHubHAResourcesToSync returns the list of resources to sync for Hub HA
func getHubHAResourcesToSync() []schema.GroupVersionKind {
	return []schema.GroupVersionKind{
		// ACM/OCM Addon resources
		{Group: "addon.open-cluster-management.io", Version: "v1beta1", Kind: "AddOnDeploymentConfig"},
		{Group: "addon.open-cluster-management.io", Version: "v1alpha1", Kind: "AddOnTemplate"},
		{Group: "addon.open-cluster-management.io", Version: "v1beta1", Kind: "ClusterManagementAddOn"},
		{Group: "addon.open-cluster-management.io", Version: "v1beta1", Kind: "ManagedClusterAddOn"},

		// ACM/OCM Agent resources
		{Group: "agent.open-cluster-management.io", Version: "v1", Kind: "KlusterletAddonConfig"},

		// ACM/OCM Application resources
		{Group: "apps.open-cluster-management.io", Version: "v1", Kind: "Channel"},
		{Group: "apps.open-cluster-management.io", Version: "v1", Kind: "Deployable"},
		{Group: "apps.open-cluster-management.io", Version: "v1beta1", Kind: "GitOpsCluster"},
		{Group: "apps.open-cluster-management.io", Version: "v1", Kind: "HelmRelease"},
		{Group: "apps.open-cluster-management.io", Version: "v1alpha1", Kind: "MulticlusterApplicationSetReport"},
		{Group: "apps.open-cluster-management.io", Version: "v1", Kind: "PlacementRule"},
		{Group: "apps.open-cluster-management.io", Version: "v1alpha1", Kind: "SubscriptionReport"},
		{Group: "apps.open-cluster-management.io", Version: "v1", Kind: "Subscription"},
		{Group: "apps.open-cluster-management.io", Version: "v1alpha1", Kind: "SubscriptionStatus"},

		// ACM/OCM Authentication resources
		{Group: "authentication.open-cluster-management.io", Version: "v1beta1", Kind: "ManagedServiceAccount"},

		// ACM/OCM Cluster resources
		{Group: "cluster.open-cluster-management.io", Version: "v1alpha1", Kind: "AddOnPlacementScore"},
		{Group: "cluster.open-cluster-management.io", Version: "v1beta1", Kind: "BackupSchedule"},
		{Group: "cluster.open-cluster-management.io", Version: "v1alpha1", Kind: "ClusterClaim"},
		{Group: "cluster.open-cluster-management.io", Version: "v1beta1", Kind: "ClusterCurator"},
		{Group: "cluster.open-cluster-management.io", Version: "v1", Kind: "ManagedCluster"},
		{Group: "cluster.open-cluster-management.io", Version: "v1beta2", Kind: "ManagedClusterSetBinding"},
		{Group: "cluster.open-cluster-management.io", Version: "v1beta2", Kind: "ManagedClusterSet"},
		{Group: "cluster.open-cluster-management.io", Version: "v1beta1", Kind: "PlacementDecision"},
		{Group: "cluster.open-cluster-management.io", Version: "v1beta1", Kind: "Placement"},
		{Group: "cluster.open-cluster-management.io", Version: "v1beta1", Kind: "Restore"},

		// ACM/OCM Config resources
		{Group: "config.open-cluster-management.io", Version: "v1alpha1", Kind: "KlusterletConfig"},

		// ACM/OCM Console resources
		{Group: "console.open-cluster-management.io", Version: "v1", Kind: "UserPreference"},

		// ACM/OCM Discovery resources
		{Group: "discovery.open-cluster-management.io", Version: "v1", Kind: "DiscoveredCluster"},
		{Group: "discovery.open-cluster-management.io", Version: "v1", Kind: "DiscoveryConfig"},

		// Global Hub resources
		{Group: "global-hub.open-cluster-management.io", Version: "v1alpha1", Kind: "ManagedClusterMigration"},

		// ACM/OCM Image Registry resources
		{Group: "imageregistry.open-cluster-management.io", Version: "v1alpha1", Kind: "ManagedClusterImageRegistry"},

		// ACM/OCM Observability resources
		{Group: "observability.open-cluster-management.io", Version: "v1beta2", Kind: "MultiClusterObservability"},
		{Group: "observability.open-cluster-management.io", Version: "v1beta1", Kind: "ObservabilityAddon"},

		// ACM/OCM Policy resources
		{Group: "policy.open-cluster-management.io", Version: "v1", Kind: "CertificatePolicy"},
		{Group: "policy.open-cluster-management.io", Version: "v1", Kind: "ConfigurationPolicy"},
		{Group: "policy.open-cluster-management.io", Version: "v1beta1", Kind: "OperatorPolicy"},
		{Group: "policy.open-cluster-management.io", Version: "v1", Kind: "PlacementBinding"},
		{Group: "policy.open-cluster-management.io", Version: "v1", Kind: "Policy"},
		{Group: "policy.open-cluster-management.io", Version: "v1beta1", Kind: "PolicyAutomation"},
		{Group: "policy.open-cluster-management.io", Version: "v1beta1", Kind: "PolicySet"},

		// ACM/OCM RBAC resources
		{Group: "rbac.open-cluster-management.io", Version: "v1alpha1", Kind: "ClusterPermission"},
		{Group: "rbac.open-cluster-management.io", Version: "v1beta1", Kind: "MulticlusterRoleAssignment"},

		// ACM/OCM Site Config resources
		{Group: "siteconfig.open-cluster-management.io", Version: "v1alpha1", Kind: "ClusterInstance"},

		// ACM/OCM Submariner resources
		{Group: "submarineraddon.open-cluster-management.io", Version: "v1alpha1", Kind: "SubmarinerConfig"},
		{Group: "submarineraddon.open-cluster-management.io", Version: "v1alpha1", Kind: "SubmarinerDiagnoseConfig"},

		// Hive extension resources
		{Group: "extensions.hive.openshift.io", Version: "v1beta1", Kind: "AgentClusterInstall"},
		{Group: "extensions.hive.openshift.io", Version: "v1alpha1", Kind: "ImageClusterInstall"},

		// Hive resources
		{Group: "hive.openshift.io", Version: "v1", Kind: "Checkpoint"},
		{Group: "hive.openshift.io", Version: "v1", Kind: "ClusterClaim"},
		{Group: "hive.openshift.io", Version: "v1", Kind: "ClusterDeploymentCustomization"},
		{Group: "hive.openshift.io", Version: "v1", Kind: "ClusterDeployment"},
		{Group: "hive.openshift.io", Version: "v1", Kind: "ClusterDeprovision"},
		{Group: "hive.openshift.io", Version: "v1", Kind: "ClusterImageSet"},
		{Group: "hive.openshift.io", Version: "v1", Kind: "ClusterPool"},
		{Group: "hive.openshift.io", Version: "v1", Kind: "ClusterProvision"},
		{Group: "hive.openshift.io", Version: "v1", Kind: "ClusterRelocate"},
		{Group: "hive.openshift.io", Version: "v1", Kind: "ClusterState"},
		{Group: "hive.openshift.io", Version: "v1", Kind: "DNSZone"},
		{Group: "hive.openshift.io", Version: "v1", Kind: "HiveConfig"},
		{Group: "hive.openshift.io", Version: "v1", Kind: "MachinePoolNameLease"},
		{Group: "hive.openshift.io", Version: "v1", Kind: "MachinePool"},
		{Group: "hive.openshift.io", Version: "v1", Kind: "SelectorSyncIdentityProvider"},
		{Group: "hive.openshift.io", Version: "v1", Kind: "SelectorSyncSet"},
		{Group: "hive.openshift.io", Version: "v1", Kind: "SyncIdentityProvider"},
		{Group: "hive.openshift.io", Version: "v1", Kind: "SyncSet"},

		// Hive internal resources
		{Group: "hiveinternal.openshift.io", Version: "v1alpha1", Kind: "ClusterSyncLease"},
		{Group: "hiveinternal.openshift.io", Version: "v1alpha1", Kind: "ClusterSync"},
		{Group: "hiveinternal.openshift.io", Version: "v1alpha1", Kind: "FakeClusterInstall"},

		// ArgoCD Application resources
		{Group: "argoproj.io", Version: "v1alpha1", Kind: "Application"},
		{Group: "argoproj.io", Version: "v1alpha1", Kind: "ApplicationSet"},
		{Group: "argoproj.io", Version: "v1alpha1", Kind: "AppProject"},
		{Group: "argoproj.io", Version: "v1alpha1", Kind: "ArgoCD"},

		// ArgoCD Workflow resources
		{Group: "argoproj.io", Version: "v1alpha1", Kind: "Workflow"},
		{Group: "argoproj.io", Version: "v1alpha1", Kind: "WorkflowTemplate"},
		{Group: "argoproj.io", Version: "v1alpha1", Kind: "ClusterWorkflowTemplate"},
		{Group: "argoproj.io", Version: "v1alpha1", Kind: "CronWorkflow"},

		// ArgoCD Events resources
		{Group: "argoproj.io", Version: "v1alpha1", Kind: "EventSource"},
		{Group: "argoproj.io", Version: "v1alpha1", Kind: "Sensor"},
		{Group: "argoproj.io", Version: "v1alpha1", Kind: "EventBus"},

		// ArgoCD Rollouts resources
		{Group: "argoproj.io", Version: "v1alpha1", Kind: "Rollout"},
		{Group: "argoproj.io", Version: "v1alpha1", Kind: "Experiment"},
		{Group: "argoproj.io", Version: "v1alpha1", Kind: "AnalysisTemplate"},
		{Group: "argoproj.io", Version: "v1alpha1", Kind: "ClusterAnalysisTemplate"},

		// Application resources
		{Group: "app.k8s.io", Version: "v1beta1", Kind: "Application"},

		// Observatorium resources
		{Group: "core.observatorium.io", Version: "v1alpha1", Kind: "Observatorium"},

		// Agent install resources (ZTP)
		{Group: "agent-install.openshift.io", Version: "v1beta1", Kind: "AgentClassification"},
		{Group: "agent-install.openshift.io", Version: "v1beta1", Kind: "Agent"},
		{Group: "agent-install.openshift.io", Version: "v1beta1", Kind: "AgentServiceConfig"},
		{Group: "agent-install.openshift.io", Version: "v1beta1", Kind: "HypershiftAgentServiceConfig"},
		{Group: "agent-install.openshift.io", Version: "v1beta1", Kind: "InfraEnv"},
		{Group: "agent-install.openshift.io", Version: "v1beta1", Kind: "NMStateConfig"},

		// CAPI Provider resources
		{Group: "capi-provider.agent-install.openshift.io", Version: "v1beta1", Kind: "AgentCluster"},
		{Group: "capi-provider.agent-install.openshift.io", Version: "v1beta1", Kind: "AgentMachine"},
		{Group: "capi-provider.agent-install.openshift.io", Version: "v1beta1", Kind: "AgentMachineTemplate"},

		// Secrets and ConfigMaps (will be filtered by labels)
		{Group: "", Version: "v1", Kind: "Secret"},
		{Group: "", Version: "v1", Kind: "ConfigMap"},
	}
}

func (s *HubHAActiveSyncer) periodicSync(ctx context.Context) {
	ticker := time.NewTicker(hubhaSyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("Hub HA active syncer stopped")
			return
		case <-ticker.C:
			if err := s.syncResources(ctx); err != nil {
				log.Errorf("failed to sync Hub HA resources to standby hub: %v", err)
			}
		}
	}
}

func (s *HubHAActiveSyncer) syncResources(ctx context.Context) error {
	bundle := generic.NewGenericBundle[*unstructured.Unstructured]()
	resourceCount := 0
	var firstListErr error

	for _, gvk := range s.resourcesToSync {
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   gvk.Group,
			Version: gvk.Version,
			Kind:    gvk.Kind + "List",
		})

		if err := s.client.List(ctx, list); err != nil {
			// If CRD doesn't exist (expected for some resources), log at debug level
			if meta.IsNoMatchError(err) {
				log.Debugf("CRD not installed for %s, skipping", gvk.String())
			} else {
				// Real error (permissions, API server issue, etc.)
				log.Warnf("failed to list resources for %s: %v", gvk.String(), err)
				if firstListErr == nil {
					firstListErr = fmt.Errorf("failed to list %s: %w", gvk.String(), err)
				}
			}
			continue
		}

		for i := range list.Items {
			obj := &list.Items[i]

			// Filter using resource filter
			if !resourceFilter.ShouldSyncResource(obj, gvk) {
				continue
			}

			// Clean metadata
			obj.SetManagedFields(nil)
			obj.SetResourceVersion("")
			obj.SetGeneration(0)
			obj.SetUID("")

			// Add to bundle as "resync" (full state sync)
			bundle.Resync = append(bundle.Resync, obj)
			resourceCount++
		}
	}

	if firstListErr != nil {
		return firstListErr
	}

	if resourceCount == 0 {
		log.Debug("no Hub HA resources to sync to standby hub")
		return nil
	}

	// Send bundle to standby hub via spec topic
	if err := s.sendBundle(ctx, bundle); err != nil {
		return fmt.Errorf("failed to send Hub HA bundle: %w", err)
	}

	log.Infof("synced %d Hub HA resources to standby hub %s", resourceCount, s.standbyHubName)
	return nil
}

func (s *HubHAActiveSyncer) sendBundle(ctx context.Context, bundle *generic.GenericBundle[*unstructured.Unstructured]) error {
	// Create CloudEvent with proper routing
	evt := utils.ToCloudEvent(
		constants.HubHAResourcesMsgKey,
		s.activeHubName,  // source = active hub
		s.standbyHubName, // clustername extension = standby hub
		bundle,
	)

	// Send to spec topic
	topicCtx := cecontext.WithTopic(ctx, s.transportConfig.KafkaCredential.SpecTopic)
	if err := s.producer.SendEvent(topicCtx, evt); err != nil {
		return fmt.Errorf("failed to send Hub HA bundle from %s to %s: %w",
			s.activeHubName, s.standbyHubName, err)
	}

	log.Debugf("sent Hub HA bundle from %s to %s via spec topic",
		s.activeHubName, s.standbyHubName)
	return nil
}
