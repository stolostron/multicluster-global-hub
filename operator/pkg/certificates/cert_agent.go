// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package certificates

import (
	"crypto/sha256"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"open-cluster-management.io/addon-framework/pkg/agent"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	addonName = "multicluster-global-hub-controller" // #nosec G101 -- Not a hardcoded credential.
	agentName = "multicluster-global-hub-agent"
)

type GlobalHubAgent struct {
	client client.Client
}

func (o *GlobalHubAgent) Manifests(
	cluster *clusterv1.ManagedCluster,
	addon *addonapiv1alpha1.ManagedClusterAddOn,
) ([]runtime.Object, error) {
	return nil, nil
}

func (o *GlobalHubAgent) GetAgentAddonOptions() agent.AgentAddonOptions {
	return agent.AgentAddonOptions{
		AddonName: addonName,
		Registration: &agent.RegistrationOption{
			CSRConfigurations: globalHubSignerConfigurations(o.client),
			CSRApproveCheck:   approve,
			PermissionConfig: func(cluster *clusterv1.ManagedCluster, addon *addonapiv1alpha1.ManagedClusterAddOn) error {
				return nil
			},
			CSRSign: Sign,
		},
	}
}

func globalHubSignerConfigurations(client client.Client) func(cluster *clusterv1.ManagedCluster) []addonapiv1alpha1.RegistrationConfig {
	return func(cluster *clusterv1.ManagedCluster) []addonapiv1alpha1.RegistrationConfig {
		observabilityConfig := addonapiv1alpha1.RegistrationConfig{
			SignerName: "open-cluster-management.io/globalhub-signer",
			Subject: addonapiv1alpha1.Subject{
				User:              "inventory-api",
				OrganizationUnits: []string{"globalhub"},
			},
		}
		kubeClientSignerConfigurations := agent.KubeClientSignerConfigurations(addonName, agentName)
		registrationConfigs := append(kubeClientSignerConfigurations(cluster), observabilityConfig)

		_, _, caCertBytes, err := getCA(client, true)
		if err == nil {
			caHashStamp := fmt.Sprintf("ca-hash-%x", sha256.Sum256(caCertBytes))
			for i := range registrationConfigs {
				registrationConfigs[i].Subject.OrganizationUnits = append(registrationConfigs[i].Subject.OrganizationUnits, caHashStamp)
			}
		}

		return registrationConfigs
	}
}
