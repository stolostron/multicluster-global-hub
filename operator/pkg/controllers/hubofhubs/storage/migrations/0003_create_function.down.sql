-- Drop the functions created in the up migration
DROP FUNCTION IF EXISTS public.trigger_set_timestamp();
DROP FUNCTION IF EXISTS public.update_local_compliance_cluster_id();
DROP FUNCTION IF EXISTS public.update_cluster_event_cluster_id();
DROP FUNCTION IF EXISTS public.set_cluster_id_to_local_compliance();
DROP FUNCTION IF EXISTS create_monthly_range_partitioned_table(text, text);
DROP FUNCTION IF EXISTS delete_monthly_range_partitioned_table(text, text);

-- Drop the procedures created in the up migration
DROP PROCEDURE IF EXISTS history.generate_local_compliance(TEXT);
DROP PROCEDURE IF EXISTS history.update_local_compliance(TEXT);

-- Drop the trigger function for updating history compliance by event
DROP FUNCTION IF EXISTS history.update_history_compliance_by_event();
