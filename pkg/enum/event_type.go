package enum

type EventType string

const (
	LocalReplicatedPolicyEventType EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.local.replicatedpolicy.update"
	LocalRootPolicyEventType       EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.local.policy.propagate"
)
