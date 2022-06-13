package utils

import "k8s.io/apimachinery/pkg/runtime/schema"

func NewRouteGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    "route.openshift.io",
		Version:  "v1",
		Resource: "routes",
	}
}

func NewManagedClustersGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    "cluster.open-cluster-management.io",
		Version:  "v1",
		Resource: "managedclusters",
	}
}

func NewPolicyGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    "policy.open-cluster-management.io",
		Version:  "v1",
		Resource: "policies",
	}
}

func NewPlacementRule() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "apps.open-cluster-management.io", Version: "v1", Resource: "placementrules"}
}

func NewSubscriptionreportsGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    "apps.open-cluster-management.io",
		Version:  "v1alpha1",
		Resource: "subscriptionreports",
	}
}
