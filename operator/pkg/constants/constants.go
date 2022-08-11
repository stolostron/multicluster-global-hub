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
	HOHConfigName       = "hub-of-hubs-config"
	LocalClusterName    = "local-cluster"
)

const (
	LeafHubClusterDisabledLabelKey = "hoh"
	LeafHubClusterDisabledLabelVal = "disabled"
	LeafHubClusterAnnotationKey    = "hub-of-hubs.open-cluster-management.io/managed-by-hoh"
	HoHOperatorOwnerLabelKey       = "hub-of-hubs.open-cluster-management.io/managed-by"
	HoHOperatorFinalizer           = "hub-of-hubs.open-cluster-management.io/res-cleanup"
)

const (
	HoHClusterManagementAddonName               = "hub-of-hubs-controller"
	HoHClusterManagementAddonDisplayName        = "Hub of Hubs Controller"
	HoHClusterManagementAddonDescription        = "Hub of Hubs Controller manages hub-of-hubs components."
	HoHManagedClusterAddonName                  = "hub-of-hubs-controller"
	HoHManagedClusterAddonDisplayName           = "Hub of Hubs Controller"
	HoHManagedClusterAddonDescription           = "Hub of Hubs Controller manages hub-of-hubs components."
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
