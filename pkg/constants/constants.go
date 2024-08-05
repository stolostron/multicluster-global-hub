package constants

// global hub common constants
const (
	// GHDefaultNamespace defines default namespace for ACM hub and Global hub operator and manager
	GHDefaultNamespace = "multicluster-global-hub"
	// GHAgentNamespace defines global hub agent namespace
	GHAgentNamespace = "multicluster-global-hub-agent"
	// ManagerDeploymentName define the global hub manager deployment name
	ManagerDeploymentName = "multicluster-global-hub-manager"
	// AgentDeploymentName define the global hub agent deployment name
	AgentDeploymentName = "multicluster-global-hub-agent"

	// GHAgentConfigCMName is the name of configmap that stores important global hub settings
	// eg. aggregationLevel and enableLocalPolicy.
	GHAgentConfigCMName = "multicluster-global-hub-agent-config"
	// GlobalHubSchedulerName - placementrule scheduler name.
	GlobalHubSchedulerName = "global-hub"
	// OpenShift console namespace
	OpenShiftConsoleNamespace = "openshift-console"
	// OpenShift console route name
	OpenShiftConsoleRouteName = "console"
	// Observability grafana namespace
	ObservabilityNamespace = "open-cluster-management-observability"
	// Observability grafana route name
	ObservabilityGrafanaRouteName = "grafana"

	DefaultAgentQPS   = float32(150)
	DefaultAgentBurst = 300

	DefaultClusterId = "00000000-0000-0000-0000-000000000000"

	BackupKey             = "cluster.open-cluster-management.io/backup"
	BackupVolumnKey       = "cluster.open-cluster-management.io/backup-hub-pvc"
	BackupExcludeKey      = "velero.io/exclude-from-backup"
	BackupActivationValue = "cluster-activation"
	BackupGlobalHubValue  = "globalhub"

	PostgresPvcLabelKey   = "component"
	PostgresPvcLabelValue = "multicluster-global-hub-operator"

	// These annotation used to do prehook when backup pvc, which is provided by volsync
	BackupPvcCopyTrigger       = "volsync.backube/copy-trigger"
	BackupPvcUserCopyTrigger   = "volsync.backube/use-copy-trigger"
	BackupPvcLatestCopyStatus  = "volsync.backube/latest-copy-status"
	BackupPvcWaitingForTrigger = "WaitingForTrigger"
	BackupPvcCompletedTrigger  = "Completed"
	BackupPvcLatestCopyTrigger = "volsync.backube/latest-copy-trigger"
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
)

const (
	LocalClusterName = "local-cluster"
	// lock the database
	LockId = "1"
)

// global hub transport and storage secret and configmap names
const (
	GHTransportSecretName      = "multicluster-global-hub-transport" // #nosec G101
	GHStorageSecretName        = "multicluster-global-hub-storage"   // #nosec G101
	GHBuiltInStorageSecretName = "multicluster-global-hub-postgres"  // #nosec G101
	KafkaCertSecretName        = "kafka-certs-secret"                // #nosec G101
	GHDefaultStorageRetention  = "18m"                               // 18 months
	PostgresCAConfigMap        = "multicluster-global-hub-postgres-ca"
)

// the global hub transport config secret for manager and agent
const (
	GHTransportConfigSecret = "transport-config" // #nosec G101
)

// global hub console secret/configmap names
const (
	CustomAlertName      = "multicluster-global-hub-custom-alerting"
	CustomGrafanaIniName = "multicluster-global-hub-custom-grafana-config"
)

const (
	// identify the resource is managed by
	GlobalHubOwnerLabelKey      = "global-hub.open-cluster-management.io/managed-by"
	GlobalHubOwnerLabelVal      = "global-hub"
	GlobalHubAddonOwnerLabelVal = "global-hub-addon"
	GHAgentOwnerLabelValue      = "global-hub-agent"
	GHOperatorOwnerLabelVal     = "global-hub-operator"
	// Deprecated identify the resource is a local-resource
	// GlobalHubLocalResource = "global-hub.open-cluster-management.io/local-resource"
	// if the resource with this label, it will be synced to database and then propagated to managed hub
	GlobalHubGlobalResourceLabel = "global-hub.open-cluster-management.io/global-resource"
	GlobalHubMetricsLabel        = "global-hub.open-cluster-management.io/metrics-resource"
)

// store all the annotations
const (
	// identify the managed cluster is managed by the specified managed hub cluster
	ManagedClusterManagedByAnnotation = "global-hub.open-cluster-management.io/managed-by"
	// identify the resource is from the global hub cluster
	OriginOwnerReferenceAnnotation = "global-hub.open-cluster-management.io/origin-ownerreference-uid"
)

// store all the finalizers
const (
	// The finalizer is only for the global resource. The finalizer will be added if it has the
	// GlobalHubGlobalResourceLabel or OriginOwnerReferenceAnnotation
	GlobalHubCleanupFinalizer = "global-hub.open-cluster-management.io/resource-cleanup"

	// indicate the removing the global hub finalizer, shouldn't add it back to the resource
	// the value is a timestamp which is the expiration time of this label
	GlobalHubFinalizerRemovingDeadline = "global-hub.open-cluster-management.io/finalizer-removing-deadline"
)

// store hub installation constants
const (
	MultiClusterHubKind = "MultiClusterHub"
	ManagedClusterKind  = "ManagedCluster"

	// VersionClusterClaimName is a claim to record the ACM version
	VersionClusterClaimName = "version.open-cluster-management.io"
	// HubClusterClaimName is a claim to record the ACM Hub
	HubClusterClaimName = "hub.open-cluster-management.io"

	// the value of the HubClusterClaimName ClusterClaim
	HubNotInstalled         = "NotInstalled"
	HubInstalledByUser      = "InstalledByUser"
	HubInstalledByGlobalHub = "InstalledByGlobalHub"
)

// message types
const (
	ResyncMsgKey = "Resync"

	// ManagedClustersLabelsMsgKey - managed clusters labels message key.
	ManagedClustersLabelsMsgKey = "ManagedClustersLabels"

	// GenericSpecMsgKey is the generic spec message key for the bundle
	GenericSpecMsgKey = "Generic"
)

// event exporter reference object label keys
const (
	// the label is added by the event exporter
	PolicyEventRootPolicyIdLabelKey = "policy.open-cluster-management.io/root-policy-id"
	// the label is added by the event exporter
	PolicyEventClusterIdLabelKey = "policy.open-cluster-management.io/cluster-id"
	// the label is added by the event exporter
	PolicyEventComplianceLabelKey = "policy.open-cluster-management.io/compliance"

	// the label is from the reference object itself
	PolicyEventRootPolicyNameLabelKey = "policy.open-cluster-management.io/root-policy"
	// the label is from the reference object itself
	PolicyEventClusterNameLabelKey = "policy.open-cluster-management.io/cluster-name"
)

const (
	// Managedcluster imported status type
	ManagedClusterImportSucceeded = "ManagedClusterImportSucceeded"
)
