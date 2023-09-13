--- the CREATE TRIGGER only for postgre 14
--- CREATE TRIGGER set_timestamp BEFORE UPDATE ON history.applications FOR EACH ROW EXECUTE FUNCTION public.trigger_set_timestamp();

DROP TRIGGER IF EXISTS set_timestamp ON local_spec.placementrules;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON local_spec.placementrules FOR EACH ROW EXECUTE FUNCTION public.trigger_set_timestamp();

DROP TRIGGER IF EXISTS set_timestamp ON history.applications;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON history.applications FOR EACH ROW EXECUTE FUNCTION public.trigger_set_timestamp();
DROP TRIGGER IF EXISTS set_timestamp ON history.channels;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON history.channels FOR EACH ROW EXECUTE FUNCTION public.trigger_set_timestamp();
DROP TRIGGER IF EXISTS set_timestamp ON history.managedclustersetbindings;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON history.managedclustersetbindings FOR EACH ROW EXECUTE FUNCTION public.trigger_set_timestamp();
DROP TRIGGER IF EXISTS set_timestamp ON history.managedclustersets;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON history.managedclustersets FOR EACH ROW EXECUTE FUNCTION public.trigger_set_timestamp();
DROP TRIGGER IF EXISTS set_timestamp ON history.placementbindings;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON history.placementbindings FOR EACH ROW EXECUTE FUNCTION public.trigger_set_timestamp();
DROP TRIGGER IF EXISTS set_timestamp ON history.placementrules;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON history.placementrules FOR EACH ROW EXECUTE FUNCTION public.trigger_set_timestamp();
DROP TRIGGER IF EXISTS set_timestamp ON history.placements;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON history.placements FOR EACH ROW EXECUTE FUNCTION public.trigger_set_timestamp();
DROP TRIGGER IF EXISTS set_timestamp ON history.policies;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON history.policies FOR EACH ROW EXECUTE FUNCTION public.trigger_set_timestamp();
DROP TRIGGER IF EXISTS set_timestamp ON history.subscriptions;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON history.subscriptions FOR EACH ROW EXECUTE FUNCTION public.trigger_set_timestamp();
DROP TRIGGER IF EXISTS set_timestamp ON local_spec.placementrules;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON local_spec.placementrules FOR EACH ROW EXECUTE FUNCTION public.trigger_set_timestamp();

DROP TRIGGER IF EXISTS move_to_history ON spec.applications;
CREATE TRIGGER move_to_history BEFORE INSERT ON spec.applications FOR EACH ROW EXECUTE FUNCTION public.move_applications_to_history();
DROP TRIGGER IF EXISTS move_to_history ON spec.channels;
CREATE TRIGGER move_to_history BEFORE INSERT ON spec.channels FOR EACH ROW EXECUTE FUNCTION public.move_channels_to_history();
DROP TRIGGER IF EXISTS move_to_history ON spec.managedclustersetbindings;
CREATE TRIGGER move_to_history BEFORE INSERT ON spec.managedclustersetbindings FOR EACH ROW EXECUTE FUNCTION public.move_managedclustersetbindings_to_history();
DROP TRIGGER IF EXISTS move_to_history ON spec.managedclustersets;
CREATE TRIGGER move_to_history BEFORE INSERT ON spec.managedclustersets FOR EACH ROW EXECUTE FUNCTION public.move_managedclustersets_to_history();
DROP TRIGGER IF EXISTS move_to_history ON spec.placementbindings;
CREATE TRIGGER move_to_history BEFORE INSERT ON spec.placementbindings FOR EACH ROW EXECUTE FUNCTION public.move_placementbindings_to_history();
DROP TRIGGER IF EXISTS move_to_history ON spec.placementrules;
CREATE TRIGGER move_to_history BEFORE INSERT ON spec.placementrules FOR EACH ROW EXECUTE FUNCTION public.move_placementrules_to_history();
DROP TRIGGER IF EXISTS move_to_history ON spec.placements;
CREATE TRIGGER move_to_history BEFORE INSERT ON spec.placements FOR EACH ROW EXECUTE FUNCTION public.move_placements_to_history();
DROP TRIGGER IF EXISTS move_to_history ON spec.policies;
CREATE TRIGGER move_to_history BEFORE INSERT ON spec.policies FOR EACH ROW EXECUTE FUNCTION public.move_policies_to_history();
DROP TRIGGER IF EXISTS move_to_history ON spec.subscriptions;
CREATE TRIGGER move_to_history BEFORE INSERT ON spec.subscriptions FOR EACH ROW EXECUTE FUNCTION public.move_subscriptions_to_history();

DROP TRIGGER IF EXISTS set_timestamp ON spec.applications;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON spec.applications FOR EACH ROW EXECUTE FUNCTION public.trigger_set_timestamp();
DROP TRIGGER IF EXISTS set_timestamp ON spec.channels;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON spec.channels FOR EACH ROW EXECUTE FUNCTION public.trigger_set_timestamp();
DROP TRIGGER IF EXISTS set_timestamp ON spec.managedclustersetbindings;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON spec.managedclustersetbindings FOR EACH ROW EXECUTE FUNCTION public.trigger_set_timestamp();
DROP TRIGGER IF EXISTS set_timestamp ON spec.managedclustersets;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON spec.managedclustersets FOR EACH ROW EXECUTE FUNCTION public.trigger_set_timestamp();
DROP TRIGGER IF EXISTS set_timestamp ON spec.placementbindings;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON spec.placementbindings FOR EACH ROW EXECUTE FUNCTION public.trigger_set_timestamp();
DROP TRIGGER IF EXISTS set_timestamp ON spec.placementrules;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON spec.placementrules FOR EACH ROW EXECUTE FUNCTION public.trigger_set_timestamp();
DROP TRIGGER IF EXISTS set_timestamp ON spec.placements;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON spec.placements FOR EACH ROW EXECUTE FUNCTION public.trigger_set_timestamp();
DROP TRIGGER IF EXISTS set_timestamp ON spec.policies;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON spec.policies FOR EACH ROW EXECUTE FUNCTION public.trigger_set_timestamp();
DROP TRIGGER IF EXISTS set_timestamp ON spec.subscriptions;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON spec.subscriptions FOR EACH ROW EXECUTE FUNCTION public.trigger_set_timestamp();

DROP TRIGGER IF EXISTS update_compliance_table ON status.compliance;
CREATE TRIGGER update_compliance_table AFTER INSERT OR UPDATE ON status.compliance FOR EACH ROW WHEN (pg_trigger_depth() < 1) EXECUTE FUNCTION public.set_cluster_id_to_compliance();

DROP TRIGGER IF EXISTS update_compliance_cluster_id_trigger ON status.managed_clusters;
CREATE TRIGGER update_compliance_cluster_id_trigger
AFTER INSERT ON status.managed_clusters
FOR EACH ROW
EXECUTE FUNCTION public.update_compliance_cluster_id();
