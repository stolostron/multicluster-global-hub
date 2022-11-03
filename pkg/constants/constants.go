package constants

// global hub common constants
const (
	// GHDefaultNamespace defines default namespace for ACM hub and Global hub operator and manager
	GHDefaultNamespace = "open-cluster-management"
	// GHSystemNamespace defines global hub system namespace
	GHSystemNamespace = "open-cluster-management-global-hub-system"
	// GHAgentLocalNamespace defines global hub agent local namespace
	GHAgentLocalNamespace = "open-cluster-management-global-hub-local"
	// GHConfigCMName is the name of configmap that stores important global hub settings
	// eg. aggregationLevel and enableLocalPolicy.
	GHConfigCMName = "multicluster-global-hub-config"
	// GHAgentIncarnationCMName is the name of incarnation configmap for global hub agent
	GHAgentIncarnationCMName = "incarnation-config"
	// GHAgentIncarnationCMName is the key of incarnation configmap data for global hub agent
	GHAgentIncarnationCMKey = "incarnation"
	// GlobalHubSchedulerName - placementrule scheduler name.
	GlobalHubSchedulerName = "global-hub"
)

const (
	// identify the resource is managed by
	GlobalHubOwnerLabelKey  = "global-hub.open-cluster-management.io/managed-by"
	GlobalHubOwnerLabelVal  = "global-hub"
	GHAgentOwnerLabelValue  = "global-hub-agent"
	GHOperatorOwnerLabelVal = "global-hub-operator"
	// identify the resource is a local-resource
	GlobalHubLocalResource = "global-hub.open-cluster-management.io/local-resource"
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

	// indicate the removing the global hub finalizer, shouldn't add it back to the resource
	// the value is a timestamp which is the expiration time of this label
	GlobalHubFinalizerRemovingDeadline = "global-hub.open-cluster-management.io/finalizer-removing-deadline"
)

// store hub installation constants
const (
	MultiClusterHubKind = "MultiClusterHub"

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

// message types
const (
	// SpecBundle - spec bundle message type.
	SpecBundle = "SpecBundle"
	// StatusBundle - status bundle message type.
	StatusBundle = "StatusBundle"

	// ControlInfoMsgKey - control info message key.
	ControlInfoMsgKey = "ControlInfo"

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
)
