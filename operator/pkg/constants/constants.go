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
	// ControllerLeaderElectionConfig allows customizing LeaseDuration, RenewDeadline and RetryPeriod
	// for operator, manager and agent via the ConfigMap
	ControllerLeaderElectionConfig = "controller-leader-election-configmap"
)

// annotations for MGH CR
const (
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
	// AnnotationMGHSkipAuth sits in MulticlusterGlobalHub annotations
	// to skip auth for non-k8s api. It is only using for test.
	AnnotationMGHSkipAuth = "mgh-skip-auth"
	// AnnotationMGHSchedulerInterval sits in MulticlusterGlobalHub annotations
	// to identify the scheduler interval for moving policy compliance history
	// valid value can be "month, week, day, hour, minute, second"
	AnnotationMGHSchedulerInterval = "mgh-scheduler-interval"
	// MGHOperandImagePrefix ...
	MGHOperandImagePrefix = "RELATED_IMAGE_"
	// AnnotationStatisticInterval to log the interval of statistic log
	AnnotationStatisticInterval = "mgh-statistic-interval"
)

// hub installation constants
const (
	LocalClusterName = "local-cluster"

	OpenshiftMarketPlaceNamespace = "openshift-marketplace"
	ACMSubscriptionPublicSource   = "redhat-operators"
	ACMSubscriptionPrivateSource  = "acm-custom-registry"
	ACMPackageManifestName        = "advanced-cluster-management"
	MCEPackageManifestName        = "multicluster-engine"
)

// global hub agent constants
const (
	GHClusterManagementAddonName = "multicluster-global-hub-controller"
	GHManagedClusterAddonName    = "multicluster-global-hub-controller"
)

// global hub names
const (
	GHManagerDeploymentName = "multicluster-global-hub-manager"
	GHGrafanaDeploymentName = "multicluster-global-hub-grafana"
)

// global hub transport and storage secret names
const (
	GHTransportSecretName     = "multicluster-global-hub-transport" // #nosec G101
	GHStorageSecretName       = "multicluster-global-hub-storage"   // #nosec G101
	GHDefaultStorageRetention = "18m"                               // 18 months
)

// global hub metrics
const (
	GHServiceMonitorNamespace = "openshift-monitoring"
	GHServiceMonitorName      = "multicluster-global-hub-metrics"
)

const (
	CustomAlertName      = "grafana-alerting-acm-global-custom-alerting"
	CustomGrafanaIniName = "multicluster-global-hub-custom-grafana-config"
)

const (
	// AnnotationAddonHostingClusterName is the annotation for indicating the hosting cluster name in the addon
	AnnotationAddonHostingClusterName = "addon.open-cluster-management.io/hosting-cluster-name"
	// AnnotationClusterHostingClusterName is the annotation for indicating the hosting cluster name in the cluster
	AnnotationClusterHostingClusterName        = "import.open-cluster-management.io/hosting-cluster-name"
	AnnotationClusterDeployMode                = "import.open-cluster-management.io/klusterlet-deploy-mode"
	AnnotationClusterKlusterletDeployNamespace = "import.open-cluster-management.io/klusterlet-namespace"
	ClusterDeployModeHosted                    = "Hosted"
	ClusterDeployModeDefault                   = "Default"

	// GHAgentDeployModeLabelKey is to indicate which deploy mode the agent is installed.
	GHAgentDeployModeLabelKey = "global-hub.open-cluster-management.io/agent-deploy-mode"
	// GHAgentDeployModeHosted is to install agent in Hosted mode
	GHAgentDeployModeHosted = "Hosted"
	// GHAgentDeployModeDefault is to install agent in Default mode
	GHAgentDeployModeDefault = "Default"
	// GHAgentDeployModeNone is to not install agent
	GHAgentDeployModeNone   = "None"
	GHAgentInstallNamespace = "open-cluster-management-agent-addon"

	// GHAgentInstallACMHubLabelKey is to indicate whether to install ACM hub on the agent
	GHAgentACMHubInstallLabelKey = "global-hub.open-cluster-management.io/hub-cluster-install"
)

// AggregationLevel specifies the level of aggregation leaf hubs should do before sending the information
// Enum=full;minimal
type AggregationLevel string

const (
	// FullAggregation is an AggregationLevel
	FullAggregation AggregationLevel = "full"
	// MinimalAggregation is an AggregationLevel
	MinimalAggregation AggregationLevel = "minimal"
)

// MessageCompressionType specifies the compression type of transport message between global hub and managed hubs
// Enum=gzip;no-op
type MessageCompressionType string

const (
	// GzipCompressType is an MessageCompressionType
	GzipCompressType MessageCompressionType = "gzip"
	// NoopCompressType is an MessageCompressionType
	NoopCompressType MessageCompressionType = "no-op"
)
