package constants

const (
	// HohSystemNamespace - Hub of Hubs dedicated namespace.
	HohSystemNamespace = "open-cluster-management-global-hub-system"
	HoHConfigName      = "multicluster-global-hub-config"
)

// message types
const (
	// SpecBundle - spec bundle message type.
	SpecBundle = "SpecBundle"
	// StatusBundle - status bundle message type.
	StatusBundle = "StatusBundle"

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

	// ControlInfoMsgKey - control info message key.
	ControlInfoMsgKey = "ControlInfo"
)

// store all the labels
const (
	// control if need to install global-hub-agent and regional hub or not
	RegionalHubTypeLabelKey = "global-hub.open-cluster-management.io/regional-hub-type"
	// NoHubInstall stands up the global hub won't install ACM hub into the managed cluster
	RegionalHubTypeNoHubInstall = "NoHubInstall"
	// NoHubAgentInstall stands up the managed cluster won't be managed by the global hub.
	RegionalHubTypeNoHubAgentInstall = "NoHubAgentInstall"

	// identify the resource is managed by
	GlobalHubOwnerLabelKey   = "global-hub.open-cluster-management.io/managed-by"
	GlobalHubOwnerLabelVal   = "global-hub"
	HoHOperatorOwnerLabelVal = "multicluster-global-hub-operator"
	HoHAgentOwnerLabelValue  = "multicluster-global-hub-agent"

	// identify the resource is a local-resource
	GlobalHubLocalResource = "global-hub.open-cluster-management.io/local-resource"
)

// store all the annotations
const (
	// identify the managed cluster is managed by the specified regional hub cluster
	ManagedClusterManagedByAnnotation = "global-hub.open-cluster-management.io/managed-by"

	// identify the resource is from the global hub cluster
	OriginOwnerReferenceAnnotation = "global-hub.open-cluster-management.io/origin-ownerreference-uid"

	// This is a temporary annotation to be used to skip console installation during e2e environment setup
	GlobalHubSkipConsoleInstallAnnotationKey = "global-hub.open-cluster-management.io/skip-console-install"
)

// store all the finalizers
const (
	// cleanup finalizer
	GlobalHubCleanupFinalizer = "global-hub.open-cluster-management.io/resource-cleanup"
)
