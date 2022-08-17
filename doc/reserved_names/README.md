# Labels

List all labels are used by the multicluster global hub.

Label | Description
--- | ----------
global-hub.open-cluster-management.io/managed-by=multicluster-global-hub-operator | This label is applied to the resources which are created by the global hub operator. The global hub operator watches the resources based on this label.
global-hub.open-cluster-management.io/managed-by=multicluster-global-hub-agent | This label is applied to the resources which are created by the global hub agent. The global hub agent watches the resources based on this label.
global-hub.open-cluster-management.io/regional-hub=disable | This label is applied to the managedcluster resource. Identify this managed cluster won't have ACM installed by the global hub.
global-hub.open-cluster-management.io/global-hub-agent=disable | This label is applied to the managedcluster resource. Identify this managed cluster won't be managed by the global hub.
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
