// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller_test

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	appsv1alpha1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1alpha1"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

var globalHubConfigConfigMap = &corev1.ConfigMap{
	ObjectMeta: metav1.ObjectMeta{
		Name:      constants.HoHConfigName,
		Namespace: constants.HohSystemNamespace,
		Annotations: map[string]string{
			constants.OriginOwnerReferenceAnnotation: "testing",
		},
	},
	Data: map[string]string{"aggregationLevel": "full", "enableLocalPolicies": "true"},
}

var testMangedCluster = &clusterv1.ManagedCluster{
	ObjectMeta: metav1.ObjectMeta{
		Name: "test-mc-1",
		Labels: map[string]string{
			"cloud":  "Other",
			"vendor": "Other",
		},
		Annotations: map[string]string{
			"cloud":  "Other",
			"vendor": "Other",
		},
	},
	Spec: clusterv1.ManagedClusterSpec{
		HubAcceptsClient:     true,
		LeaseDurationSeconds: 60,
	},
}

var (
	testGlobalPolicyOriginUID = "test-globalpolicy-uid"
	testGlobalPolicy          = &policyv1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-globalpolicy-1",
			Namespace: "default",
			Annotations: map[string]string{
				constants.OriginOwnerReferenceAnnotation: testGlobalPolicyOriginUID,
			},
		},
		Spec: policyv1.PolicySpec{
			Disabled:        false,
			PolicyTemplates: []*policyv1.PolicyTemplate{},
		},
		Status: policyv1.PolicyStatus{},
	}
)

var (
	testGlobalPlacementRuleOriginUID = "test-globalplacementrule-uid"
	testGlobalPlacementRule          = &placementrulev1.PlacementRule{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "test-globalplacementrule-1",
			Namespace:    "default",
			Annotations: map[string]string{
				constants.OriginOwnerReferenceAnnotation: testGlobalPlacementRuleOriginUID,
			},
		},
		Spec: placementrulev1.PlacementRuleSpec{},
	}
)

var (
	testGlobalPlacementOriginUID = "test-globalplacement-uid"
	testGlobalPlacement          = &clusterv1beta1.Placement{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-globalplacement-1",
			Namespace: "default",
			Annotations: map[string]string{
				constants.OriginOwnerReferenceAnnotation: testGlobalPlacementOriginUID,
			},
		},
		Spec: clusterv1beta1.PlacementSpec{},
	}
)

var testPlacementDecision = &clusterv1beta1.PlacementDecision{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "test-placementdecision-1",
		Namespace: "default",
	},
	Status: clusterv1beta1.PlacementDecisionStatus{},
}

var testSubscriptionReport = &appsv1alpha1.SubscriptionReport{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "test-subscriptionreport-1",
		Namespace: "default",
	},
	ReportType: "Application",
	Summary: appsv1alpha1.SubscriptionReportSummary{
		Deployed:          "1",
		InProgress:        "0",
		Failed:            "0",
		PropagationFailed: "0",
		Clusters:          "1",
	},
	Results: []*appsv1alpha1.SubscriptionReportResult{
		{
			Source: "hub1-mc1",
			Result: "deployed",
		},
	},
	Resources: []*corev1.ObjectReference{
		{
			Kind:       "Deployment",
			Namespace:  "default",
			Name:       "nginx-sample",
			APIVersion: "apps/v1",
		},
	},
}
