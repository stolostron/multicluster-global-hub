package enum

const EventTypePrefix = "io.open-cluster-management.operator.multiclusterglobalhubs."

type EventType string

const (
	HubClusterInfoType        EventType = EventTypePrefix + "managedhub.info"
	HubClusterHeartbeatType   EventType = EventTypePrefix + "managedhub.heartbeat"
	KlusterletAddonConfigType EventType = EventTypePrefix + "managedcluster.klusterletaddonconfig"
	ManagedClusterType        EventType = EventTypePrefix + "managedcluster"
	ManagedClusterInfoType    EventType = EventTypePrefix + "managedclusterinfo"
	SubscriptionReportType    EventType = EventTypePrefix + "subscription.report"
	SubscriptionStatusType    EventType = EventTypePrefix + "subscription.status"

	// used by the local resources
	LocalComplianceType         EventType = EventTypePrefix + "policy.localcompliance"
	LocalCompleteComplianceType EventType = EventTypePrefix + "policy.localcompletecompliance"
	LocalPolicySpecType         EventType = EventTypePrefix + "policy.localspec"

	// used by the global resources
	ComplianceType         EventType = EventTypePrefix + "policy.compliance"
	CompleteComplianceType EventType = EventTypePrefix + "policy.completecompliance"
	DeltaComplianceType    EventType = EventTypePrefix + "policy.deltacompliance"
	MiniComplianceType     EventType = EventTypePrefix + "policy.minicompliance"

	// used to send kube events
	LocalReplicatedPolicyEventType EventType = EventTypePrefix + "event.localreplicatedpolicy"
	LocalRootPolicyEventType       EventType = EventTypePrefix + "event.localrootpolicy"

	ManagedClusterEventType       EventType = EventTypePrefix + "event.managedcluster"
	ClusterGroupUpgradesEventType EventType = EventTypePrefix + "event.clustergroupupgrade"

	PlacementDecisionType      EventType = EventTypePrefix + "placementdecision"
	LocalPlacementRuleSpecType EventType = EventTypePrefix + "placementrule.localspec"
	PlacementRuleSpecType      EventType = EventTypePrefix + "placementrule.spec"
	PlacementSpecType          EventType = EventTypePrefix + "placement.spec"

	// Used to send security alerts:
	SecurityAlertCountsType EventType = EventTypePrefix + "security.alertcounts"
)
