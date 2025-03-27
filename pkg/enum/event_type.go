package enum

const EventTypePrefix = "io.open-cluster-management.operator.multiclusterglobalhubs."

type EventType string

const (
	HubClusterInfoType        EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.managedhub.info"
	HubClusterHeartbeatType   EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.managedhub.heartbeat"
	KlusterletAddonConfigType EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.managedcluster.klusterletaddonconfig"
	ManagedClusterType        EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.managedcluster"
	ManagedClusterInfoType    EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.managedclusterinfo"
	SubscriptionReportType    EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.subscription.report"
	SubscriptionStatusType    EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.subscription.status"

	// used by the local resources
	//nolint: go:S103
	LocalComplianceType EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.policy.localcompliance"
	//nolint: go:S103
	LocalCompleteComplianceType EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.policy.localcompletecompliance"
	LocalPolicySpecType         EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.policy.localspec"

	// used by the global resources
	ComplianceType EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.policy.compliance"
	//nolint: go:S103
	CompleteComplianceType EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.policy.completecompliance"

	DeltaComplianceType EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.policy.deltacompliance"
	MiniComplianceType  EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.policy.minicompliance"

	// used to send kube events
	//nolint: go:S103
	LocalReplicatedPolicyEventType EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.event.localreplicatedpolicy"
	//nolint: go:S103
	LocalRootPolicyEventType      EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.event.localrootpolicy"
	ManagedClusterEventType       EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.event.managedcluster"
	ClusterGroupUpgradesEventType EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.event.clustergroupupgrade"

	PlacementDecisionType EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.placementdecision"
	//nolint: go:S103
	LocalPlacementRuleSpecType EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.placementrule.localspec"
	PlacementRuleSpecType      EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.placementrule.spec"
	PlacementSpecType          EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.placement.spec"

	// Used to send security alerts:
	SecurityAlertCountsType EventType = "io.open-cluster-management.operator.multiclusterglobalhubs.security.alertcounts"
)
