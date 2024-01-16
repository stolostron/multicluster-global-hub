package conflator

// ConflationPriority sets processing priorities of bundles.
type ConflationPriority uint8

// priority list of conflation unit.
const (
	HubClusterHeartbeatPriority        ConflationPriority = iota
	ManagedClustersPriority            ConflationPriority = iota
	CompliancePriority                 ConflationPriority = iota
	CompleteCompliancePriority         ConflationPriority = iota
	DeltaCompliancePriority            ConflationPriority = iota
	MinimalCompliancePriority          ConflationPriority = iota
	HubClusterInfoPriority             ConflationPriority = iota
	LocalPolicySpecPriority            ConflationPriority = iota
	LocalCompliancePriority            ConflationPriority = iota
	LocalCompleteCompliancePriority    ConflationPriority = iota
	LocalReplicatedPolicyEventPriority ConflationPriority = iota

	// enable global resource
	PlacementRulePriority           ConflationPriority = iota
	PlacementPriority               ConflationPriority = iota
	PlacementDecisionPriority       ConflationPriority = iota
	SubscriptionStatusPriority      ConflationPriority = iota
	SubscriptionReportPriority      ConflationPriority = iota
	LocalPlacementRulesSpecPriority ConflationPriority = iota
)
