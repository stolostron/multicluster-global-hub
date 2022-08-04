CREATE TABLE IF NOT EXISTS  history.applications (
    id uuid NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS  history.channels (
    id uuid NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS  history.configs (
    id uuid NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS  history.managedclustersetbindings (
    id uuid NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS  history.managedclustersets (
    id uuid NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS  history.placementbindings (
    id uuid NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS  history.placementrules (
    id uuid NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS  history.placements (
    id uuid NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS  history.policies (
    id uuid NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS  history.subscriptions (
    id uuid NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS  local_spec.placementrules (
    leaf_hub_name text,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL
);

CREATE TABLE IF NOT EXISTS  local_spec.policies (
    leaf_hub_name text,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL
);

CREATE TABLE IF NOT EXISTS  local_status.compliance (
    id uuid NOT NULL,
    cluster_name character varying(63) NOT NULL,
    leaf_hub_name character varying(63) NOT NULL,
    error status.error_type NOT NULL,
    compliance local_status.compliance_type NOT NULL
);

CREATE TABLE IF NOT EXISTS  spec.applications (
    id uuid NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS  spec.channels (
    id uuid NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS  spec.configs (
    id uuid NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS  spec.managed_cluster_sets_tracking (
    cluster_set_name character varying(63) NOT NULL,
    leaf_hub_name character varying(63) NOT NULL,
    managed_clusters jsonb DEFAULT '[]'::jsonb NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL
);

CREATE TABLE IF NOT EXISTS  spec.managed_clusters_labels (
    leaf_hub_name character varying(63) DEFAULT ''::character varying NOT NULL,
    managed_cluster_name character varying(63) NOT NULL,
    labels jsonb DEFAULT '{}'::jsonb NOT NULL,
    deleted_label_keys jsonb DEFAULT '[]'::jsonb NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    version bigint DEFAULT 0 NOT NULL,
    CONSTRAINT managed_clusters_labels_version_check CHECK ((version >= 0))
);

CREATE TABLE IF NOT EXISTS  spec.managedclustersetbindings (
    id uuid NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS  spec.managedclustersets (
    id uuid NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS  spec.placementbindings (
    id uuid NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS  spec.placementrules (
    id uuid NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS  spec.placements (
    id uuid NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS  spec.policies (
    id uuid NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS  spec.subscriptions (
    id uuid NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS  status.aggregated_compliance (
    id uuid NOT NULL,
    leaf_hub_name character varying(63) NOT NULL,
    applied_clusters integer NOT NULL,
    non_compliant_clusters integer NOT NULL
);

CREATE TABLE IF NOT EXISTS  status.compliance (
    id uuid NOT NULL,
    cluster_name character varying(63) NOT NULL,
    leaf_hub_name character varying(63) NOT NULL,
    error status.error_type NOT NULL,
    compliance status.compliance_type NOT NULL
);

CREATE TABLE IF NOT EXISTS  status.leaf_hub_heartbeats (
    leaf_hub_name character varying(63) NOT NULL,
    last_timestamp timestamp without time zone DEFAULT now() NOT NULL
);

CREATE TABLE IF NOT EXISTS  status.managed_clusters (
    leaf_hub_name character varying(63) NOT NULL,
    payload jsonb NOT NULL,
    error status.error_type NOT NULL
);

CREATE TABLE IF NOT EXISTS  status.placementdecisions (
    id uuid NOT NULL,
    leaf_hub_name character varying(63) NOT NULL,
    payload jsonb NOT NULL
);

CREATE TABLE IF NOT EXISTS  status.placementrules (
    id uuid NOT NULL,
    leaf_hub_name character varying(63) NOT NULL,
    payload jsonb NOT NULL
);

CREATE TABLE IF NOT EXISTS  status.placements (
    id uuid NOT NULL,
    leaf_hub_name character varying(63) NOT NULL,
    payload jsonb NOT NULL
);

CREATE TABLE IF NOT EXISTS  status.subscription_reports (
    id uuid NOT NULL,
    leaf_hub_name character varying(63) NOT NULL,
    payload jsonb NOT NULL
);

CREATE TABLE IF NOT EXISTS  status.subscription_statuses (
    id uuid NOT NULL,
    leaf_hub_name character varying(63) NOT NULL,
    payload jsonb NOT NULL
);

ALTER TABLE history.applications DROP CONSTRAINT IF EXISTS applications_pkey;
ALTER TABLE ONLY history.applications
    ADD CONSTRAINT applications_pkey PRIMARY KEY (id);

ALTER TABLE history.channels DROP CONSTRAINT IF EXISTS channels_pkey;
ALTER TABLE ONLY history.channels
    ADD CONSTRAINT channels_pkey PRIMARY KEY (id);

ALTER TABLE history.configs DROP CONSTRAINT IF EXISTS configs_pkey;
ALTER TABLE ONLY history.configs
    ADD CONSTRAINT configs_pkey PRIMARY KEY (id);

ALTER TABLE history.managedclustersetbindings DROP CONSTRAINT IF EXISTS managedclustersetbindings_pkey;
ALTER TABLE ONLY history.managedclustersetbindings
    ADD CONSTRAINT managedclustersetbindings_pkey PRIMARY KEY (id);

ALTER TABLE history.managedclustersets DROP CONSTRAINT IF EXISTS managedclustersets_pkey;
ALTER TABLE ONLY history.managedclustersets
    ADD CONSTRAINT managedclustersets_pkey PRIMARY KEY (id);

ALTER TABLE history.placementbindings DROP CONSTRAINT IF EXISTS placementbindings_pkey;
ALTER TABLE ONLY history.placementbindings
    ADD CONSTRAINT placementbindings_pkey PRIMARY KEY (id);

ALTER TABLE history.placementrules DROP CONSTRAINT IF EXISTS placementrules_pkey;
ALTER TABLE ONLY history.placementrules
    ADD CONSTRAINT placementrules_pkey PRIMARY KEY (id);

ALTER TABLE history.placements DROP CONSTRAINT IF EXISTS placements_pkey;
ALTER TABLE ONLY history.placements
    ADD CONSTRAINT placements_pkey PRIMARY KEY (id);


ALTER TABLE history.policies DROP CONSTRAINT IF EXISTS policies_pkey;
ALTER TABLE ONLY history.policies
    ADD CONSTRAINT policies_pkey PRIMARY KEY (id);

ALTER TABLE history.subscriptions DROP CONSTRAINT IF EXISTS subscriptions_pkey;
ALTER TABLE ONLY history.subscriptions
    ADD CONSTRAINT subscriptions_pkey PRIMARY KEY (id);

ALTER TABLE spec.applications DROP CONSTRAINT IF EXISTS applications_pkey;
ALTER TABLE ONLY spec.applications
    ADD CONSTRAINT applications_pkey PRIMARY KEY (id);


ALTER TABLE spec.channels DROP CONSTRAINT IF EXISTS channels_pkey;
ALTER TABLE ONLY spec.channels
    ADD CONSTRAINT channels_pkey PRIMARY KEY (id);

ALTER TABLE spec.configs DROP CONSTRAINT IF EXISTS configs_pkey;
ALTER TABLE ONLY spec.configs
    ADD CONSTRAINT configs_pkey PRIMARY KEY (id);


ALTER TABLE spec.managedclustersetbindings DROP CONSTRAINT IF EXISTS managedclustersetbindings_pkey;
ALTER TABLE ONLY spec.managedclustersetbindings
    ADD CONSTRAINT managedclustersetbindings_pkey PRIMARY KEY (id);


ALTER TABLE spec.managedclustersets DROP CONSTRAINT IF EXISTS managedclustersets_pkey;
ALTER TABLE ONLY spec.managedclustersets
    ADD CONSTRAINT managedclustersets_pkey PRIMARY KEY (id);

ALTER TABLE spec.placementbindings DROP CONSTRAINT IF EXISTS placementbindings_pkey;
ALTER TABLE ONLY spec.placementbindings
    ADD CONSTRAINT placementbindings_pkey PRIMARY KEY (id);

ALTER TABLE spec.placementrules DROP CONSTRAINT IF EXISTS placementrules_pkey;
ALTER TABLE ONLY spec.placementrules
    ADD CONSTRAINT placementrules_pkey PRIMARY KEY (id);

ALTER TABLE spec.placements DROP CONSTRAINT IF EXISTS placements_pkey;
ALTER TABLE ONLY spec.placements
    ADD CONSTRAINT placements_pkey PRIMARY KEY (id);

ALTER TABLE spec.policies DROP CONSTRAINT IF EXISTS policies_pkey;
ALTER TABLE ONLY spec.policies
    ADD CONSTRAINT policies_pkey PRIMARY KEY (id);

ALTER TABLE spec.subscriptions DROP CONSTRAINT IF EXISTS subscriptions_pkey;
ALTER TABLE ONLY spec.subscriptions
    ADD CONSTRAINT subscriptions_pkey PRIMARY KEY (id);


CREATE UNIQUE INDEX IF NOT EXISTS placementrules_leaf_hub_name_id_idx ON local_spec.placementrules USING btree (leaf_hub_name, (((payload -> 'metadata'::text) ->> 'uid'::text)));

CREATE UNIQUE INDEX IF NOT EXISTS policies_leaf_hub_name_id_idx ON local_spec.policies USING btree (leaf_hub_name, (((payload -> 'metadata'::text) ->> 'uid'::text)));

CREATE UNIQUE INDEX IF NOT EXISTS managed_cluster_sets_tracking_cluster_set_name_and_leaf_hub_nam ON spec.managed_cluster_sets_tracking USING btree (cluster_set_name, leaf_hub_name);

CREATE UNIQUE INDEX IF NOT EXISTS managed_clusters_labels_leaf_hub_name_and_cluster_name_idx ON spec.managed_clusters_labels USING btree (leaf_hub_name, managed_cluster_name);

CREATE INDEX IF NOT EXISTS compliance_leaf_hub_cluster_idx ON status.compliance USING btree (leaf_hub_name, cluster_name);

CREATE INDEX IF NOT EXISTS compliance_leaf_hub_non_compliant_idx ON status.compliance USING btree (leaf_hub_name, compliance) WHERE (compliance <> 'compliant'::status.compliance_type);

CREATE UNIQUE INDEX IF NOT EXISTS compliance_leaf_hub_policy_cluster_idx ON status.compliance USING btree (leaf_hub_name, id, cluster_name);

CREATE UNIQUE INDEX IF NOT EXISTS leaf_hub_heartbeats_leaf_hub_idx ON status.leaf_hub_heartbeats USING btree (leaf_hub_name);

CREATE UNIQUE INDEX IF NOT EXISTS managed_clusters_leaf_hub_name_metadata_name_idx ON status.managed_clusters USING btree (leaf_hub_name, (((payload -> 'metadata'::text) ->> 'name'::text)));

CREATE INDEX IF NOT EXISTS managed_clusters_metadata_name_idx ON status.managed_clusters USING btree ((((payload -> 'metadata'::text) ->> 'name'::text)));

CREATE UNIQUE INDEX IF NOT EXISTS placementdecisions_leaf_hub_name_and_payload_name_namespace_idx ON status.placementdecisions USING btree (leaf_hub_name, (((payload -> 'metadata'::text) ->> 'name'::text)), (((payload -> 'metadata'::text) ->> 'namespace'::text)));

CREATE INDEX IF NOT EXISTS placementdecisions_payload_name_and_namespace_idx ON status.placementdecisions USING btree ((((payload -> 'metadata'::text) ->> 'name'::text)), (((payload -> 'metadata'::text) ->> 'namespace'::text)));

CREATE UNIQUE INDEX IF NOT EXISTS placementrules_leaf_hub_name_and_payload_name_namespace_idx ON status.placementrules USING btree (leaf_hub_name, (((payload -> 'metadata'::text) ->> 'name'::text)), (((payload -> 'metadata'::text) ->> 'namespace'::text)));

CREATE INDEX IF NOT EXISTS placementrules_payload_name_and_namespace_idx ON status.placementrules USING btree ((((payload -> 'metadata'::text) ->> 'name'::text)), (((payload -> 'metadata'::text) ->> 'namespace'::text)));

CREATE UNIQUE INDEX IF NOT EXISTS placements_leaf_hub_name_and_payload_name_namespace_idx ON status.placements USING btree (leaf_hub_name, (((payload -> 'metadata'::text) ->> 'name'::text)), (((payload -> 'metadata'::text) ->> 'namespace'::text)));

CREATE INDEX IF NOT EXISTS placements_payload_name_and_namespace_idx ON status.placements USING btree ((((payload -> 'metadata'::text) ->> 'name'::text)), (((payload -> 'metadata'::text) ->> 'namespace'::text)));

CREATE UNIQUE INDEX IF NOT EXISTS subscription_reports_leaf_hub_name_and_payload_name_namespace_i ON status.subscription_reports USING btree (leaf_hub_name, (((payload -> 'metadata'::text) ->> 'name'::text)), (((payload -> 'metadata'::text) ->> 'namespace'::text)));

CREATE INDEX IF NOT EXISTS subscription_reports_payload_name_and_namespace_idx ON status.subscription_reports USING btree ((((payload -> 'metadata'::text) ->> 'name'::text)), (((payload -> 'metadata'::text) ->> 'namespace'::text)));

CREATE UNIQUE INDEX IF NOT EXISTS subscription_statuses_leaf_hub_name_and_payload_name_namespace_ ON status.subscription_statuses USING btree (leaf_hub_name, (((payload -> 'metadata'::text) ->> 'name'::text)), (((payload -> 'metadata'::text) ->> 'namespace'::text)));

CREATE INDEX IF NOT EXISTS subscription_statuses_payload_name_and_namespace_idx ON status.subscription_statuses USING btree ((((payload -> 'metadata'::text) ->> 'name'::text)), (((payload -> 'metadata'::text) ->> 'namespace'::text)));
