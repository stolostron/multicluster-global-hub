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

1. To simulate the scalability for 1 year, change the scheduler interval of moving to compliance_history to 1 minute, then it needs about `12 * 30 / 60 = 6 hours` to generate `8 million` policy compliance history data, create the following MGH instance:

```yaml
apiVersion: operator.open-cluster-management.io/v1alpha4
kind: MulticlusterGlobalHub
metadata:
  annotations:
    mgh-scheduler-interval: minute # change scheduler interval of moving to compliance_history to 1 minute
  name: multiclusterglobalhub
  namespace: multicluster-global-hub
```
Note: You may need to restart the `multicluster-global-hub-operator` pod after the `multiclusterglobalhub` instance updated
```bash
oc delete pod multicluster-global-hub-operator-xxx -n multicluster-global-hub
```

2. Insert the simulated data into database:

If you have crunchy operator installed, use the following commands to insert:
```bash
kubectl cp create_local_policies.sql multicluster-global-hub/$(kubectl get pods -n multicluster-global-hub -l postgres-operator.crunchydata.com/role=master -o jsonpath='{.items..metadata.name}'):/tmp
kubectl exec -t $(kubectl get pods -n multicluster-global-hub -l postgres-operator.crunchydata.com/role=master -o jsonpath='{.items..metadata.name}') -c database -n multicluster-global-hub -- ls -l /tmp/create_local_policies.sql
kubectl exec -t $(kubectl get pods -n multicluster-global-hub -l postgres-operator.crunchydata.com/role=master -o jsonpath='{.items..metadata.name}') -c database -n multicluster-global-hub -- psql -U postgres -d hoh -f /tmp/create_local_policies.sql
```
If you have postgres statefulset installed, use the following commands to insert:
```bash
kubectl cp create_local_policies.sql multicluster-global-hub/multicluster-global-hub-postgres-0:/tmp
kubectl exec -t multicluster-global-hub-postgres-0 -n multicluster-global-hub -- ls -l /tmp/create_local_policies.sql
kubectl exec -t multicluster-global-hub-postgres-0 -n multicluster-global-hub -- psql -U postgres -d hoh -f /tmp/create_local_policies.sql
```

3. Data Inserted into database

- `status.managed_clusters` will have around 699 rows
- `local_spec.policies` will have around 90 rows
- `local_status.compliance` will have around 699 * 30 = 20970 rows

4. Check the manager logs to make sure the scheduler job running successfully:

```bash
# oc -n multicluster-global-hub logs -f deploy/multicluster-global-hub-manager multicluster-global-hub-manager
{"level":"info","ts":1684217414.735211,"logger":"local-compliance-job","msg":"start running","LastRun":"2023-05-16 06:10:14","NextRun":"2023-05-16 06:11:14"}
batchSize: 1000, insert: 1000, offset: 0
batchSize: 1000, insert: 1000, offset: 1000
batchSize: 1000, insert: 1000, offset: 2000
batchSize: 1000, insert: 1000, offset: 3000
batchSize: 1000, insert: 1000, offset: 4000
batchSize: 1000, insert: 1000, offset: 5000
batchSize: 1000, insert: 1000, offset: 6000
batchSize: 1000, insert: 1000, offset: 7000
batchSize: 1000, insert: 1000, offset: 8000
batchSize: 1000, insert: 1000, offset: 9000
batchSize: 1000, insert: 1000, offset: 10000
...
```

5. After about 6 hours, check the row count of `history.local_compliance` table:

```bash
# oc exec -it $(oc get pods -n multicluster-global-hub -l postgres-operator.crunchydata.com/role=master -o jsonpath='{.items..metadata.name}') -c database -n multicluster-global-hub -- psql -U postgres -d hoh -c "SELECT count(*) from history.local_compliance"
 count
--------
 7942000
(1 row)
```
OR
```bash
# oc exec -it multicluster-global-hub-postgres-0 -n multicluster-global-hub -- psql -U postgres -d hoh -c "SELECT count(*) from history.local_compliance"
 count
--------
 7942000
(1 row)
```

6. Check the PV usage of the postgres:

```bash
# oc -n multicluster-global-hub exec -it hoh-pgha1-275z-0 -- df -h
or
oc -n multicluster-global-hub exec -it multicluster-global-hub-postgres-0 -- df -h
```

`/pgdata` uses about 3Gi space for `8 million` data.

7. Check the Grafana dashbord for "Global Hub - Policy Group Compliancy Overview".
