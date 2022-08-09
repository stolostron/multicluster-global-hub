DO
$do$
BEGIN
   IF EXISTS (
      SELECT FROM pg_catalog.pg_roles
      WHERE  rolname = 'process_user') THEN
      RAISE NOTICE 'Role "process_user" already exists. Skipping.';
   ELSE
      CREATE USER process_user;
   END IF;
END
$do$;

DO
$do$
BEGIN
   IF EXISTS (
      SELECT FROM pg_catalog.pg_roles
      WHERE  rolname = 'transport_user') THEN
      RAISE NOTICE 'Role "transport_user" already exists. Skipping.';
   ELSE
      CREATE USER transport_user;
   END IF;
END
$do$;

-- grant user to schema
GRANT USAGE ON SCHEMA history TO process_user;
GRANT USAGE ON SCHEMA history TO transport_user;

GRANT USAGE ON SCHEMA local_spec TO process_user;
GRANT USAGE ON SCHEMA local_spec TO transport_user;

GRANT USAGE ON SCHEMA local_status TO process_user;
GRANT USAGE ON SCHEMA local_status TO transport_user;

GRANT USAGE ON SCHEMA spec TO process_user;
GRANT USAGE ON SCHEMA spec TO transport_user;

GRANT USAGE ON SCHEMA status TO process_user;
GRANT USAGE ON SCHEMA status TO transport_user;

-- grant user to table
GRANT SELECT,INSERT ON TABLE history.applications TO process_user;
GRANT SELECT,INSERT ON TABLE history.channels TO process_user;
GRANT SELECT,INSERT ON TABLE history.configs TO process_user;
GRANT SELECT,INSERT ON TABLE history.managedclustersetbindings TO process_user;
GRANT SELECT,INSERT ON TABLE history.managedclustersets TO process_user;
GRANT SELECT,INSERT ON TABLE history.placementbindings TO process_user;
GRANT SELECT,INSERT ON TABLE history.placementrules TO process_user;
GRANT SELECT,INSERT ON TABLE history.placements TO process_user;
GRANT SELECT,INSERT ON TABLE history.policies TO process_user;
GRANT SELECT,INSERT ON TABLE history.subscriptions TO process_user;

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE local_spec.placementrules TO transport_user;
GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE local_spec.policies TO transport_user;
GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE local_status.compliance TO transport_user;

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE spec.applications TO process_user;
GRANT SELECT ON TABLE spec.applications TO transport_user;

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE spec.channels TO process_user;
GRANT SELECT ON TABLE spec.channels TO transport_user;

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE spec.configs TO process_user;
GRANT SELECT ON TABLE spec.configs TO transport_user;

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE spec.managed_cluster_sets_tracking TO process_user;
GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE spec.managed_cluster_sets_tracking TO transport_user;

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE spec.managed_clusters_labels TO process_user;
GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE spec.managed_clusters_labels TO transport_user;

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE spec.managedclustersetbindings TO process_user;
GRANT SELECT ON TABLE spec.managedclustersetbindings TO transport_user;

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE spec.managedclustersets TO process_user;
GRANT SELECT ON TABLE spec.managedclustersets TO transport_user;

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE spec.placementbindings TO process_user;
GRANT SELECT ON TABLE spec.placementbindings TO transport_user;

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE spec.placementrules TO process_user;
GRANT SELECT ON TABLE spec.placementrules TO transport_user;

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE spec.placements TO process_user;
GRANT SELECT ON TABLE spec.placements TO transport_user;

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE spec.policies TO process_user;
GRANT SELECT ON TABLE spec.policies TO transport_user;

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE spec.subscriptions TO process_user;
GRANT SELECT ON TABLE spec.subscriptions TO transport_user;


GRANT SELECT ON TABLE status.aggregated_compliance TO process_user;
GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE status.aggregated_compliance TO transport_user;

GRANT SELECT ON TABLE status.compliance TO process_user;
GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE status.compliance TO transport_user;

GRANT SELECT ON TABLE status.leaf_hub_heartbeats TO process_user;
GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE status.leaf_hub_heartbeats TO transport_user;

GRANT SELECT ON TABLE status.managed_clusters TO process_user;
GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE status.managed_clusters TO transport_user;

GRANT SELECT ON TABLE status.placementdecisions TO process_user;
GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE status.placementdecisions TO transport_user;

GRANT SELECT ON TABLE status.placementrules TO process_user;
GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE status.placementrules TO transport_user;

GRANT SELECT ON TABLE status.placements TO process_user;
GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE status.placements TO transport_user;

GRANT SELECT ON TABLE status.subscription_reports TO process_user;
GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE status.subscription_reports TO transport_user;

GRANT SELECT ON TABLE status.subscription_statuses TO process_user;
GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE status.subscription_statuses TO transport_user;
