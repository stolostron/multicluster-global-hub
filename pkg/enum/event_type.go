package enum

const EventTypePrefix = "io.open-cluster-management.operator.multiclusterglobalhubs."

type EventType string

const (
	HubClusterInfoType      EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.managedhub.info"
	HubClusterHeartbeatType EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.managedhub.heartbeat"
	ManagedClusterType      EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.managedcluster"
	SubscriptionReportType  EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.subscription.report"

	LocalComplianceType         EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.policy.localcompliance"
	LocalCompleteComplianceType EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.policy.localcompletecompliance"
	LocalPolicySpecType         EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.policy.localspec"
	ComplianceType              EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.policy.compliance"
	CompleteComplianceType      EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.policy.completecompliance"

	LocalReplicatedPolicyEventType EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.event.localreplicatedpolicy.update"
	LocalRootPolicyEventType       EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.event.localpolicy.propagate"

	PlacementDecisionType      EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.placementdecision"
	LocalPlacementRuleSpecType EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.placementrule.localspec"
	PlacementRuleSpecType      EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.placementrule.spec"
	PlacementSpecType          EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.placement.spec"
)
