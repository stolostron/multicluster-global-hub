package constants

// global hub common constants
const (
	// GHDefaultNamespace defines default namespace for ACM hub and Global hub operator and manager
	GHDefaultNamespace = "open-cluster-management"
	// GHSystemNamespace defines global hub system namespace
	GHSystemNamespace = "open-cluster-management-global-hub-system" // GHAgentConfigCMName is the name of configmap that stores important global hub settings
	// eg. aggregationLevel and enableLocalPolicy.
	GHAgentConfigCMName = "multicluster-global-hub-agent-config"
	// GlobalHubSchedulerName - placementrule scheduler name.
	GlobalHubSchedulerName = "global-hub"
	// OpenShift console namespace
	OpenShiftConsoleNamespace = "openshift-console"
	// OpenShift console route name
	OpenShiftConsoleRouteName = "console"
)

const (
	// identify the resource is managed by
	GlobalHubOwnerLabelKey  = "global-hub.open-cluster-management.io/managed-by"
	GlobalHubOwnerLabelVal  = "global-hub"
	GHAgentOwnerLabelValue  = "global-hub-agent"
	GHOperatorOwnerLabelVal = "global-hub-operator"
	// Deprecated identify the resource is a local-resource
	// GlobalHubLocalResource = "global-hub.open-cluster-management.io/local-resource"
	// if the resource with this label, it will be synced to database and then propagated to managed hub
	GlobalHubGlobalResourceLabel = "global-hub.open-cluster-management.io/global-resource"
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
	// cleanup finalizer
	GlobalHubCleanupFinalizer = "global-hub.open-cluster-management.io/resource-cleanup"

	// indicate the removing the global hub finalizer, shouldn't add it back to the resource
	// the value is a timestamp which is the expiration time of this label
	GlobalHubFinalizerRemovingDeadline = "global-hub.open-cluster-management.io/finalizer-removing-deadline"
)

// store hub installation constants
const (
	MultiClusterHubKind = "MultiClusterHub"

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
	// SpecBundle - spec bundle message type.
	SpecBundle = "SpecBundle"
	// StatusBundle - status bundle message type.
	StatusBundle = "StatusBundle"

	// ControlInfoMsgKey - control info message key.
	ControlInfoMsgKey = "ControlInfo"

	// HubClusterInfoMsgKey - hub cluster info message key.
	HubClusterInfoMsgKey = "HubClusterInfo"

	// ManagedClustersMsgKey - managed clusters message key.
	ManagedClustersMsgKey = "ManagedClusters"
	// ManagedClustersLabelsMsgKey - managed clusters labels message key.
	ManagedClustersLabelsMsgKey = "ManagedClustersLabels"

	// ClustersPerPolicyMsgKey - clusters per policy message key.
	ClustersPerPolicyMsgKey = "ClustersPerPolicy"
	// PolicyCompleteComplianceMsgKey - policy complete state compliance message key.
	PolicyCompleteComplianceMsgKey = "PolicyCompleteCompliance"
	// PolicyDeltaComplianceMsgKey - policy delta state compliance message key.
	PolicyDeltaComplianceMsgKey = "PolicyDeltaCompliance"
	// MinimalPolicyComplianceMsgKey - minimal policy compliance message key.
	MinimalPolicyComplianceMsgKey = "MinimalPolicyCompliance"

	// LocalPolicySpecMsgKey - the local policy spec message key.
	LocalPolicySpecMsgKey = "LocalPolicySpec"
	// LocalClustersPerPolicyMsgKey - local clusters per policy message key.
	LocalClustersPerPolicyMsgKey = "LocalClustersPerPolicy"
	// LocalPolicyCompleteComplianceMsgKey - local policy compliance message key.
	LocalPolicyCompleteComplianceMsgKey = "LocalPolicyCompleteCompliance"
	// LocalPlacementRulesMsgKey - local placement rules message key.
	LocalPlacementRulesMsgKey = "LocalPlacementRules"
	// LocalClusterPolicyStatusEventMsgKey - local cluster policy history events message key.
	LocalClusterPolicyStatusEventMsgKey = "LocalClusterPolicyStatusEvents"

	// SubscriptionStatusMsgKey - subscription-status message key.
	SubscriptionStatusMsgKey = "SubscriptionStatus"
	// SubscriptionReportMsgKey - subscription-report message key.
	SubscriptionReportMsgKey = "SubscriptionReport"

	// PlacementRuleMsgKey - placement-rule message key.
	PlacementRuleMsgKey = "PlacementRule"
	// PlacementMsgKey - placement message key.
	PlacementMsgKey = "Placement"
	// PlacementDecisionMsgKey - placement-decision message key.
	PlacementDecisionMsgKey = "PlacementDecision"
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
