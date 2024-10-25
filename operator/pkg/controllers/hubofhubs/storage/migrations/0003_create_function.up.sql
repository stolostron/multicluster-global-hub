-- Create the function to update the `updated_at` field
CREATE OR REPLACE FUNCTION public.trigger_set_timestamp() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION public.update_local_compliance_cluster_id()
    RETURNS TRIGGER
    LANGUAGE plpgsql
AS $$
BEGIN
    UPDATE local_status.compliance
    SET cluster_id = NEW.cluster_id
    WHERE leaf_hub_name = NEW.leaf_hub_name
        AND cluster_name = (NEW.payload -> 'metadata' ->> 'name');    
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION public.update_cluster_event_cluster_id()
    RETURNS TRIGGER
    LANGUAGE plpgsql
AS $$
BEGIN
    UPDATE event.managed_clusters
    SET cluster_id = NEW.cluster_id
    WHERE leaf_hub_name = NEW.leaf_hub_name
        AND cluster_name = (NEW.payload -> 'metadata' ->> 'name')
        AND cluster_id = (New.payload -> 'metadata' ->> 'uid')::uuid;
    RETURN NEW;
END;
$$;

-- Procedure to generate local compliance data for a specific date
CREATE OR REPLACE PROCEDURE history.generate_local_compliance(curr_date TEXT)
LANGUAGE plpgsql
AS $$
DECLARE
    prev_date TEXT;
BEGIN
    prev_date := (curr_date::DATE - INTERVAL '1 day')::TEXT;
    EXECUTE format('
        INSERT INTO history.local_compliance (policy_id, cluster_id, leaf_hub_name, compliance_date, compliance, compliance_changed_frequency)
        SELECT
            policy_id, cluster_id, leaf_hub_name, %1$L, compliance, 0
        FROM
            history.local_compliance
        WHERE compliance_date = %2$L
        ON CONFLICT (leaf_hub_name, policy_id, cluster_id, compliance_date) DO NOTHING
    ', curr_date, prev_date);
END;
$$;

-- Procedure to update compliance for a specific date
CREATE OR REPLACE PROCEDURE history.update_local_compliance(curr_date text)
LANGUAGE plpgsql
AS $$
DECLARE
    next_date TEXT;
BEGIN
    next_date := (curr_date::DATE + INTERVAL '1 day')::TEXT;
    EXECUTE format('
        INSERT INTO history.local_compliance (policy_id, cluster_id, leaf_hub_name, compliance_date, compliance, compliance_changed_frequency)
        WITH compliance_aggregate AS (
            SELECT cluster_id, policy_id, leaf_hub_name,
                CASE
                    WHEN bool_and(compliance = ''compliant'') THEN ''compliant''
                    WHEN bool_and(compliance = ''pending'') THEN ''pending''
                    WHEN bool_and(compliance = ''unknown'') THEN ''unknown''
                    ELSE ''non_compliant''
                END::local_status.compliance_type AS aggregated_compliance
            FROM event.local_policies
            WHERE created_at BETWEEN %1$L::date AND %2$L::date
            GROUP BY cluster_id, policy_id, leaf_hub_name
        )
        SELECT policy_id, cluster_id, leaf_hub_name, %1$L, aggregated_compliance,
            (SELECT COUNT(*) FROM (
                SELECT created_at, compliance, 
                    LAG(compliance) OVER (PARTITION BY cluster_id, policy_id ORDER BY created_at ASC) AS prev_compliance
                FROM event.local_policies lp
                WHERE (lp.created_at BETWEEN %1$L::date AND %2$L::date) 
                    AND lp.cluster_id = ca.cluster_id AND lp.policy_id = ca.policy_id
                ORDER BY created_at ASC
            ) AS subquery WHERE compliance <> prev_compliance) AS compliance_changed_frequency
        FROM compliance_aggregate ca
        ORDER BY cluster_id, policy_id
        ON CONFLICT (leaf_hub_name, policy_id, cluster_id, compliance_date)
        DO UPDATE SET
            compliance = EXCLUDED.compliance,
            compliance_changed_frequency = EXCLUDED.compliance_changed_frequency',
        curr_date, next_date);
END;
$$;

-- Function to create monthly range-partitioned tables
CREATE OR REPLACE FUNCTION create_monthly_range_partitioned_table(full_table_name text, input_time text)
RETURNS VOID AS
$$ 
BEGIN 
    EXECUTE format('CREATE TABLE IF NOT EXISTS %1$s_%2$s PARTITION OF %1$s FOR VALUES FROM (%3$L) TO (%4$L)',
                   full_table_name, 
                   to_char(input_time::date, 'YYYY_MM'),
                   DATE_TRUNC('MONTH', input_time::date),
                   DATE_TRUNC('MONTH', (input_time::date + INTERVAL '1 MONTH'))
                  );
END $$ LANGUAGE plpgsql;

-- Trigger function to update local compliance
CREATE OR REPLACE FUNCTION public.set_cluster_id_to_local_compliance() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  UPDATE local_status.compliance set cluster_id = (
      SELECT cluster_id FROM status.managed_clusters
      WHERE payload -> 'metadata' ->> 'name' = NEW.cluster_name AND leaf_hub_name = NEW.leaf_hub_name AND deleted_at IS NULL
      LIMIT 1
  )
  WHERE cluster_name = NEW.cluster_name AND leaf_hub_name = NEW.leaf_hub_name AND cluster_id IS NULL;
  RETURN NEW;
END;
$$;

-- Function to delete monthly range-partitioned tables
CREATE OR REPLACE FUNCTION delete_monthly_range_partitioned_table(full_table_name text, input_time text)
RETURNS VOID AS
$$ 
BEGIN 
    EXECUTE format('DROP TABLE IF EXISTS %1$s_%2$s',
                   full_table_name, 
                   to_char(input_time::date, 'YYYY_MM')
                  );
END $$ LANGUAGE plpgsql;

-- Trigger function to update history compliance by event
CREATE OR REPLACE FUNCTION history.update_history_compliance_by_event() 
RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO history.local_compliance (
        policy_id, 
        cluster_id, 
        leaf_hub_name, 
        compliance, 
        compliance_date, 
        compliance_changed_frequency
    ) VALUES (
        NEW.policy_id, 
        NEW.cluster_id, 
        NEW.leaf_hub_name, 
        NEW.compliance, 
        NEW.created_at::DATE,
        0
    ) ON CONFLICT (leaf_hub_name, policy_id, cluster_id, compliance_date) 
    DO UPDATE SET 
        compliance = 
            CASE
                WHEN history.local_compliance.compliance = 'pending' OR EXCLUDED.compliance = 'pending' THEN 'pending'::local_status.compliance_type
                WHEN history.local_compliance.compliance = 'unknown' OR EXCLUDED.compliance = 'unknown' THEN 'unknown'::local_status.compliance_type
                WHEN history.local_compliance.compliance = 'non_compliant' OR EXCLUDED.compliance = 'non_compliant' THEN 'non_compliant'::local_status.compliance_type
                ELSE 'compliant'::local_status.compliance_type
            END,
        compliance_changed_frequency = 
            CASE 
                WHEN history.local_compliance.compliance <> EXCLUDED.compliance THEN history.local_compliance.compliance_changed_frequency + 1 
                ELSE history.local_compliance.compliance_changed_frequency 
            END;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
