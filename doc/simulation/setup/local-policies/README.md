# Create Simulated Local Policies for Scalability Testing

For scalability testing of local policies, we have the following scenario will be created:

1. 699 clusters across 3 ACM hubs
2. Each cluster will have around 30 policies deployed

Which means:

1. `local_status.compliance` will have around 699 * 30 = 20970 rows
2. `local_spec.policies` will have around 90 rows
3. `status.managed_clusters` will have around 699 rows

And daily summary will add to `history.local_compliance` 20970 rows/day, which means around `4 million` for 6 months and `8 million` for 1 year.

## How To Use


1. Insert the simulated data into database:

If you have crunchy operator installed, use the following commands to insert:
```bash
kubectl cp create_local_policies.sql multicluster-global-hub/$(kubectl get pods -n multicluster-global-hub -l postgres-operator.crunchydata.com/role=master -o jsonpath='{.items..metadata.name}'):/tmp
kubectl exec -t $(kubectl get pods -n multicluster-global-hub -l postgres-operator.crunchydata.com/role=master -o jsonpath='{.items..metadata.name}') -c database -n multicluster-global-hub -- ls -l /tmp/create_local_policies.sql
kubectl exec -t $(kubectl get pods -n multicluster-global-hub -l postgres-operator.crunchydata.com/role=master -o jsonpath='{.items..metadata.name}') -c database -n multicluster-global-hub -- psql -U postgres -d hoh -f /tmp/create_local_policies.sql
```
If you have postgres statefulset installed, use the following commands to insert:
```bash
kubectl cp create_local_policies.sql multicluster-global-hub/multicluster-global-hub-postgresql-0:/tmp
kubectl exec -t multicluster-global-hub-postgresql-0 -n multicluster-global-hub -- ls -l /tmp/create_local_policies.sql
kubectl exec -t multicluster-global-hub-postgresql-0 -n multicluster-global-hub -- psql -U postgres -d hoh -f /tmp/create_local_policies.sql
```

2. Data Inserted into database

- `status.managed_clusters` will have around 699 rows
- `local_spec.policies` will have around 90 rows
- `local_status.compliance` will have around 699 * 30 = 20970 rows

3. To simulate the scalability for 1 year, use the following script to generate 1 year data in `history.local_compliance`:

```bash
kubectl cp history_local_compliance.sql multicluster-global-hub/multicluster-global-hub-postgresql-0:/tmp
kubectl exec -t multicluster-global-hub-postgresql-0 -n multicluster-global-hub -- ls -l /tmp/history_local_compliance.sql
kubectl exec -t multicluster-global-hub-postgresql-0 -n multicluster-global-hub -- psql -U postgres -d hoh -f /tmp/history_local_compliance.sql
```

4. Check the row count of `history.local_compliance` table:

```bash
# oc exec -it $(oc get pods -n multicluster-global-hub -l postgres-operator.crunchydata.com/role=master -o jsonpath='{.items..metadata.name}') -c database -n multicluster-global-hub -- psql -U postgres -d hoh -c "SELECT count(*) from history.local_compliance"
 count
--------
 7942000
(1 row)
```
OR
```bash
# oc exec -it multicluster-global-hub-postgresql-0 -n multicluster-global-hub -- psql -U postgres -d hoh -c "SELECT count(*) from history.local_compliance"
 count
--------
 7942000
(1 row)
```

5. Check the PV usage of the postgres:

```bash
# oc -n multicluster-global-hub exec -it hoh-pgha1-275z-0 -- df -h
or
oc -n multicluster-global-hub exec -it multicluster-global-hub-postgresql-0 -- df -h
```

`/pgdata` uses about 3Gi space for `8 million` data.

6. Check the Grafana dashbord for "Global Hub - Policy Group Compliancy Overview".
