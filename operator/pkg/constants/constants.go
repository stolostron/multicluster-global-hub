/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package constants

const (
	HOHDefaultNamespace = "open-cluster-management"
	HOHSystemNamespace  = "hoh-system"
	HOHConfigName       = "multicluster-globalhub-config"
	LocalClusterName    = "local-cluster"
)

const (
	LeafHubClusterDisabledLabelKey           = "hoh"
	LeafHubClusterDisabledLabelVal           = "disabled"
	LeafHubClusterAnnotationKey              = "hub-of-hubs.open-cluster-management.io/managed-by-hoh"
	GlobalHubSkipConsoleInstallAnnotationKey = "globalhub.open-cluster-management.io/skip-console-install"
	HoHOperatorOwnerLabelKey                 = "globalhub.open-cluster-management.io/managed-by"
	HoHOperatorOwnerLabelVal                 = "multicluster-globalhub-operator"
	HoHOperatorFinalizer                     = "globalhub.open-cluster-management.io/res-cleanup"

	LeafHubClusterInstallHubLabelKey        = "globalhub.open-cluster-management.io/hub-install"
	LeafHubClusterDisableInstallHubLabelVal = "disabled"
)

const (
	HoHClusterManagementAddonName        = "multicluster-globalhub-controller"
	HoHClusterManagementAddonDisplayName = "Multicluster Global Hub Controller"
	HoHClusterManagementAddonDescription = "Multicluster Global Hub Controller " +
		"manages multicluster-globalhub components."
	HoHManagedClusterAddonName        = "multicluster-globalhub-controller"
	HoHManagedClusterAddonDisplayName = "Multicluster Global Hub Controller"
	HoHManagedClusterAddonDescription = "Multicluster Global Hub Controller " +
		"manages multicluster-globalhub components."
	HoHManagedClusterAddonInstallationNamespace = "open-cluster-management"
)

const (
	HOHHubSubscriptionWorkSuffix = "hoh-hub-subscription"
	HoHHubMCHWorkSuffix          = "hoh-hub-mch"
	HoHHostingHubWorkSuffix      = "hoh-hub-hosting"
	HoHHostedHubWorkSuffix       = "hoh-hub-hosted"
	HoHHostingAgentWorkSuffix    = "hoh-agent-hosting"
	HoHHostedAgentWorkSuffix     = "hoh-agent-hosted"
	HoHAgentWorkSuffix           = "hoh-agent"
)

const (
	WorkPostponeDeleteAnnotationKey = "open-cluster-management/postpone-delete"
	OpenshiftMarketPlaceNamespace   = "openshift-marketplace"
	ACMSubscriptionPublicSource     = "redhat-operators"
	ACMSubscriptionPrivateSource    = "acm-custom-registry"
	ACMPackageManifestName          = "advanced-cluster-management"
	MCEPackageManifestName          = "multicluster-engine"
)

const (
	DefaultACMUpstreamImageRegistry   = "quay.io/stolostron"
	DefaultMCEUpstreamImageRegistry   = "quay.io/stolostron"
	DefaultACMDownStreamImageRegistry = "registry.redhat.io/rhacm2"
	DefaultMCEDownStreamImageRegistry = "registry.redhat.io/multicluster-engine"
)

const (
	HoHDefaultImageRepository  = "quay.io/stolostron"
	HoHImageRepoAnnotationKey  = "hoh-image-repository"
	HoHImageTagAnnotationKey   = "hoh-image-tag"
	HoHHubACMSnapShotKey       = "hoh-hub-ACM-snapshot"
	HoHHubMCESnapShotKey       = "hoh-hub-MCE-snapshot"
	HoHKafkaBootstrapServerKey = "hoh-kafka-bootstrap-server"
)
