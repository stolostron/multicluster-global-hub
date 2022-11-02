package constants

const (
	// HohSystemNamespace - Hub of Hubs dedicated namespace.
	HohSystemNamespace  = "open-cluster-management-global-hub-system"
	HoHConfigName       = "multicluster-global-hub-config"
	MultiClusterHubKind = "MultiClusterHub"
	// ControllerLeaderElectionConfig allows customizing LeaseDuration, RenewDeadline and RetryPeriod
	// for operator, manager and agent via the ConfigMap
	ControllerLeaderElectionConfig = "controller-leader-election-configmap"
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

	// GlobalHubSchedulerName - placementrule scheduler name.
	GlobalHubSchedulerName = "global-hub"
)

// store all the labels
const (
	// AgentDeployModeLabelKey is to indicate which deploy mode the agent is installed.
	AgentDeployModeLabelKey = "global-hub.open-cluster-management.io/agent-deploy-mode"
	// AgentDeployModeHosted is to install agent in Hosted mode
	AgentDeployModeHosted = "Hosted"
	// AgentDeployModeDefault is to install agent in Default mode
	AgentDeployModeDefault = "Default"
	// AgentDeployModeNone is to not install agent
	AgentDeployModeNone = "None"

	// identify the resource is managed by
	GlobalHubOwnerLabelKey   = "global-hub.open-cluster-management.io/managed-by"
	GlobalHubOwnerLabelVal   = "global-hub"
	HoHAgentOwnerLabelValue  = "global-hub-agent"
	HoHOperatorOwnerLabelVal = "multicluster-global-hub-operator"

	// identify the resource is a local-resource
	GlobalHubLocalResource = "global-hub.open-cluster-management.io/local-resource"

	// indicate the removing the global hub finalizer, shouldn't add it back to the resource
	// the value is a timestamp which is the expiration time of this label
	GlobalHubFinalizerRemovingDeadline = "global-hub.open-cluster-management.io/finalizer-removing-deadline"
)

// store all the annotations
const (
	// identify the managed cluster is managed by the specified regional hub cluster
	ManagedClusterManagedByAnnotation = "global-hub.open-cluster-management.io/managed-by"

	// identify the resource is from the global hub cluster
	OriginOwnerReferenceAnnotation = "global-hub.open-cluster-management.io/origin-ownerreference-uid"
)

// store all the finalizers
const (
	// cleanup finalizer
	GlobalHubCleanupFinalizer = "global-hub.open-cluster-management.io/resource-cleanup"
)

const (
	// VersionClusterClaimName is a claim to record the ACM version
	VersionClusterClaimName = "version.open-cluster-management.io"

	// HubClusterClaimName is a claim to record the ACM Hub
	HubClusterClaimName = "hub.open-cluster-management.io"

	// the value of the HubClusterClaimName ClusterClaim
	HubNotInstalled                   = "NotInstalled"
	HubInstalledWithSelfManagement    = "InstalledWithSelfManagement"
	HubInstalledWithoutSelfManagement = "InstalledWithoutSelfManagement"
	HubInstalledByHoH                 = "InstalledByHoH"
)
