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
	HOHDefaultNamespace        = "open-cluster-management"
	HOHSystemNamespace         = "open-cluster-management-global-hub-system"
	HOHConfigName              = "multicluster-global-hub-config"
	LocalClusterName           = "local-cluster"
	DefaultImagePullSecretName = "multiclusterhub-operator-pull-secret"
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
)

const (
	HOHHubSubscriptionWorkSuffix = "mgh-hub-subscription"
	HoHHubMCHWorkSuffix          = "mgh-hub-mch"
	HoHHostingHubWorkSuffix      = "mgh-hub-hosting"
	HoHHostedHubWorkSuffix       = "mgh-hub-hosted"
	HoHHostingAgentWorkSuffix    = "mgh-agent-hosting"
	HoHHostedAgentWorkSuffix     = "mgh-agent-hosted"
	HoHAgentWorkSuffix           = "mgh-agent"
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
	// AnnotationMGHSkipDBInit skips database initialization, used for dev/test
	AnnotationMGHSkipDBInit = "mgh-skip-database-init"
	// AnnotationImageRepo sits in MulticlusterGlobalHub annotations
	// to identify a custom image repository to use
	AnnotationImageRepo = "mgh-image-repository"
	// AnnotationImageOverridesCM sits in MulticlusterGlobalHub annotations
	// to identify a custom configmap containing image overrides
	AnnotationImageOverridesCM = "mgh-image-overrides-cm"
	// MGHOperandImagePrefix ...
	MGHOperandImagePrefix = "OPERAND_IMAGE_"
)

const (
	// AnnotationHostingClusterName is the annotation for indicating the hosting cluster name
	AnnotationHostingClusterName = "addon.open-cluster-management.io/hosting-cluster-name"
)
