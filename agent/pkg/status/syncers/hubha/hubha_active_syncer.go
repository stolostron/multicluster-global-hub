// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubha

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

var log = logger.DefaultZapLogger()

const (
	groupAddonOCM                 = "addon.open-cluster-management.io"
	groupAgentOCM                 = "agent.open-cluster-management.io"
	groupAppsOCM                  = "apps.open-cluster-management.io"
	groupAuthenticationOCM        = "authentication.open-cluster-management.io"
	groupClusterOCM               = "cluster.open-cluster-management.io"
	groupConfigOCM                = "config.open-cluster-management.io"
	groupConsoleOCM               = "console.open-cluster-management.io"
	groupDiscoveryOCM             = "discovery.open-cluster-management.io"
	groupGlobalHubOCM             = "global-hub.open-cluster-management.io"
	groupImageRegistryOCM         = "imageregistry.open-cluster-management.io"
	groupObservabilityOCM         = "observability.open-cluster-management.io"
	groupPolicyOCM                = "policy.open-cluster-management.io"
	groupRBACOCM                  = "rbac.open-cluster-management.io"
	groupSiteConfigOCM            = "siteconfig.open-cluster-management.io"
	groupSubmarinerAddonOCM       = "submarineraddon.open-cluster-management.io"
	groupHiveExtensions           = "extensions.hive.openshift.io"
	groupHive                     = "hive.openshift.io"
	groupHiveInternal             = "hiveinternal.openshift.io"
	groupArgoProj                 = "argoproj.io"
	groupAppK8s                   = "app.k8s.io"
	groupObservatorium            = "core.observatorium.io"
	groupAgentInstall             = "agent-install.openshift.io"
	groupCAPIProviderAgentInstall = "capi-provider.agent-install.openshift.io"
)

type hubHAResourceSyncer struct {
	client  client.Client
	emitter *HubHAEmitter
	enabled atomic.Bool
}

func StartHubHAResourceSyncer(mgr ctrl.Manager, allGVKs []schema.GroupVersionKind,
	emitter *HubHAEmitter,
) (*hubHAResourceSyncer, error) {
	syncer := &hubHAResourceSyncer{
		client:  mgr.GetClient(),
		emitter: emitter,
	}
	syncer.enabled.Store(true)

	builder := ctrl.NewControllerManagedBy(mgr).
		Named("hubha").
		WithEventFilter(emitter.Predicate())

	var activeGVKs []schema.GroupVersionKind

	for _, gvk := range allGVKs {
		_, err := mgr.GetRESTMapper().RESTMapping(gvk.GroupKind(), gvk.Version)
		if err != nil {
			log.Debugf("skipped watch for %s: CRD not installed", gvk.String())
			continue
		}

		instance := &unstructured.Unstructured{}
		instance.SetGroupVersionKind(gvk)

		builder = builder.WatchesMetadata(instance, handler.EnqueueRequestsFromMapFunc(
			func(ctx context.Context, obj client.Object) []ctrl.Request {
				objGVK := obj.GetObjectKind().GroupVersionKind()

				encodedName := fmt.Sprintf("%s||%s||%s||%s",
					objGVK.Group, objGVK.Version, objGVK.Kind, obj.GetName())

				return []ctrl.Request{{
					NamespacedName: types.NamespacedName{
						Namespace: obj.GetNamespace(),
						Name:      encodedName,
					},
				}}
			},
		))

		activeGVKs = append(activeGVKs, gvk)
		log.Debugf("added Hub HA watch for %s", gvk.String())
	}

	if err := builder.Complete(syncer); err != nil {
		return nil, fmt.Errorf("failed to build Hub HA controller: %w", err)
	}

	emitter.SetActiveResources(activeGVKs)

	return syncer, nil
}

func (c *hubHAResourceSyncer) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if !c.enabled.Load() {
		return ctrl.Result{}, nil
	}

	parts := strings.Split(req.Name, "||")
	if len(parts) != 4 {
		log.Errorf("invalid encoded name format: %s", req.Name)
		return ctrl.Result{}, fmt.Errorf("invalid encoded name format: %s", req.Name)
	}

	gvk := schema.GroupVersionKind{
		Group:   parts[0],
		Version: parts[1],
		Kind:    parts[2],
	}
	realName := parts[3]
	realNamespace := req.Namespace

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)

	err := c.client.Get(ctx, types.NamespacedName{
		Namespace: realNamespace,
		Name:      realName,
	}, obj)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			obj.SetNamespace(realNamespace)
			obj.SetName(realName)
			obj.SetGroupVersionKind(gvk)
			if err := c.emitter.Delete(obj); err != nil {
				log.Errorf("failed to handle delete for %s/%s (%s): %v",
					realNamespace, realName, gvk.Kind, err)
				return ctrl.Result{RequeueAfter: 5 * time.Second}, err
			}
			return ctrl.Result{}, nil
		}
		log.Errorf("failed to get object %s/%s (%s): %v", realNamespace, realName, gvk.Kind, err)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, err
	}

	if !obj.GetDeletionTimestamp().IsZero() {
		if err := c.emitter.Delete(obj); err != nil {
			log.Errorf("failed to handle delete for %s/%s (%s): %v",
				realNamespace, realName, gvk.Kind, err)
			return ctrl.Result{RequeueAfter: 5 * time.Second}, err
		}
		return ctrl.Result{}, nil
	}

	if err := c.emitter.Update(obj); err != nil {
		log.Errorf("failed to handle update for %s/%s (%s): %v",
			realNamespace, realName, gvk.Kind, err)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, err
	}

	return ctrl.Result{}, nil
}

func GetHubHAResourcesToSync() []schema.GroupVersionKind {
	return []schema.GroupVersionKind{
		{Group: groupAddonOCM, Version: "v1beta1", Kind: "AddOnDeploymentConfig"},
		{Group: groupAddonOCM, Version: "v1alpha1", Kind: "AddOnTemplate"},
		{Group: groupAddonOCM, Version: "v1beta1", Kind: "ClusterManagementAddOn"},
		{Group: groupAddonOCM, Version: "v1beta1", Kind: "ManagedClusterAddOn"},

		{Group: groupAgentOCM, Version: "v1", Kind: "KlusterletAddonConfig"},

		{Group: groupAppsOCM, Version: "v1", Kind: "Channel"},
		{Group: groupAppsOCM, Version: "v1", Kind: "Deployable"},
		{Group: groupAppsOCM, Version: "v1beta1", Kind: "GitOpsCluster"},
		{Group: groupAppsOCM, Version: "v1", Kind: "HelmRelease"},
		{Group: groupAppsOCM, Version: "v1alpha1", Kind: "MulticlusterApplicationSetReport"},
		{Group: groupAppsOCM, Version: "v1", Kind: "PlacementRule"},
		{Group: groupAppsOCM, Version: "v1alpha1", Kind: "SubscriptionReport"},
		{Group: groupAppsOCM, Version: "v1", Kind: "Subscription"},
		{Group: groupAppsOCM, Version: "v1alpha1", Kind: "SubscriptionStatus"},

		{Group: groupAuthenticationOCM, Version: "v1beta1", Kind: "ManagedServiceAccount"},

		{Group: groupClusterOCM, Version: "v1alpha1", Kind: "AddOnPlacementScore"},
		{Group: groupClusterOCM, Version: "v1beta1", Kind: "BackupSchedule"},
		{Group: groupClusterOCM, Version: "v1alpha1", Kind: "ClusterClaim"},
		{Group: groupClusterOCM, Version: "v1beta1", Kind: "ClusterCurator"},
		{Group: groupClusterOCM, Version: "v1", Kind: "ManagedCluster"},
		{Group: groupClusterOCM, Version: "v1beta2", Kind: "ManagedClusterSetBinding"},
		{Group: groupClusterOCM, Version: "v1beta2", Kind: "ManagedClusterSet"},
		{Group: groupClusterOCM, Version: "v1beta1", Kind: "PlacementDecision"},
		{Group: groupClusterOCM, Version: "v1beta1", Kind: "Placement"},
		{Group: groupClusterOCM, Version: "v1beta1", Kind: "Restore"},

		{Group: groupConfigOCM, Version: "v1alpha1", Kind: "KlusterletConfig"},

		{Group: groupConsoleOCM, Version: "v1", Kind: "UserPreference"},

		{Group: groupDiscoveryOCM, Version: "v1", Kind: "DiscoveredCluster"},
		{Group: groupDiscoveryOCM, Version: "v1", Kind: "DiscoveryConfig"},

		{Group: groupGlobalHubOCM, Version: "v1alpha1", Kind: "ManagedClusterMigration"},

		{Group: groupImageRegistryOCM, Version: "v1alpha1", Kind: "ManagedClusterImageRegistry"},

		{Group: groupObservabilityOCM, Version: "v1beta2", Kind: "MultiClusterObservability"},
		{Group: groupObservabilityOCM, Version: "v1beta1", Kind: "ObservabilityAddon"},

		{Group: groupPolicyOCM, Version: "v1", Kind: "CertificatePolicy"},
		{Group: groupPolicyOCM, Version: "v1", Kind: "ConfigurationPolicy"},
		{Group: groupPolicyOCM, Version: "v1beta1", Kind: "OperatorPolicy"},
		{Group: groupPolicyOCM, Version: "v1", Kind: "PlacementBinding"},
		{Group: groupPolicyOCM, Version: "v1", Kind: "Policy"},
		{Group: groupPolicyOCM, Version: "v1beta1", Kind: "PolicyAutomation"},
		{Group: groupPolicyOCM, Version: "v1beta1", Kind: "PolicySet"},

		{Group: groupRBACOCM, Version: "v1alpha1", Kind: "ClusterPermission"},
		{Group: groupRBACOCM, Version: "v1beta1", Kind: "MulticlusterRoleAssignment"},

		{Group: groupSiteConfigOCM, Version: "v1alpha1", Kind: "ClusterInstance"},

		{Group: groupSubmarinerAddonOCM, Version: "v1alpha1", Kind: "SubmarinerConfig"},
		{Group: groupSubmarinerAddonOCM, Version: "v1alpha1", Kind: "SubmarinerDiagnoseConfig"},

		{Group: groupHiveExtensions, Version: "v1beta1", Kind: "AgentClusterInstall"},
		{Group: groupHiveExtensions, Version: "v1alpha1", Kind: "ImageClusterInstall"},

		{Group: groupHive, Version: "v1", Kind: "Checkpoint"},
		{Group: groupHive, Version: "v1", Kind: "ClusterClaim"},
		{Group: groupHive, Version: "v1", Kind: "ClusterDeploymentCustomization"},
		{Group: groupHive, Version: "v1", Kind: "ClusterDeployment"},
		{Group: groupHive, Version: "v1", Kind: "ClusterDeprovision"},
		{Group: groupHive, Version: "v1", Kind: "ClusterImageSet"},
		{Group: groupHive, Version: "v1", Kind: "ClusterPool"},
		{Group: groupHive, Version: "v1", Kind: "ClusterProvision"},
		{Group: groupHive, Version: "v1", Kind: "ClusterRelocate"},
		{Group: groupHive, Version: "v1", Kind: "ClusterState"},
		{Group: groupHive, Version: "v1", Kind: "DNSZone"},
		{Group: groupHive, Version: "v1", Kind: "HiveConfig"},
		{Group: groupHive, Version: "v1", Kind: "MachinePoolNameLease"},
		{Group: groupHive, Version: "v1", Kind: "MachinePool"},
		{Group: groupHive, Version: "v1", Kind: "SelectorSyncIdentityProvider"},
		{Group: groupHive, Version: "v1", Kind: "SelectorSyncSet"},
		{Group: groupHive, Version: "v1", Kind: "SyncIdentityProvider"},
		{Group: groupHive, Version: "v1", Kind: "SyncSet"},

		{Group: groupHiveInternal, Version: "v1alpha1", Kind: "ClusterSyncLease"},
		{Group: groupHiveInternal, Version: "v1alpha1", Kind: "ClusterSync"},
		{Group: groupHiveInternal, Version: "v1alpha1", Kind: "FakeClusterInstall"},

		{Group: groupArgoProj, Version: "v1alpha1", Kind: "Application"},
		{Group: groupArgoProj, Version: "v1alpha1", Kind: "ApplicationSet"},
		{Group: groupArgoProj, Version: "v1alpha1", Kind: "AppProject"},
		{Group: groupArgoProj, Version: "v1alpha1", Kind: "ArgoCD"},

		{Group: groupArgoProj, Version: "v1alpha1", Kind: "Workflow"},
		{Group: groupArgoProj, Version: "v1alpha1", Kind: "WorkflowTemplate"},
		{Group: groupArgoProj, Version: "v1alpha1", Kind: "ClusterWorkflowTemplate"},
		{Group: groupArgoProj, Version: "v1alpha1", Kind: "CronWorkflow"},

		{Group: groupArgoProj, Version: "v1alpha1", Kind: "EventSource"},
		{Group: groupArgoProj, Version: "v1alpha1", Kind: "Sensor"},
		{Group: groupArgoProj, Version: "v1alpha1", Kind: "EventBus"},

		{Group: groupArgoProj, Version: "v1alpha1", Kind: "Rollout"},
		{Group: groupArgoProj, Version: "v1alpha1", Kind: "Experiment"},
		{Group: groupArgoProj, Version: "v1alpha1", Kind: "AnalysisTemplate"},
		{Group: groupArgoProj, Version: "v1alpha1", Kind: "ClusterAnalysisTemplate"},

		{Group: groupAppK8s, Version: "v1beta1", Kind: "Application"},

		{Group: groupObservatorium, Version: "v1alpha1", Kind: "Observatorium"},

		{Group: groupAgentInstall, Version: "v1beta1", Kind: "AgentClassification"},
		{Group: groupAgentInstall, Version: "v1beta1", Kind: "Agent"},
		{Group: groupAgentInstall, Version: "v1beta1", Kind: "AgentServiceConfig"},
		{Group: groupAgentInstall, Version: "v1beta1", Kind: "HypershiftAgentServiceConfig"},
		{Group: groupAgentInstall, Version: "v1beta1", Kind: "InfraEnv"},
		{Group: groupAgentInstall, Version: "v1beta1", Kind: "NMStateConfig"},

		{Group: groupCAPIProviderAgentInstall, Version: "v1beta1", Kind: "AgentCluster"},
		{Group: groupCAPIProviderAgentInstall, Version: "v1beta1", Kind: "AgentMachine"},
		{Group: groupCAPIProviderAgentInstall, Version: "v1beta1", Kind: "AgentMachineTemplate"},

		{Group: "", Version: "v1", Kind: "Secret"},
		{Group: "", Version: "v1", Kind: "ConfigMap"},
	}
}
