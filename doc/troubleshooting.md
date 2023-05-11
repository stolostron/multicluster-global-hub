# Running the must-gather command for troubleshooting

Run `must-gather` to gather details, logs, and take steps in debugging issues, these debugging information is also useful when opening a support case. The `oc adm must-gather CLI` command collects the information from your cluster that is most likely needed for debugging issues, including:

1. Resource definitions
2. Service logs

### Prerequisites

1. Access to the global hub and regional hub clusters as a user with the cluster-admin role.
2. The OpenShift Container Platform CLI (oc) installed.

### Must-gather procedure

See the following procedure to start using the must-gather command:

1. Learn about the must-gather command and install the prerequisites that you need at [Gathering data about your cluster](https://docs.openshift.com/container-platform/4.8/support/gathering-cluster-data.html?extIdCarryOver=true&sc_cid=701f2000001Css5AAC) in the RedHat OpenShift Container Platform documentation.

2. Log in to your global hub cluster. For the usual use-case, you should run the must-gather while you are logged into your global hub cluster.

```bash
oc adm must-gather --image=quay.io/stolostron/must-gather:SNAPSHOTNAME
```

Note: If you want to check your regional hub clusters, run the `must-gather` command on those clusters.

Note: If you need the results to be saved in a named directory, then following the must-gather instructions, this can be run. Also added are commands to create a gzipped tarball:

```bash
oc adm must-gather --image=quay.io/stolostron/must-gather:SNAPSHOTNAME --dest-dir=SOMENAME ; tar -cvzf SOMENAME.tgz SOMENAME
```

### Information Captured

1. Two peer levels: `cluster-scoped-resources` and `namespaces` resources.
2. Sub-level for each: API group for the custom resource definitions for both cluster-scope and namespace-scoped resources.
3. Next level for each: YAML file sorted by kind.
4. For the global hub, you can check the `PostgresCluster` and `Kafka` in the `namespaces` resources.

![must-gather-global-hub-postgres](must-gather/must-gather-global-hub-postgres.png)

![must-gather-global-hub-kafka](must-gather/must-gather-global-hub-kafka.png)

5. For the global hub, you can check the multicluster global hub related pods and logs in `pods` of `namespaces` resources.

![must-gather-global-hub-pods](must-gather/must-gather-global-hub-pods.png)

6. For the regional hub cluster, you can check the multicluster global hub agent pods and logs in `pods` of `namespaces` resources.

![must-gather-regional-hub-pods](must-gather/must-gather-regional-hub-pods.png)
