// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package certificates

import (
	"fmt"
	"strings"

	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
)

const SignerName = "open-cluster-management.io/globalhub-signer"

// default: https://github.com/open-cluster-management-io/addon-framework/blob/main/pkg/agent/inteface.go#L213
func SignerAndCsrConfigurations(addonName string) func(cluster *clusterv1.ManagedCluster) []addonapiv1alpha1.RegistrationConfig {
	return func(cluster *clusterv1.ManagedCluster) []addonapiv1alpha1.RegistrationConfig {
		globalHubRegistrationConfig := addonapiv1alpha1.RegistrationConfig{
			SignerName: SignerName,
			Subject: addonapiv1alpha1.Subject{
				User: config.GetKafkaUserName(cluster.Name),
				// Groups: getGroups(cluster.Name, addonName),
			},
		}
		registrationConfigs := []addonapiv1alpha1.RegistrationConfig{globalHubRegistrationConfig}
		return registrationConfigs
	}
}

// https://github.com/open-cluster-management-io/ocm/blob/main/pkg/registration/spoke/addon/configuration.go
func AagentCertificateSecretName(clusterName string) string {
	return fmt.Sprintf("%s-%s-client-cert", operatorconstants.GHManagedClusterAddonName,
		strings.ReplaceAll(SignerName, "/", "-"))
}
