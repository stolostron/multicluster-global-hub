package migration

import "k8s.io/apimachinery/pkg/runtime/schema"

type MigrationResource struct {
	// namespace and name is same as cluster name if it's not set
	name      string
	namespace string
	gvk       schema.GroupVersionKind
	// if it is need to sync the resource status
	needStatus bool
	// if the value is false, return error when it is not found
	optional bool
	// if this is ztp resources
	isZtp bool
}

// when add a new kind of resource here, should also update perimssion in the following files:
// - operator/config/rbac/role.yaml
// - operator/pkg/controllers/agent/addon/manifests/templates/agent/multicluster-global-hub-agent-clusterrole.yaml
// - operator/pkg/controllers/agent/local_agent_controller.go
// - operator/pkg/controllers/agent/manifests/clusterrole.yaml
var migrateResources = []MigrationResource{
	// should migrate secrets and configmaps first
	{
		gvk: schema.GroupVersionKind{
			Group:   "",
			Version: "v1",
			Kind:    "Secret",
		},
		name:       "<CLUSTER_NAME>-admin-password",
		needStatus: false,
		isZtp:      true,
	},
	{
		gvk: schema.GroupVersionKind{
			Group:   "",
			Version: "v1",
			Kind:    "Secret",
		},
		name:       "<CLUSTER_NAME>-bmc-secret",
		needStatus: false,
		isZtp:      true,
	},
	{
		gvk: schema.GroupVersionKind{
			Group:   "",
			Version: "v1",
			Kind:    "Secret",
		},
		name:       "assisted-deployment-pull-secret",
		needStatus: false,
		isZtp:      true,
	},
	{
		gvk: schema.GroupVersionKind{
			Group:   "",
			Version: "v1",
			Kind:    "Secret",
		},
		name:       "<CLUSTER_NAME>-admin-kubeconfig",
		needStatus: false,
		isZtp:      true,
	},
	{
		gvk: schema.GroupVersionKind{
			Group:   "",
			Version: "v1",
			Kind:    "ConfigMap",
		},
		needStatus: false,
		isZtp:      true,
		optional:   true,
	},
	// managedclusters and addons
	{
		gvk: schema.GroupVersionKind{
			Group:   "cluster.open-cluster-management.io",
			Version: "v1",
			Kind:    "ManagedCluster",
		},
		needStatus: false,
	},
	{
		gvk: schema.GroupVersionKind{
			Group:   "agent.open-cluster-management.io",
			Version: "v1",
			Kind:    "KlusterletAddonConfig",
		},
		needStatus: false,
	},
	// ZTP related resources
	{
		gvk: schema.GroupVersionKind{
			Group:   "metal3.io",
			Version: "v1alpha1",
			Kind:    "BareMetalHost",
		},
		needStatus: true,
		isZtp:      true,
	},
	{
		gvk: schema.GroupVersionKind{
			Group:   "agent-install.openshift.io",
			Version: "v1beta1",
			Kind:    "InfraEnv",
		},
		needStatus: true,
		isZtp:      true,
		optional:   true,
	},
	{
		gvk: schema.GroupVersionKind{
			Group:   "metal3.io",
			Version: "v1alpha1",
			Kind:    "HostFirmwareSettings",
		},
		needStatus: true,
		isZtp:      true,
	},
	{
		gvk: schema.GroupVersionKind{
			Group:   "agent-install.openshift.io",
			Version: "v1beta1",
			Kind:    "NMStateConfig",
		},
		needStatus: false,
		isZtp:      true,
		optional:   true,
	},
	{
		gvk: schema.GroupVersionKind{
			Group:   "extensions.hive.openshift.io",
			Version: "v1alpha1",
			Kind:    "ImageClusterInstall",
		},
		needStatus: true,
		isZtp:      true,
		optional:   true,
	},
	{
		gvk: schema.GroupVersionKind{
			Group:   "hive.openshift.io",
			Version: "v1",
			Kind:    "ClusterDeployment",
		},
		needStatus: true,
		isZtp:      true,
	},
	{
		gvk: schema.GroupVersionKind{
			Group:   "siteconfig.open-cluster-management.io",
			Version: "v1alpha1",
			Kind:    "ClusterInstance",
		},
		needStatus: true,
		isZtp:      true,
		name:       "<CLUSTER_NAME>-clusterinstance",
	},
}
