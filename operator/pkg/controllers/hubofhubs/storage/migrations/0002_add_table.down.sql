-- Drop Tables

DROP TABLE IF EXISTS local_spec.policies CASCADE;
DROP TABLE IF EXISTS local_status.compliance CASCADE;
DROP TABLE IF EXISTS status.leaf_hub_heartbeats CASCADE;
DROP TABLE IF EXISTS status.managed_clusters CASCADE;
DROP TABLE IF EXISTS status.leaf_hubs CASCADE;

-- Partition tables
DROP TABLE IF EXISTS event.managed_clusters CASCADE;
DROP TABLE IF EXISTS event.local_policies CASCADE;
DROP TABLE IF EXISTS event.local_root_policies CASCADE;

-- Log tables
DROP TABLE IF EXISTS event.data_retention_job_log CASCADE;

-- History tables
DROP TABLE IF EXISTS history.local_compliance CASCADE;
DROP TABLE IF EXISTS history.local_compliance_job_log CASCADE;

DROP TABLE IF EXISTS status.transport CASCADE;
DROP TABLE IF EXISTS security.alert_counts CASCADE;
