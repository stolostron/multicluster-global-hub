package db

// db schemas.
const (
	// StatusSchema schema for status updates.
	StatusSchema = "status"
	// LocalStatusSchema schema for local status updates.
	LocalStatusSchema = "local_status"
	// LocalSpecSchema schema for local spec updates.
	LocalSpecSchema = "local_spec"
)

// table names.
const (
	// ManagedClustersTableName table name of managed clusters.
	ManagedClustersTableName = "managed_clusters"

	// ComplianceTableName table name of policy compliance status.
	ComplianceTableName = "compliance"
	// MinimalComplianceTable table name of minimal policy compliance status.
	MinimalComplianceTable = "aggregated_compliance"
	// LocalPolicySpecTableName table name of local policy spec.
	LocalPolicySpecTableName = "policies"

	// SubscriptionStatusesTableName table name of subscription-statuses.
	SubscriptionStatusesTableName = "subscription_statuses"
	// SubscriptionReportsTableName table name of subscription-reports.
	SubscriptionReportsTableName = "subscription_reports"

	// PlacementRulesTableName table name of placement-rules.
	PlacementRulesTableName = "placementrules"
	// PlacementsTableName table name of placements.
	PlacementsTableName = "placements"
	// PlacementDecisionsTableName table name of placement-decisions.
	PlacementDecisionsTableName = "placementdecisions"

	// LeafHubHeartbeatsTableName table name for LH heartbeats.
	LeafHubHeartbeatsTableName = "leaf_hub_heartbeats"
)

// default values.
const (
	// ErrorNone is default value when no error occurs.
	ErrorNone = "none"
)

// ComplianceStatus represents the possible options for compliance status.
type ComplianceStatus string

// compliance states.
const (
	// NonCompliant non compliant state.
	NonCompliant ComplianceStatus = "non_compliant"
	// Compliant compliant state.
	Compliant ComplianceStatus = "compliant"
	// Unknown unknown compliance state.
	Unknown ComplianceStatus = "unknown"
)

// unique db types.
const (
	// UUID unique type.
	UUID = "uuid"
	// Jsonb unique type.
	Jsonb = "jsonb"
	// ComplianceType unique type.
	ComplianceType = "compliance_type"
)
