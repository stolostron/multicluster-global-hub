-- This script will generate <365> days data in history.local_compliance
do
$$
declare
    all_days int = 365;
    full_table_name text;
    partitiondate text;
begin
    ---create partition table
    for m in 1..all_days/30 loop
        full_table_name = 'history.local_compliance';
        partitiondate = to_char(current_date - m*30, 'YYYY-MM-DD');
        raise notice 'Create partition table for %', partitiondate;
        EXECUTE format('CREATE TABLE IF NOT EXISTS %1$s_%2$s PARTITION OF %1$s FOR VALUES FROM (%3$L) TO (%4$L)',
                    full_table_name, 
                    to_char(partitiondate::date, 'YYYY_MM'),
                    DATE_TRUNC('MONTH', partitiondate::date),
                    DATE_TRUNC('MONTH', (partitiondate::date + INTERVAL '1 MONTH'))
                );
    end loop;

    -- insert data to table
    for m in 1..all_days loop
        raise notice 'Generate % data in history.local_compliance', CURRENT_DATE - m;
        INSERT INTO history.local_compliance (policy_id, cluster_id, leaf_hub_name, compliance, compliance_date) (
					SELECT policy_id, cluster_id, leaf_hub_name, compliance, (CURRENT_DATE - m) 
					FROM 
						local_status.compliance
				)
				ON CONFLICT (leaf_hub_name, policy_id, cluster_id, compliance_date) DO NOTHING;
    end loop;
end;
$$;
