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
}

// when add a new kind of resource here, should also update perimssion in the following files:
// - operator/config/rbac/role.yaml
// - operator/pkg/controllers/agent/addon/manifests/templates/agent/multicluster-global-hub-agent-clusterrole.yaml
// - operator/pkg/controllers/agent/local_agent_controller.go
// - operator/pkg/controllers/agent/manifests/clusterrole.yaml
var migrateResources = []MigrationResource{
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
}
