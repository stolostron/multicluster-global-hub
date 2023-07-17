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


CREATE TABLE IF NOT EXISTS status.managed_clusters (
    leaf_hub_name character varying(63) NOT NULL,
    cluster_name character varying(63) generated always as (payload -> 'metadata' ->> 'name') stored,
    cluster_id uuid NOT NULL,
    payload jsonb NOT NULL,
    error status.error_type NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted_at timestamp without time zone,
    CONSTRAINT managed_cluster_unique_constraint UNIQUE (leaf_hub_name, cluster_id, deleted_at)
) PARTITION BY RANGE (deleted_at);

-- create the default partition for deleted_at is null
CREATE TABLE IF NOT EXISTS status.managed_clusters_default PARTITION OF status.managed_clusters (check (deleted_at is null)) DEFAULT;

CREATE TABLE IF NOT EXISTS status.leaf_hubs (
    leaf_hub_name character varying(63) NOT NULL PRIMARY KEY,
    payload jsonb NOT NULL,
    console_url text generated always as (payload ->> 'consoleURL') stored,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted_at timestamp without time zone
) PARTITION BY RANGE (deleted_at);

-- create the default partition for deleted_at is null
CREATE TABLE IF NOT EXISTS status.leaf_hubs_default PARTITION OF status.leaf_hubs (check (deleted_at is null)) DEFAULT;

CREATE TABLE IF NOT EXISTS local_spec.policies (
    leaf_hub_name character varying(63) NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted_at timestamp without time zone,
    policy_id uuid generated always as (uuid(payload->'metadata'->>'uid')) stored,
    policy_name character varying(255) generated always as (payload -> 'metadata' ->> 'name') stored,
    policy_standard character varying(255) generated always as (payload -> 'metadata' -> 'annotations' ->> 'policy.open-cluster-management.io/standards') stored,
    policy_category character varying(255) generated always as (payload -> 'metadata' -> 'annotations' ->> 'policy.open-cluster-management.io/categories') stored,
    policy_control character varying(255) generated always as (payload -> 'metadata' -> 'annotations' ->> 'policy.open-cluster-management.io/controls') stored,
    CONSTRAINT local_spec_policies_unique_constraint UNIQUE (leaf_hub_name, policy_id, deleted_at)
) PARTITION BY RANGE (deleted_at);

-- create the default partition for deleted_at is null
CREATE TABLE IF NOT EXISTS local_spec.policies_default PARTITION OF local_spec.policies (check (deleted_at is null)) DEFAULT;