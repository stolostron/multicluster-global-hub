// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package scheme

import (
	"fmt"

	configv1 "github.com/stolostron/hub-of-hubs/pkg/apis/config/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1alpha1 "open-cluster-management.io/api/cluster/v1alpha1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	channelv1 "open-cluster-management.io/multicloud-operators-channel/pkg/apis/apps/v1"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	appsv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1"
	subscriptionv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1"
	appsv1alpha1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1alpha1"
	applicationv1beta1 "sigs.k8s.io/application/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

// AddToScheme adds all the resources to be processed to the Scheme.
func AddToScheme(scheme *runtime.Scheme) error {
	schemeInstallFuncs := []func(scheme *runtime.Scheme) error{
		clusterv1alpha1.Install,
		clusterv1beta1.Install,
	}

	for _, schemeInstallFunc := range schemeInstallFuncs {
		if err := schemeInstallFunc(scheme); err != nil {
			return fmt.Errorf("failed to install scheme: %w", err)
		}
	}

	for _, schemeBuilder := range getSchemeBuilders() {
		if err := schemeBuilder.AddToScheme(scheme); err != nil {
			return fmt.Errorf("failed to add scheme: %w", err)
		}
	}

	// install cluster v1alpha1 / v1beta1 schemes
	if err := clusterv1alpha1.Install(scheme); err != nil {
		return fmt.Errorf("failed to install scheme: %w", err)
	}

	if err := clusterv1beta1.Install(scheme); err != nil {
		return fmt.Errorf("failed to install scheme: %w", err)
	}

	return nil
}

func getSchemeBuilders() []*scheme.Builder {
	return []*scheme.Builder{
		configv1.SchemeBuilder, policyv1.SchemeBuilder, placementrulev1.SchemeBuilder,
		appsv1.SchemeBuilder, appsv1alpha1.SchemeBuilder, channelv1.SchemeBuilder,
		subscriptionv1.SchemeBuilder, applicationv1beta1.SchemeBuilder,
	}
}
