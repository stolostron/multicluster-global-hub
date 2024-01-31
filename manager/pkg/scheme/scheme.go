// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package scheme

import (
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	clusterv1alpha1 "open-cluster-management.io/api/cluster/v1alpha1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	clusterv1beta2 "open-cluster-management.io/api/cluster/v1beta2"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	channelv1 "open-cluster-management.io/multicloud-operators-channel/pkg/apis/apps/v1"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	subscriptionv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1"
	subscriptionv1alpha1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1alpha1"
	applicationv1beta1 "sigs.k8s.io/application/api/v1beta1"
)

func AddToScheme(scheme *runtime.Scheme) {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(clusterv1alpha1.AddToScheme(scheme))
	utilruntime.Must(clusterv1beta1.AddToScheme(scheme))
	utilruntime.Must(clusterv1beta2.AddToScheme(scheme))
	utilruntime.Must(clusterv1.AddToScheme(scheme))
	utilruntime.Must(policyv1.AddToScheme(scheme))
	utilruntime.Must(placementrulev1.AddToScheme(scheme))
	utilruntime.Must(subscriptionv1.SchemeBuilder.AddToScheme(scheme))
	utilruntime.Must(subscriptionv1alpha1.SchemeBuilder.AddToScheme(scheme))
	utilruntime.Must(channelv1.AddToScheme(scheme))
	utilruntime.Must(applicationv1beta1.AddToScheme(scheme))
	utilruntime.Must(mchv1.AddToScheme(scheme))
}
