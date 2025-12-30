package migration

import "k8s.io/apimachinery/pkg/runtime/schema"

const clusterNamePlaceholder = "<CLUSTER_NAME>"

type MigrationResource struct {
	// if the name is not set, will migrate all resources in cluster namespace
	name string
	// if annotationKey is set, should sync all resources with annotationKey
	annotationKey string
	gvk           schema.GroupVersionKind
	// Need to sync the resource status or not
	needStatus bool
}

// when add a new kind of resource here, should also update perimssion in the following files:
// - operator/config/rbac/role.yaml
// - operator/pkg/controllers/agent/addon/manifests/templates/agent/multicluster-global-hub-agent-clusterrole.yaml
// - operator/pkg/controllers/agent/local_agent_controller.go
// - operator/pkg/controllers/agent/manifests/clusterrole.yaml
var migrateResources = []MigrationResource{
	{
		gvk: schema.GroupVersionKind{
			Group:   "",
			Version: "v1",
			Kind:    "Secret",
		},
		name:       clusterNamePlaceholder + "-admin-password",
		needStatus: false,
	},
	{
		gvk: schema.GroupVersionKind{
			Group:   "",
			Version: "v1",
			Kind:    "Secret",
		},
		name:       clusterNamePlaceholder + "-admin-kubeconfig",
		needStatus: false,
	},
	// Should sync NetworkSecret, it is created with annotation <siteconfig.open-cluster-management.io/sync-wave: "1">
	// https://github.com/stolostron/siteconfig/blob/50303ea9/internal/templates/image-based-installer/template.go#L158
	{
		gvk: schema.GroupVersionKind{
			Group:   "",
			Version: "v1",
			Kind:    "Secret",
		},
		annotationKey: "siteconfig.open-cluster-management.io/sync-wave",
		needStatus:    false,
	},
	// managedclusters and addons
	{
		gvk: schema.GroupVersionKind{
			Group:   "cluster.open-cluster-management.io",
			Version: "v1",
			Kind:    "ManagedCluster",
		},
		needStatus: false,
		name:       clusterNamePlaceholder,
	},
	{
		gvk: schema.GroupVersionKind{
			Group:   "agent.open-cluster-management.io",
			Version: "v1",
			Kind:    "KlusterletAddonConfig",
		},
		needStatus: false,
		name:       clusterNamePlaceholder,
	},
	// clusterinstance related resources
	{
		gvk: schema.GroupVersionKind{
			Group:   "metal3.io",
			Version: "v1alpha1",
			Kind:    "BareMetalHost",
		},
		needStatus: true,
	},
	{
		gvk: schema.GroupVersionKind{
			Group:   "metal3.io",
			Version: "v1alpha1",
			Kind:    "HostFirmwareSettings",
		},
		needStatus: true,
	},
	{
		gvk: schema.GroupVersionKind{
			Group:   "metal3.io",
			Version: "v1alpha1",
			Kind:    "FirmwareSchema",
		},
		needStatus: true,
	},
	{
		gvk: schema.GroupVersionKind{
			Group:   "metal3.io",
			Version: "v1alpha1",
			Kind:    "HostFirmwareComponents",
		},
		needStatus: true,
	},
	{
		gvk: schema.GroupVersionKind{
			Group:   "extensions.hive.openshift.io",
			Version: "v1alpha1",
			Kind:    "ImageClusterInstall",
		},
		needStatus: true,
		name:       clusterNamePlaceholder,
	},
	{
		gvk: schema.GroupVersionKind{
			Group:   "hive.openshift.io",
			Version: "v1",
			Kind:    "ClusterDeployment",
		},
		needStatus: true,
		name:       clusterNamePlaceholder,
	},
}
