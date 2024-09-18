--- Only handle Upgrade from 1.0.x to 1.1.x, Should remove this file after 1.1
ALTER TABLE status.leaf_hubs ADD IF NOT EXISTS cluster_id uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';

ALTER TABLE status.leaf_hubs DROP CONSTRAINT leaf_hubs_pkey;
ALTER TABLE status.leaf_hubs ADD CONSTRAINT leaf_hubs_pkey PRIMARY KEY (cluster_id, leaf_hub_name);

ALTER TABLE status.leaf_hub_heartbeats ADD COLUMN IF NOT EXISTS status VARCHAR(10) DEFAULT 'active';
CREATE INDEX IF NOT EXISTS leaf_hub_heartbeats_leaf_hub_status_idx ON status.leaf_hub_heartbeats(status);

ALTER TABLE history.local_compliance DROP CONSTRAINT IF EXISTS local_policies_unique_constraint;
ALTER TABLE history.local_compliance ADD CONSTRAINT local_policies_unique_constraint UNIQUE (leaf_hub_name, policy_id, cluster_id, compliance_date);

---- Handle Upgrade from 1.1 to 1.2
ALTER TYPE status.compliance_type ADD VALUE IF NOT EXISTS 'pending';
ALTER TYPE local_status.compliance_type ADD VALUE IF NOT EXISTS 'pending';

ALTER TABLE event.local_policies ADD COLUMN IF NOT EXISTS event_namespace text;
ALTER TABLE event.local_policies ADD COLUMN IF NOT EXISTS cluster_name text;
ALTER TABLE event.local_root_policies ADD COLUMN IF NOT EXISTS event_namespace text;
