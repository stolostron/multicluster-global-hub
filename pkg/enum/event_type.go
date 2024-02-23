package enum

type EventType string

const (
	HubClusterInfoType EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.managedhub.info"
	ManagedClusterType EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.managedcluster"

	LocalPolicyComplianceType         EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.policy.localcompliance"
	LocalPolicyCompleteComplianceType EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.policy.localcompletecompliance"
	LocalPolicySpecType               EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.policy.localspec"
	PolicyComplianceType              EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.policy.compliance"
	PolicyCompleteComplianceType      EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.policy.completecompliance"

	LocalReplicatedPolicyEventType EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.event.localreplicatedpolicy.update"
	LocalRootPolicyEventType       EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.event.localpolicy.propagate"

	PlacementDecisionType      EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.placementdecision"
	LocalPlacementRuleSpecType EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.placementrule.localspec"
	PlacementRuleSpecType      EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.placementrule.spec"
	PlacementSpecType          EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.placement.spec"
)
