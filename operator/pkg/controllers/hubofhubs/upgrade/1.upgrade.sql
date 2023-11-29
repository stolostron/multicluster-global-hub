ALTER TABLE status.leaf_hubs ADD cluster_id uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';

ALTER TABLE status.leaf_hubs
DROP CONSTRAINT leaf_hubs_pkey;

ALTER TABLE status.leaf_hubs
ADD CONSTRAINT leaf_hubs_pkey PRIMARY KEY (cluster_id, leaf_hub_name);
