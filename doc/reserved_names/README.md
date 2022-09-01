# Labels

List all labels are used by the multicluster global hub.

Label | Description
--- | ----------
global-hub.open-cluster-management.io/managed-by=`multicluster-global-hub-operator\|multicluster-global-hub-agent` | If the value is `multicluster-global-hub-operator`, it means the resources are created by the global hub operator. The global hub operator watches the resources based on this label.
global-hub.open-cluster-management.io/regional-hub-type=`NoHubInstall\|NoHubAgentInstall` | This label is applied to the managedcluster resource. If the value is `NoHubInstall`, the global hub operator installs the global hub agent only in the managed cluster. If the value is `NoHubAgentInstall`, the managed cluster won't be managed by the global hub.
global-hub.open-cluster-management.io/local-resource= | This label is added during creating some resources. It is used to identify the resource is only applied to global hub cluster. It won't be transfered to the regional hub clusters.

# Annotations

List all annotations are used by multicluster global hub.

Annotation | Description
--- | ----------
global-hub.open-cluster-management.io/managed-by= | This annotation is used to identify the managed cluster is managed by which regional hub cluster.
global-hub.open-cluster-management.io/origin-ownerreference-uid= | This annotation is used to identify the resource is from the global hub cluster. The global hub agent is only handled with the resource which has this annotation.

# Finalizer

List all finalizers are used by the multicluster global hub.

Finalizer | Description
--- | ----------
global-hub.open-cluster-management.io/resource-cleanup | This is the finalizer which is used by the multicluster global hub.
