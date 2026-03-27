// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubha

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var log = logger.DefaultZapLogger()

const (
	// Full resync every 30 minutes as a safety net for consistency
	// This handles edge cases like missed events during network issues or agent restarts
	hubhaResyncInterval = 30 * time.Minute
)

// hubHAController implements reconciliation for Hub HA resources using list-watch pattern
type hubHAController struct {
	client  client.Client
	emitter *HubHAEmitter
	gvk     schema.GroupVersionKind
}

// StartHubHAActiveSyncer starts the active hub syncer using controller-based list-watch pattern
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

	log.Infof("starting Hub HA active syncer with list-watch pattern: %s (active) -> %s (standby)",
		agentConfig.LeafHubName, standbyHub)

	// Create shared emitter for all Hub HA resources
	emitter := NewHubHAEmitter(
		producer,
		agentConfig.TransportConfig,
		agentConfig.LeafHubName,
		standbyHub,
	)

	// Start controllers for each GVK
	resourcesToSync := getHubHAResourcesToSync()
	for _, gvk := range resourcesToSync {
		if err := startHubHAController(mgr, gvk, emitter); err != nil {
			// Log error but don't fail - some CRDs might not be installed
			log.Debugf("skipped controller for %s: %v", gvk.String(), err)
		}
	}

	// Start periodic resync as a safety net for consistency (handles edge cases like missed events)
	go periodicResync(ctx, mgr.GetClient(), emitter, resourcesToSync, hubhaResyncInterval)

	return nil
}

// startHubHAController starts a controller for a specific GVK
func startHubHAController(mgr ctrl.Manager, gvk schema.GroupVersionKind, emitter *HubHAEmitter) error {
	// Create unstructured object instance for this GVK
	instance := &unstructured.Unstructured{}
	instance.SetGroupVersionKind(gvk)

	// Create controller
	controller := &hubHAController{
		client:  mgr.GetClient(),
		emitter: emitter,
		gvk:     gvk,
	}

	// Build controller with manager
	if err := ctrl.NewControllerManagedBy(mgr).
		For(instance).
		WithEventFilter(emitter.Predicate()).
		Named(fmt.Sprintf("hubha-%s", gvk.Kind)).
		Complete(controller); err != nil {
		return err
	}

	log.Debugf("started Hub HA controller for %s", gvk.String())
	return nil
}

// Reconcile handles changes to Hub HA resources
func (c *hubHAController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(c.gvk)

	err := c.client.Get(ctx, req.NamespacedName, obj)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			// Object was deleted
			obj.SetNamespace(req.Namespace)
			obj.SetName(req.Name)
			if err := c.emitter.Delete(obj); err != nil {
				log.Errorf("failed to handle delete for %s/%s (%s): %v",
					req.Namespace, req.Name, c.gvk.Kind, err)
				return ctrl.Result{RequeueAfter: 5 * time.Second}, err
			}
			return ctrl.Result{}, nil
		}
		log.Errorf("failed to get object %s/%s (%s): %v", req.Namespace, req.Name, c.gvk.Kind, err)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, err
	}

	// Object exists - update or delete based on deletion timestamp
	if !obj.GetDeletionTimestamp().IsZero() {
		if err := c.emitter.Delete(obj); err != nil {
			log.Errorf("failed to handle delete for %s/%s (%s): %v",
				req.Namespace, req.Name, c.gvk.Kind, err)
			return ctrl.Result{RequeueAfter: 5 * time.Second}, err
		}
		return ctrl.Result{}, nil
	}

	// Update
	if err := c.emitter.Update(obj); err != nil {
		log.Errorf("failed to handle update for %s/%s (%s): %v",
			req.Namespace, req.Name, c.gvk.Kind, err)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, err
	}

	return ctrl.Result{}, nil
}

// periodicResync performs full resyncs at regular intervals for consistency
func periodicResync(ctx context.Context, c client.Client, emitter *HubHAEmitter,
	resourcesToSync []schema.GroupVersionKind, interval time.Duration,
) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("Hub HA periodic resync stopped")
			return
		case <-ticker.C:
			log.Info("starting Hub HA full resync")
			if err := performFullResync(ctx, c, emitter, resourcesToSync); err != nil {
				log.Errorf("failed to perform Hub HA full resync: %v", err)
			}
		}
	}
}

// performFullResync lists all resources and triggers a resync
func performFullResync(ctx context.Context, c client.Client, emitter *HubHAEmitter,
	resourcesToSync []schema.GroupVersionKind,
) error {
	allObjects := []client.Object{}

	for _, gvk := range resourcesToSync {
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   gvk.Group,
			Version: gvk.Version,
			Kind:    gvk.Kind + "List",
		})

		if err := c.List(ctx, list); err != nil {
			// If CRD doesn't exist, skip it
			if meta.IsNoMatchError(err) {
				log.Debugf("CRD not installed for %s, skipping resync", gvk.String())
				continue
			}
			log.Warnf("failed to list resources for %s during resync: %v", gvk.String(), err)
			continue
		}

		for i := range list.Items {
			obj := &list.Items[i]
			allObjects = append(allObjects, obj)
		}
	}

	log.Infof("performing Hub HA full resync with %d total objects", len(allObjects))
	if err := emitter.Resync(allObjects); err != nil {
		return err
	}
	// Send the resync bundle immediately
	return emitter.Send()
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
