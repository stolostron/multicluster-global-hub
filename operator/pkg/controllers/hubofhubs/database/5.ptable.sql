CREATE TABLE IF NOT EXISTS event.local_policies (
    event_name character varying(63) NOT NULL,
    policy_id uuid NOT NULL,
    cluster_id uuid NOT NULL,
    leaf_hub_name character varying(63) NOT NULL,
    message text,
    reason text,
    count integer NOT NULL DEFAULT 0,
    source jsonb,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    compliance local_status.compliance_type NOT NULL,
    CONSTRAINT local_policies_unique_constraint UNIQUE (event_name, count, created_at)
) PARTITION BY RANGE (created_at);

CREATE TABLE IF NOT EXISTS event.local_root_policies (
    event_name character varying(63) NOT NULL,
    policy_id uuid NOT NULL,
    leaf_hub_name character varying(63) NOT NULL,
    message text,
    reason text,
    count integer NOT NULL DEFAULT 0,
    source jsonb,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    compliance local_status.compliance_type NOT NULL,
    CONSTRAINT local_root_policies_unique_constraint UNIQUE (event_name, count, created_at)
) PARTITION BY RANGE (created_at);

--- create the monthly partitioned tables function by created_at column
--- sample 1: SELECT event.create_partitioned_table('event.local_root_policies', to_char(current_date, 'YYYY-MM-DD'));
--- sample 2: SELECT event.create_partitioned_table('event.local_root_policies', '2023-08-01');
CREATE OR REPLACE FUNCTION event.create_partitioned_table(full_table_name text, input_time text)
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

--- create the current month partitioned tables for local_policies and local_root_policies
SELECT event.create_partitioned_table('event.local_root_policies', to_char(current_date, 'YYYY-MM-DD'));
SELECT event.create_partitioned_table('event.local_policies', to_char(current_date, 'YYYY-MM-DD'));

CREATE TABLE IF NOT EXISTS event.data_retention_job_log (
    table_name varchar(63) NOT NULL,
    start_at timestamp NOT NULL DEFAULT now(),
    end_at timestamp NOT NULL DEFAULT now(),
    min_partition varchar(63), -- minimum partition after the job
    max_partition varchar(63), -- maximum partition after the job
    min_deletion  timestamp, -- the oldest deleted record in the table after the job
    error TEXT
);


