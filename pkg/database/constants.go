package database

// db schemas.
const (
	// StatusSchema schema for status updates.
	StatusSchema = "status"
	// LocalStatusSchema schema for local status updates.
	LocalStatusSchema = "local_status"
	// LocalSpecSchema schema for local spec updates.
	LocalSpecSchema = "local_spec"
	// EventSchema schema for policy events
	EventSchema = "event"
	// SecuritySchemma schema for security updates.
	SecuritySchema = "security"
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

	// LeafHubHeartbeatsTableName table name for LH heartbeats.
	LeafHubHeartbeatsTableName = "leaf_hub_heartbeats"

	// HubClusterInfo table name of leaf_hubs.
	HubClusterInfoTableName = "leaf_hubs"

	// PolicyEvent table name of leaf_hubs.
	LocalPolicyEventTableName     = "local_policies"
	LocalRootPolicyEventTableName = "local_root_policies"

	// SecurityAlertCountsTable is the name of the table for security alert counts.
	SecurityAlertCountsTable = "alert_counts"
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
	// Pending state
	Pending ComplianceStatus = "pending"
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
