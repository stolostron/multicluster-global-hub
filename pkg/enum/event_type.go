package enum

type EventType string

const (
	//lint:ignore go:S103
	LocalReplicatedPolicyEventType EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.local.replicatedpolicy.update"
	//lint:ignore go:S103
	LocalRootPolicyEventType EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.local.policy.propagate"
)
