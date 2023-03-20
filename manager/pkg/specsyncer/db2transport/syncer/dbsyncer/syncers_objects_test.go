// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package dbsyncer_test

import (
	"fmt"

	"github.com/google/uuid"
)

var (
	configUID       = uuid.New().String()
	configJSONBytes = []byte(fmt.Sprintf(`{
"apiVersion": "v1",
"data": {
"aggregationLevel": "full",
"enableLocalPolicies": "true"
},
"kind": "ConfigMap",
"metadata": {
"creationTimestamp": null,
"labels": {
"global-hub.open-cluster-management.io/managed-by": "global-hub-operator",
"global-hub.open-cluster-management.io/global-resource": ""
},
"annotations": {
"global-hub.open-cluster-management.io/origin-ownerreference-uid": "%s"
},
"name": "multicluster-global-hub-config",
"namespace": "open-cluster-management-global-hub-system"
}
}`, configUID))
)

var (
	managedclusterUID  = uuid.New().String()
	leafhubName        = "hub1"
	managedclusterName = "mc1"
	labelsToAdd        = map[string]string{
		"foo": "bar",
		"env": "dev",
	}
)

var labelKeysToRemove = []string{
	"goo",
	"haa",
}

var (
	managedclustersetUID       = uuid.New().String()
	managedclustersetJSONBytes = []byte(fmt.Sprintf(`{
"apiVersion": "cluster.open-cluster-management.io/v1beta2",
"kind": "ManagedClusterSet",
"metadata": {
"name": "test-managedclusterset-1",
"namespace": "default",
"creationTimestamp": null,
"labels": {
	"global-hub.open-cluster-management.io/global-resource": ""
},
"annotations": {
"global-hub.open-cluster-management.io/origin-ownerreference-uid": "%s"
}
},
"spec": {
"clusterSelector": {
"selectorType": "ExclusiveClusterSetLabel"
}
},
"status": {
"conditions": []
}
}`, managedclustersetUID))
)

var (
	managedclustersetbindingUID       = uuid.New().String()
	managedclustersetbindingJSONBytes = []byte(fmt.Sprintf(`{
"apiVersion": "cluster.open-cluster-management.io/v1beta2",
"kind": "ManagedClusterSetBinding",
"metadata": {
"name": "test-managedclustersetbinding-1",
"namespace": "default",
"creationTimestamp": null,
"labels": {
	"global-hub.open-cluster-management.io/global-resource": ""
},
"annotations": {
"global-hub.open-cluster-management.io/origin-ownerreference-uid": "%s"
}
},
"spec": {
"clusterSet": "test-managedclusterset-1"
},
"status": {
"conditions": []
}
}`, managedclustersetbindingUID))
)

var (
	policyUID       = uuid.New().String()
	policyJSONBytes = []byte(fmt.Sprintf(`{
"apiVersion": "policy.open-cluster-management.io/v1",
"kind": "Policy",
"metadata": {
"name": "test-policy-1",
"namespace": "default",
"creationTimestamp": null,
"annotations": {
"global-hub.open-cluster-management.io/origin-ownerreference-uid": "%s",
"policy.open-cluster-management.io/standards": "NIST SP 800-53",
"policy.open-cluster-management.io/categories": "AU Audit and Accountability",
"policy.open-cluster-management.io/controls": "AU-3 Content of Audit Records"
},
"labels": {
"env": "production",
"global-hub.open-cluster-management.io/global-resource": ""
}
},
"spec": {
"disabled": false,
"policy-templates": []
},
"status": {}
}`, policyUID))
)

var (
	placementruleUID       = uuid.New().String()
	placementruleJSONBytes = []byte(fmt.Sprintf(`{
"apiVersion": "apps.open-cluster-management.io/v1",
"kind": "PlacementRule",
"metadata": {
"name": "test-placementrule-1",
"namespace": "default",
"creationTimestamp": null,
"labels": {
"global-hub.open-cluster-management.io/global-resource": ""
},
"annotations": {
"global-hub.open-cluster-management.io/origin-ownerreference-uid": "%s"
}
},
"spec": {},
"status": {}
}`, placementruleUID))
)

var (
	placementbindingUID       = uuid.New().String()
	placementbindingJSONBytes = []byte(fmt.Sprintf(`{
"apiVersion": "policy.open-cluster-management.io/v1",
"kind": "PlacementBinding",
"metadata": {
"name": "test-placementbinding-1",
"namespace": "default",
"creationTimestamp": null,
"labels": {
	"global-hub.open-cluster-management.io/global-resource": ""
	},
"annotations": {
"global-hub.open-cluster-management.io/origin-ownerreference-uid": "%s"
}
},
"placementRef": {
"name": "test-placementrule-1",
"kind": "PlacementRule",
"apiGroup": "apps.open-cluster-management.io"
},
"subjects": [
{
"name": "test-policy-1",
"kind": "Policy",
"apiGroup": "policy.open-cluster-management.io"
}
],
"status": {}
}`, placementbindingUID))
)

var (
	placementUID       = uuid.New().String()
	placementJSONBytes = []byte(fmt.Sprintf(`{
"apiVersion": "cluster.open-cluster-management.io/v1beta1",
"kind": "Placement",
"metadata": {
"name": "test-placement-1",
"namespace": "default",
"creationTimestamp": null,
"annotations": {
"global-hub.open-cluster-management.io/origin-ownerreference-uid": "%s"
},
"labels": {
"env": "production",
"global-hub.open-cluster-management.io/global-resource": ""
}
},
"spec": {
"prioritizerPolicy": {
"mode": "Additive"
}
},
"status": {
	"numberOfSelectedClusters": 0,
	"conditions": []
}
}`, placementUID))
)

var (
	applicationUID       = uuid.New().String()
	applicationJSONBytes = []byte(fmt.Sprintf(`{
"apiVersion": "app.k8s.io/v1beta1",
"kind": "Application",
"metadata": {
"name": "test-application-1",
"namespace": "default",
"labels": {
	"global-hub.open-cluster-management.io/global-resource": ""
	},
"annotations": {
"global-hub.open-cluster-management.io/origin-ownerreference-uid": "%s"
},
"creationTimestamp": null
},
"spec": {
"componentKinds": [
{
"group": "apps.open-cluster-management.io",
"kind": "Subscription"
}
],
"descriptor": {},
"selector": {
"matchExpressions": [
{
"key": "app",
"operator": "In",
"values": [
"helloworld-app"
]
}
]
}
},
"status": {}
}`, applicationUID))
)

var (
	channelUID       = uuid.New().String()
	channelJSONBytes = []byte(fmt.Sprintf(`{
"apiVersion": "apps.open-cluster-management.io/v1",
"kind": "Channel",
"metadata": {
"name": "test-channel-1",
"namespace": "default",
"labels": {
	"global-hub.open-cluster-management.io/global-resource": ""
	},
"annotations": {
"global-hub.open-cluster-management.io/origin-ownerreference-uid": "%s"
},
"creationTimestamp": null
},
"spec": {
"type": "GitHub",
"pathname": "https://github.com/open-cluster-management/application-samples.git"
},
"status": {}
}`, channelUID))
)

var (
	subscriptionUID       = uuid.New().String()
	subscriptionJSONBytes = []byte(fmt.Sprintf(`{
"apiVersion": "apps.open-cluster-management.io/v1",
"kind": "Subscription",
"metadata": {
"annotations": {
"global-hub.open-cluster-management.io/origin-ownerreference-uid": "%s",
"apps.open-cluster-management.io/git-branch": "main",
"apps.open-cluster-management.io/git-path": "bar",
"apps.open-cluster-management.io/reconcile-option": "merge"
},
"labels": {
"app": "bar",
"app.kubernetes.io/part-of": "bar",
"apps.open-cluster-management.io/reconcile-rate": "medium",
"global-hub.open-cluster-management.io/global-resource": ""
},
"name": "test-subscription-1",
"namespace": "default",
"creationTimestamp": null
},
"spec": {
"channel": "git-application-samples-ns/git-application-samples",
"placement": {
"placementRef": {
	"kind": "PlacementRule",
	"name": "test-placement-1"
}
}
},
"status": {
"lastUpdateTime": null,
"ansiblejobs": {}
}
}`, subscriptionUID))
)
