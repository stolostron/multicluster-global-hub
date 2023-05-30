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
apiVersion: operator.open-cluster-management.io/v1alpha2
kind: MulticlusterGlobalHub
metadata:
  annotations:
    mgh-scheduler-interval: minute # change scheduler interval of moving to compliance_history to 1 minute
  name: multiclusterglobalhub
  namespace: open-cluster-management
spec:
  dataLayer:
    largeScale:
      kafka:
        name: transport-secret
        transportFormat: cloudEvents
      postgres:
        name: storage-secret
    type: largeScale
```

2. Insert the simulated data into database:

```bash
kubectl cp create_local_policies.sql hoh-postgres/$(kubectl get pods -n hoh-postgres -l postgres-operator.crunchydata.com/role=master -o jsonpath='{.items..metadata.name}'):/tmp
kubectl exec -it $(kubectl get pods -n hoh-postgres -l postgres-operator.crunchydata.com/role=master -o jsonpath='{.items..metadata.name}') -c database -n hoh-postgres -- ls -l /tmp/create_local_policies.sql
kubectl exec -it $(kubectl get pods -n hoh-postgres -l postgres-operator.crunchydata.com/role=master -o jsonpath='{.items..metadata.name}') -c database -n hoh-postgres -- psql -U postgres -d hoh -f /tmp/create_local_policies.sql
```

3. Data Inserted into database

- `status.managed_clusters` will have around 699 rows
- `local_spec.policies` will have around 90 rows
- `local_status.compliance` will have around 699 * 30 = 20970 rows

4. Check the manager logs to make sure the scheduler job running successfully:

```bash
# oc -n open-cluster-management logs -f deploy/multicluster-global-hub-manager
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
# oc exec -it $(oc get pods -n hoh-postgres -l postgres-operator.crunchydata.com/role=master -o jsonpath='{.items..metadata.name}') -c database -n hoh-postgres -- psql -U postgres -d hoh -c "SELECT count(*) from history.local_compliance"
 count
--------
 7942000
(1 row)
```

6. Check the PV usage of the postgres:

```bash
# oc -n hoh-postgres exec -it hoh-pgha1-275z-0 -- df -h
Defaulted container "database" out of: database, replication-cert-copy, pgbackrest, pgbackrest-config, postgres-startup (init), nss-wrapper-init (init)
Filesystem      Size  Used Avail Use% Mounted on
overlay         120G   30G   90G  26% /
tmpfs            64M     0   64M   0% /dev
tmpfs            31G     0   31G   0% /sys/fs/cgroup
tmpfs            31G   84M   31G   1% /etc/passwd
/dev/nvme0n1p4  120G   30G   90G  26% /tmp
/dev/nvme4n1     20G  1.3G   20G   2% /pgdata
tmpfs            61G   24K   61G   1% /pgconf/tls
tmpfs            61G   24K   61G   1% /etc/database-containerinfo
tmpfs            61G   16K   61G   1% /etc/patroni
tmpfs            61G   28K   61G   1% /dev/shm
tmpfs            61G   24K   61G   1% /etc/pgbackrest/conf.d
tmpfs            61G   20K   61G   1% /run/secrets/kubernetes.io/serviceaccount
tmpfs            31G     0   31G   0% /proc/acpi
tmpfs            31G     0   31G   0% /proc/scsi
tmpfs            31G     0   31G   0% /sys/firmware
```

As we can see from above output, `/pgdata` use about 3G space for `8 million` data.

7. Check the Grafana dashbord for "Global Hub - Policy Group Compliancy Overview".
