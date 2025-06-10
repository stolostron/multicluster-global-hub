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
	// ControllerConfig allows customizing LeaseDuration, RenewDeadline and RetryPeriod
	// for operator, manager and agent via the ConfigMap
	ControllerConfig = "controller-config"
	// Global Hub Controller Name
	GlobalHubControllerName = "global-hub-controller"
)

// global hub metrics
const (
	// metrics monitoring namespace label
	ClusterMonitoringLabelKey = "openshift.io/cluster-monitoring"
	ClusterMonitoringLabelVal = "true"
	GHServiceMonitorName      = "multicluster-global-hub-metrics"
)

// annotations for MGH CR
const (
	// AnnotationMCHPause sits in MulticlusterGlobalHub annotations
	// to identify if the MulticlusterGlobalHub is paused or not
	AnnotationMGHPause = "mgh-pause"
	// AnnotationLaunchJobNames will exec the job once the container restart, used for dev/test
	AnnotationLaunchJobNames = "mgh-launch-job-names"
	// AnnotationImageRepo sits in MulticlusterGlobalHub annotations
	// to identify a custom image repository to use
	AnnotationImageRepo = "mgh-image-repository"
	// AnnotationImageOverridesCM sits in MulticlusterGlobalHub annotations
	// to identify a custom configmap containing image overrides
	AnnotationImageOverridesCM = "mgh-image-overrides-cm"
	// AnnotationMGHSkipAuth sits in MulticlusterGlobalHub annotations
	// to skip auth for non-k8s api. It is only using for test.
	AnnotationMGHSkipAuth = "mgh-skip-auth"
	// AnnotationMGHInstallCrunchyOperator installs crunchy operator to provide postgres
	AnnotationMGHInstallCrunchyOperator = "mgh-install-crunchy-operator"
	// AnnotationMGHSchedulerInterval sits in MulticlusterGlobalHub annotations
	// to identify the scheduler interval for moving policy compliance history
	// valid value can be "month, week, day, hour, minute, second"
	AnnotationMGHSchedulerInterval = "mgh-scheduler-interval"
	// MGHOperandImagePrefix ...
	MGHOperandImagePrefix = "RELATED_IMAGE_"
	// AnnotationStatisticInterval to log the interval of statistic log
	AnnotationStatisticInterval = "mgh-statistic-interval"
	// AnnotationMetricsScrapeInterval to set the scrape interval for metrics
	AnnotationMetricsScrapeInterval = "mgh-metrics-scrape-interval"
	// AnnotationONMulticlusterHub indicates the addons are running on a hub cluster
	AnnotationONMulticlusterHub = "addon.open-cluster-management.io/on-multicluster-hub"
	// AnnotationPolicyONMulticlusterHub indicates the policy spec sync is running on a hub cluster
	AnnotationPolicyONMulticlusterHub = "policy.open-cluster-management.io/sync-policies-on-multicluster-hub"
	// AnnotationMGHWithInventory indicates the inventory is deployed
	AnnotationMGHWithInventory = "global-hub.open-cluster-management.io/with-inventory"
	// AnnotationMGHWithStackroxIntegration indicates that the integration with Stackrox is enabled.
	AnnotationMGHWithStackroxIntegration = "global-hub.open-cluster-management.io/with-stackrox-integration"
	// AnnotationMGHWithStackroxPollInterval specifies the StackRox API poll interval. This is intended mostly for
	// development environments, where is is convenient to reduce the poll interval. The value should be a string
	// that can be parsed with the time.ParseDuration function.
	AnnotationMGHWithStackroxPollInterval = "global-hub.open-cluster-management.io/with-stackrox-poll-interval"
)

// hub installation constants
const (
	OpenshiftMarketPlaceNamespace = "openshift-marketplace"
	ACMSubscriptionPublicSource   = "redhat-operators"
	ACMSubscriptionPrivateSource  = "acm-custom-registry"
	ACMPackageManifestName        = "advanced-cluster-management"
	MCEPackageManifestName        = "multicluster-engine"
)

// global hub names
const (
	GHManagerDeploymentName = "multicluster-global-hub-manager"
	GHGrafanaDeploymentName = "multicluster-global-hub-grafana"
)

const (
	GHAgentInstallNamespace = "open-cluster-management-agent-addon"

	// GHAgentInstallACMHubLabelKey is to indicate whether to install ACM hub on the agent
	GHAgentACMHubInstallLabelKey = "global-hub.open-cluster-management.io/hub-cluster-install"

	// CatalogSourceNameKey defines the catalog source name. it is mainly used for deploy kafka in KinD cluster.
	// Customer may also need to set this value if they want to use a custom catalog source for strimzi.
	CatalogSourceNameKey = "global-hub.open-cluster-management.io/strimzi-catalog-source-name"
	// CatalogSourceNamespaceKey defines the catalog source namespace.
	// It is mainly used for deploying kafka in KinD cluster.
	// Customer may also need to set this value if they want to use a custom catalog source for strimzi.
	CatalogSourceNamespaceKey = "global-hub.open-cluster-management.io/strimzi-catalog-source-namespace"

	// SubscriptionPackageName defines the subscription package name for strimzi.
	SubscriptionPackageName = "global-hub.open-cluster-management.io/strimzi-subscription-package-name"
	// SubscriptionChannel defines the subscription channel for strimzi.
	SubscriptionChannel = "global-hub.open-cluster-management.io/strimzi-subscription-channel"

	// KinDClusterIPKey defines a KinD container host which is used for test.
	// It will be inject to the server certificates of kafka and inventory
	KinDClusterIPKey = "global-hub.open-cluster-management.io/kind-cluster-ip"
	// KafkaUseNodeport indicates that Kafka is exposed via NodePort, and it is intended for testing purposes.
	KafkaUseNodeport = "global-hub.open-cluster-management.io/kafka-use-nodeport"
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

// default resources for each component
const (

	// default resources for postgres
	Postgres              = "postgres"
	PostgresMemoryLimit   = "4Gi"
	PostgresMemoryRequest = "128Mi"
	PostgresCPURequest    = "25m"

	// default resources for manager
	Manager              = "manager"
	ManagerMemoryLimit   = "600Mi"
	ManagerMemoryRequest = "100Mi"
	ManagerCPURequest    = "100m"

	// default resources for agent
	Agent              = "agent"
	AgentMemoryLimit   = "1200Mi"
	AgentMemoryRequest = "200Mi"
	AgentCPURequest    = "10m"

	// default resources for grafana
	Grafana              = "grafana"
	GrafanaMemoryLimit   = "1Gi"
	GrafanaCPULimit      = "500m"
	GrafanaMemoryRequest = "100Mi"
	GrafanaCPURequest    = "4m"

	// default resources for kafka
	Kafka              = "kafka"
	KafkaMemoryLimit   = "4Gi"
	KafkaMemoryRequest = "128Mi"
	KafkaCPURequest    = "25m"
)

const (
	OauthProxyImageStreamName      = "oauth-proxy"
	OauthProxyImageStreamNamespace = "openshift"
)
