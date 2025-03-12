
CREATE TABLE IF NOT EXISTS local_spec.policies (
    leaf_hub_name character varying(254) NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted_at timestamp without time zone,
    policy_id uuid PRIMARY KEY,
    policy_name character varying(254) generated always as (payload -> 'metadata' ->> 'name') stored,
    policy_standard character varying(254) generated always as (payload -> 'metadata' -> 'annotations' ->> 'policy.open-cluster-management.io/standards') stored,
    policy_category character varying(254) generated always as (payload -> 'metadata' -> 'annotations' ->> 'policy.open-cluster-management.io/categories') stored,
    policy_control character varying(254) generated always as (payload -> 'metadata' -> 'annotations' ->> 'policy.open-cluster-management.io/controls') stored
);
CREATE INDEX IF NOT EXISTS local_policies_deleted_at_idx ON local_spec.policies (deleted_at);
CREATE INDEX IF NOT EXISTS local_policies_leafhub_idx ON local_spec.policies (leaf_hub_name);

CREATE TABLE IF NOT EXISTS local_status.compliance (
    policy_id uuid NOT NULL,
    cluster_name character varying(254) NOT NULL,
    leaf_hub_name character varying(254) NOT NULL,
    error status.error_type NOT NULL,
    compliance local_status.compliance_type NOT NULL,
    cluster_id uuid,
    PRIMARY KEY (policy_id, cluster_name, leaf_hub_name)
);

CREATE TABLE IF NOT EXISTS status.leaf_hub_heartbeats (
    leaf_hub_name character varying(254) NOT NULL,
    last_timestamp timestamp without time zone DEFAULT now() NOT NULL,
    status VARCHAR(10) DEFAULT 'active'
);
CREATE UNIQUE INDEX IF NOT EXISTS leaf_hub_heartbeats_leaf_hub_idx ON status.leaf_hub_heartbeats (leaf_hub_name);
CREATE INDEX IF NOT EXISTS leaf_hub_heartbeats_leaf_hub_timestamp_idx ON status.leaf_hub_heartbeats(last_timestamp);
CREATE INDEX IF NOT EXISTS leaf_hub_heartbeats_leaf_hub_status_idx ON status.leaf_hub_heartbeats(status);

CREATE TABLE IF NOT EXISTS status.managed_clusters (
    leaf_hub_name character varying(254) NOT NULL,
    cluster_name character varying(254) generated always as (payload -> 'metadata' ->> 'name') stored,
    cluster_id uuid PRIMARY KEY,
    payload jsonb NOT NULL,
    error status.error_type NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted_at timestamp without time zone
);
CREATE INDEX IF NOT EXISTS cluster_deleted_at_idx ON status.managed_clusters (deleted_at);
CREATE INDEX IF NOT EXISTS leafhub_cluster_idx ON status.managed_clusters (leaf_hub_name, cluster_name);

CREATE TABLE IF NOT EXISTS status.leaf_hubs (
    leaf_hub_name character varying(254) NOT NULL,
    cluster_id uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    payload jsonb NOT NULL,
    console_url text generated always as (payload ->> 'consoleURL') stored,
    grafana_url text generated always as (payload ->> 'grafanaURL') stored,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted_at timestamp without time zone,
    PRIMARY KEY (cluster_id, leaf_hub_name)
);
CREATE INDEX IF NOT EXISTS leafhub_deleted_at_idx ON status.leaf_hubs (deleted_at);

-- Partition tables
CREATE TABLE IF NOT EXISTS event.managed_clusters (
    event_namespace text NOT NULL,
    event_name text NOT NULL,
    cluster_name text NOT NULL,
    cluster_id uuid NOT NULL,
    leaf_hub_name character varying(256) NOT NULL,
    message text,
    reason text,
    reporting_controller text, 
    reporting_instance text, 
    event_type character varying(64) NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    CONSTRAINT managed_clusters_unique_constraint UNIQUE (leaf_hub_name, event_name, created_at)
) PARTITION BY RANGE (created_at);

CREATE TABLE IF NOT EXISTS event.local_policies (
    event_name text NOT NULL,
    event_namespace text,
    policy_id uuid NOT NULL,
    cluster_id uuid NOT NULL,
    cluster_name text,
    leaf_hub_name character varying(254) NOT NULL,
    message text,
    reason text,
    count integer NOT NULL DEFAULT 0,
    source jsonb,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    compliance local_status.compliance_type NOT NULL,
    CONSTRAINT local_policies_unique_constraint UNIQUE (event_name, count, created_at)
) PARTITION BY RANGE (created_at);

CREATE TABLE IF NOT EXISTS event.local_root_policies (
    event_name text NOT NULL,
    event_namespace text,
    policy_id uuid NOT NULL,
    leaf_hub_name character varying(254) NOT NULL,
    message text,
    reason text,
    count integer NOT NULL DEFAULT 0,
    source jsonb,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    compliance local_status.compliance_type NOT NULL,
    CONSTRAINT local_root_policies_unique_constraint UNIQUE (event_name, count, created_at)
) PARTITION BY RANGE (created_at);

-- log tables
CREATE TABLE IF NOT EXISTS event.data_retention_job_log (
    table_name varchar(254) NOT NULL,
    start_at timestamp NOT NULL DEFAULT now(),
    end_at timestamp NOT NULL DEFAULT now(),
    min_partition varchar(254), -- minimum partition after the job
    max_partition varchar(254), -- maximum partition after the job
    min_deletion  timestamp, -- the oldest deleted record in the table after the job
    error TEXT
);

CREATE TABLE IF NOT EXISTS history.local_compliance (
    policy_id uuid NOT NULL,
    cluster_id uuid,
    leaf_hub_name character varying(254) NOT NULL,
    compliance_date DATE DEFAULT (CURRENT_DATE - INTERVAL '1 day') NOT NULL, 
    compliance local_status.compliance_type NOT NULL,
    compliance_changed_frequency integer NOT NULL DEFAULT 0,
    CONSTRAINT local_policies_unique_constraint UNIQUE (leaf_hub_name, policy_id, cluster_id, compliance_date)
) PARTITION BY RANGE (compliance_date);

CREATE TABLE IF NOT EXISTS history.local_compliance_job_log (
    name varchar(254) NOT NULL,
    start_at timestamp NOT NULL DEFAULT now(),
    end_at timestamp NOT NULL DEFAULT now(),
    total int8,
    inserted int8,
    offsets int8, 
    error TEXT
);

CREATE TABLE IF NOT EXISTS status.transport (
    -- transport name, it is the topic name for the kafka transport
    name character varying(254) PRIMARY KEY,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL
);

CREATE TABLE IF NOT EXISTS security.alert_counts (
    hub_name text NOT NULL,
    low integer NOT NULL,
    medium integer NOT NULL,
    high integer NOT NULL,
    critical integer NOT NULL,
    detail_url text NOT NULL,
    source text NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (hub_name, source)
);

CREATE TABLE IF NOT EXISTS status.managed_cluster_migration (
    id SERIAL PRIMARY KEY,
    from_hub TEXT NOT NULL,
    to_hub TEXT NOT NULL,
    cluster_name TEXT NOT NULL,
    payload JSONB NOT NULL,
    stage TEXT NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    UNIQUE (from_hub, to_hub, cluster_name) -- Ensures uniqueness of the migration record
);
