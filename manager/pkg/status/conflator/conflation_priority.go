package conflator

// ConflationPriority sets processing priorities of bundles.
type ConflationPriority uint8

// priority list of conflation unit.
const (
	HubClusterHeartbeatPriority        ConflationPriority = iota
	HubClusterInfoPriority             ConflationPriority = iota
	ManagedClustersPriority            ConflationPriority = iota
	ManagedClusterEventPriority        ConflationPriority = iota
	LocalPolicySpecPriority            ConflationPriority = iota
	LocalCompliancePriority            ConflationPriority = iota
	LocalCompleteCompliancePriority    ConflationPriority = iota
	LocalEventRootPolicyPriority       ConflationPriority = iota
	LocalReplicatedPolicyEventPriority ConflationPriority = iota
	LocalPlacementRulesSpecPriority    ConflationPriority = iota
	SecurityAlertCountsPriority        ConflationPriority = iota
	ManagedClusterMigrationPriority    ConflationPriority = iota

	// enable global resource
	CompliancePriority         ConflationPriority = iota
	CompleteCompliancePriority ConflationPriority = iota
	DeltaCompliancePriority    ConflationPriority = iota
	MinimalCompliancePriority  ConflationPriority = iota

	PlacementRulePriority     ConflationPriority = iota
	PlacementPriority         ConflationPriority = iota
	PlacementDecisionPriority ConflationPriority = iota

	SubscriptionStatusPriority ConflationPriority = iota
	SubscriptionReportPriority ConflationPriority = iota
)
