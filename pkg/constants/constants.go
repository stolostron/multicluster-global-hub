package constants

const (
	// HohSystemNamespace - Hub of Hubs dedicated namespace.
	HohSystemNamespace = "hoh-system"

	// OriginOwnerReferenceAnnotation - Origin owner reference annotation.
	OriginOwnerReferenceAnnotation = "hub-of-hubs.open-cluster-management.io/originOwnerReferenceUid"
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

	// hubOfHubsLocalResource - local-resource label
	HubOfHubsLocalResource = "hub-of-hubs.open-cluster-management.io/local-resource"
)
