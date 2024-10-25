-- Down Migration: Drop triggers and partition tables

-- Drop trigger that sets cluster_id to compliance table
DROP TRIGGER IF EXISTS update_compliance_table ON local_status.compliance;

-- Drop trigger that updates compliance cluster_id
DROP TRIGGER IF EXISTS update_local_compliance_cluster_id_trigger ON status.managed_clusters;

-- Drop trigger that updates cluster event cluster_id
DROP TRIGGER IF EXISTS update_cluster_event_cluster_id_trigger ON status.managed_clusters;

-- Drop trigger to update history.local_compliance by event inserts
DROP TRIGGER IF EXISTS trg_update_history_compliance_by_event ON event.local_policies;

-- Drop current month partitioned tables
SELECT delete_monthly_range_partitioned_table('event.local_root_policies', to_char(current_date, 'YYYY-MM-DD'));
SELECT delete_monthly_range_partitioned_table('event.local_policies', to_char(current_date, 'YYYY-MM-DD'));
SELECT delete_monthly_range_partitioned_table('history.local_compliance', to_char(current_date, 'YYYY-MM-DD'));
SELECT delete_monthly_range_partitioned_table('event.managed_clusters', to_char(current_date, 'YYYY-MM-DD'));

-- Drop previous month partitioned tables
SELECT delete_monthly_range_partitioned_table('event.local_root_policies', to_char(current_date - interval '1 month', 'YYYY-MM-DD'));
SELECT delete_monthly_range_partitioned_table('event.local_policies', to_char(current_date - interval '1 month', 'YYYY-MM-DD'));
SELECT delete_monthly_range_partitioned_table('history.local_compliance', to_char(current_date - interval '1 month', 'YYYY-MM-DD'));
SELECT delete_monthly_range_partitioned_table('event.managed_clusters', to_char(current_date - interval '1 month', 'YYYY-MM-DD'));