CREATE TABLE IF NOT EXISTS history.applications (
    id uuid PRIMARY KEY,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS history.channels (
    id uuid PRIMARY KEY,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS history.managedclustersetbindings (
    id uuid PRIMARY KEY,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS history.managedclustersets (
    id uuid PRIMARY KEY,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS history.placementbindings (
    id uuid PRIMARY KEY,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS history.placementrules (
    id uuid PRIMARY KEY,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS history.placements (
    id uuid PRIMARY KEY,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS history.policies (
    id uuid PRIMARY KEY,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS history.subscriptions (
    id uuid PRIMARY KEY,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS local_spec.placementrules (
    id uuid PRIMARY KEY,
    leaf_hub_name character varying(254) NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL
);

CREATE TABLE IF NOT EXISTS spec.applications (
    id uuid PRIMARY KEY,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS spec.channels (
    id uuid PRIMARY KEY,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS spec.managed_cluster_sets_tracking (
    cluster_set_name character varying(254) NOT NULL,
    leaf_hub_name character varying(254) NOT NULL,
    managed_clusters jsonb DEFAULT '[]'::jsonb NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL
);

CREATE TABLE IF NOT EXISTS spec.managed_clusters_labels (
    id uuid NOT NULL,
    leaf_hub_name character varying(254) DEFAULT ''::character varying NOT NULL,
    managed_cluster_name character varying(254) NOT NULL,
    labels jsonb DEFAULT '{}'::jsonb NOT NULL,
    deleted_label_keys jsonb DEFAULT '[]'::jsonb NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    version bigint DEFAULT 0 NOT NULL,
    CONSTRAINT managed_clusters_labels_version_check CHECK ((version >= 0))
);

CREATE TABLE IF NOT EXISTS spec.managedclustersetbindings (
    id uuid PRIMARY KEY,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS spec.managedclustersets (
    id uuid PRIMARY KEY,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS spec.placementbindings (
    id uuid PRIMARY KEY,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS spec.placementrules (
    id uuid PRIMARY KEY,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS spec.placements (
    id uuid PRIMARY KEY,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS spec.policies (
    id uuid PRIMARY KEY,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS spec.subscriptions (
    id uuid PRIMARY KEY,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS status.aggregated_compliance (
    policy_id uuid NOT NULL,
    leaf_hub_name character varying(254) NOT NULL,
    applied_clusters integer NOT NULL,
    non_compliant_clusters integer NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS aggregated_compliance_unique_idx ON status.aggregated_compliance (policy_id, leaf_hub_name);

CREATE TABLE IF NOT EXISTS status.compliance (
    policy_id uuid NOT NULL,
    cluster_name character varying(254) NOT NULL,
    leaf_hub_name character varying(254) NOT NULL,
    error status.error_type NOT NULL,
    compliance status.compliance_type NOT NULL,
    cluster_id uuid
);

CREATE TABLE IF NOT EXISTS status.placementdecisions (
    id uuid NOT NULL,
    leaf_hub_name character varying(254) NOT NULL,
    payload jsonb NOT NULL
);

CREATE TABLE IF NOT EXISTS status.placementrules (
    id uuid NOT NULL,
    leaf_hub_name character varying(254) NOT NULL,
    payload jsonb NOT NULL
);

CREATE TABLE IF NOT EXISTS status.placements (
    id uuid NOT NULL,
    leaf_hub_name character varying(254) NOT NULL,
    payload jsonb NOT NULL
);

CREATE TABLE IF NOT EXISTS status.subscription_reports (
    id uuid NOT NULL,
    leaf_hub_name character varying(254) NOT NULL,
    payload jsonb NOT NULL
);

CREATE TABLE IF NOT EXISTS status.subscription_statuses (
    id uuid NOT NULL,
    leaf_hub_name character varying(254) NOT NULL,
    payload jsonb NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS managed_cluster_sets_tracking_cluster_set_name_and_leaf_hub_name_idx ON spec.managed_cluster_sets_tracking (cluster_set_name, leaf_hub_name);

CREATE INDEX IF NOT EXISTS compliance_leaf_hub_cluster_idx ON status.compliance (leaf_hub_name, cluster_name);

CREATE INDEX IF NOT EXISTS compliance_leaf_hub_non_compliant_idx ON status.compliance (leaf_hub_name, compliance) WHERE (compliance <> 'compliant'::status.compliance_type);

CREATE UNIQUE INDEX IF NOT EXISTS compliance_leaf_hub_policy_cluster_idx ON status.compliance (leaf_hub_name, policy_id, cluster_name);

CREATE UNIQUE INDEX IF NOT EXISTS placementdecisions_leaf_hub_name_and_payload_id_namespace_idx ON status.placementdecisions (leaf_hub_name, id, (((payload -> 'metadata'::text) ->> 'namespace'::text)));

CREATE INDEX IF NOT EXISTS placementdecisions_payload_name_and_namespace_idx ON status.placementdecisions ((((payload -> 'metadata'::text) ->> 'name'::text)), (((payload -> 'metadata'::text) ->> 'namespace'::text)));

CREATE UNIQUE INDEX IF NOT EXISTS placementrules_leaf_hub_name_and_payload_id_namespace_idx ON status.placementrules (leaf_hub_name, id, (((payload -> 'metadata'::text) ->> 'namespace'::text)));

CREATE INDEX IF NOT EXISTS placementrules_payload_name_and_namespace_idx ON status.placementrules ((((payload -> 'metadata'::text) ->> 'name'::text)), (((payload -> 'metadata'::text) ->> 'namespace'::text)));

CREATE UNIQUE INDEX IF NOT EXISTS placements_leaf_hub_name_and_payload_id_namespace_idx ON status.placements (leaf_hub_name, id, (((payload -> 'metadata'::text) ->> 'namespace'::text)));

CREATE INDEX IF NOT EXISTS placements_payload_name_and_namespace_idx ON status.placements ((((payload -> 'metadata'::text) ->> 'name'::text)), (((payload -> 'metadata'::text) ->> 'namespace'::text)));

CREATE UNIQUE INDEX IF NOT EXISTS subscription_reports_leaf_hub_name_and_payload_id_namespace_idx ON status.subscription_reports (leaf_hub_name, id, (((payload -> 'metadata'::text) ->> 'namespace'::text)));

CREATE INDEX IF NOT EXISTS subscription_reports_payload_name_and_namespace_idx ON status.subscription_reports ((((payload -> 'metadata'::text) ->> 'name'::text)), (((payload -> 'metadata'::text) ->> 'namespace'::text)));

CREATE UNIQUE INDEX IF NOT EXISTS subscription_statuses_leaf_hub_name_and_payload_id_namespace_idx ON status.subscription_statuses (leaf_hub_name, id, (((payload -> 'metadata'::text) ->> 'namespace'::text)));

CREATE INDEX IF NOT EXISTS subscription_statuses_payload_name_and_namespace_idx ON status.subscription_statuses ((((payload -> 'metadata'::text) ->> 'name'::text)), (((payload -> 'metadata'::text) ->> 'namespace'::text)));