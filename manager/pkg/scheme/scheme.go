// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package scheme

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	clusterv1alpha1 "open-cluster-management.io/api/cluster/v1alpha1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	channelv1 "open-cluster-management.io/multicloud-operators-channel/pkg/apis/apps/v1"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	appsubv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1"
	appsubv1alpha1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1alpha1"
	appv1beta1 "sigs.k8s.io/application/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/scheme"

	mghoperatorv1alpha2 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha2"
)

// AddToScheme adds all the resources to be processed to the Scheme.
func AddToScheme(runtimeScheme *runtime.Scheme) error {
	schemeInstallFuncs := []func(scheme *runtime.Scheme) error{
		clusterv1alpha1.Install,
		clusterv1beta1.Install,
	}

	schemeBuilders := []*scheme.Builder{
		policyv1.SchemeBuilder,
		placementrulev1.SchemeBuilder,
		appsubv1alpha1.SchemeBuilder,
		channelv1.SchemeBuilder,
		appsubv1.SchemeBuilder,
		appv1beta1.SchemeBuilder,
		mghoperatorv1alpha2.SchemeBuilder,
	}

	for _, schemeInstallFunc := range schemeInstallFuncs {
		if err := schemeInstallFunc(runtimeScheme); err != nil {
			return fmt.Errorf("failed to install scheme: %w", err)
		}
	}

	for _, schemeBuilder := range schemeBuilders {
		if err := schemeBuilder.AddToScheme(runtimeScheme); err != nil {
			return fmt.Errorf("failed to add scheme: %w", err)
		}
	}

	return nil
}
