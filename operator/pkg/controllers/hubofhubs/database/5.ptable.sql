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

CREATE TABLE IF NOT EXISTS event.data_retention_job_log (
    name varchar(63) NOT NULL,
    start_at timestamp NOT NULL DEFAULT now(),
    end_at timestamp NOT NULL DEFAULT now(),
    error TEXT
);