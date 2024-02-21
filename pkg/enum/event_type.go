package enum

type EventType string

const (
	ManagedClusterType                EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.managedcluster"
	LocalPolicyComplianceType         EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.policy.localcompliance"
	LocalPolicyCompleteComplianceType EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.policy.localcompletecompliance"
	LocalPolicySpecType               EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.policy.localspec"
	LocalReplicatedPolicyEventType    EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.event.localreplicatedpolicy.update"
	LocalRootPolicyEventType          EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.event.localpolicy.propagate"
)
