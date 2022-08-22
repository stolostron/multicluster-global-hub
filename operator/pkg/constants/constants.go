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
	HOHConfigName       = "multicluster-global-hub-config"
	LocalClusterName    = "local-cluster"
)

const (
	HoHClusterManagementAddonName        = "multicluster-global-hub-controller"
	HoHClusterManagementAddonDisplayName = "Multicluster Global Hub Controller"
	HoHClusterManagementAddonDescription = "Multicluster Global Hub Controller " +
		"manages multicluster-global-hub components."
	HoHManagedClusterAddonName        = "multicluster-global-hub-controller"
	HoHManagedClusterAddonDisplayName = "Multicluster Global Hub Controller"
	HoHManagedClusterAddonDescription = "Multicluster Global Hub Controller " +
		"manages multicluster-global-hub components."
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
	// AnnotationHubACMSnapshot sits in MulticlusterGlobalHub annotations
	// to identify the ACM image snapshot for regional hub
	AnnotationHubACMSnapshot = "mgh-hub-ACM-snapshot"
	// AnnotationHubMCESnapshot sits in MulticlusterGlobalHub annotations
	// to identify the MCE image snapshot for regional hub
	AnnotationHubMCESnapshot = "mgh-hub-MCE-snapshot"
	// AnnotationKafkaBootstrapServer sits in MulticlusterGlobalHub annotations
	// to identify kafka server address used to dev/test
	AnnotationKafkaBootstrapServer = "mgh-kafka-bootstrap-server"
	// AnnotationMCHPause sits in MulticlusterGlobalHub annotations
	// to identify if the MulticlusterGlobalHub is paused or not
	AnnotationMGHPause = "mgh-pause"
	// AnnotationImageRepo sits in MulticlusterGlobalHub annotations
	// to identify a custom image repository to use
	AnnotationImageRepo = "mgh-image-repository"
	// AnnotationImageOverridesCM sits in MulticlusterGlobalHub annotations
	// to identify a custom configmap containing image overrides
	AnnotationImageOverridesCM = "mgh-image-overrides-cm"
	// MGHOperandImagePrefix ...
	MGHOperandImagePrefix = "OPERAND_IMAGE_"
)
