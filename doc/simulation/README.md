# Simulation at High Scale

## Environment

This simulation requires a **Red Hat OpenShift Container Platform clusters**. And also you need a virtual machine to create the KinD cluster and managed hub. Then we will mock the resources on the KinD cluster and observe the status on the database. The overall architecture is shown in the figure below.

![Scale Test Environment](./../images/global-hub-scale-test-overview.png)

You can use the scale test environment to simulate a large-scale cluster. Since the transport status path is very sensitive to the performance and represents a scale larger than the spec path.

There are two main metrics to express the scalability of global hubs:

- Initialization: The time to load all managed clusters and Policies to the Global Hub
- Policy status rotation: Rotate all the policies on the managed hubs and verify status changes on the database and observe the CPU and Memory consumption of the components.

## Analysis

You can setup `5` hubs, each with `300` clusters, `15000` replicas policies and at least `15000` policy events, by following the [document](./setup/README.md). Then run the global hub [inspector](./inspector/README.md) to `analysis` the CPU and `Memory` consumptoin of the components.

The steps we simulate here are as follows:

1. Install the global hub and then join the `5` simulated managed hubs into it.

2. Deploy the `multicluster-global-hub-agent` to the `hub1` cluster and then rotating all the polcies status from `Complianct` to `NonCompliant`, or vice versa.

3. After the step 2, apply the `agent` to `hub2` and `hub3`, then roating all the status from `NonCompliant` to `Compliant`.

4. At last, install the `agent` to `hub4` and `hub5`

Through the above steps, we can see the metrics changing trends of the global hub components under the management of `1`, `3` and `5` hubs


- 





  ![Record Counter](./../images/global-hub-record-counter.png)

  The results above illustrate the growth of records in the databasae over time, indicating that these 5 managed hubs took approximately 5 minutes from initially receiving data to synchronizing all compliance status and clusters with the database.

- Check the CPU consumption

  ![Max Initialization CPU](./../images/global-hub-cpu-initialization-max.png)
  ![Min Initialization CPU](./../images/global-hub-cpu-initialization-min.png)

  It can be seen from the above graph that the CPU changes with the amount of data. The `multicluster-global-hub-grafana` CPU exhibits minor fluctuations, basically keep below `10m`. However, the `multicluster-global-hub-manager` CPU shows significant ups and downs in reponse to the data changes. It reaches its peak at `171 m`, and as the processing data decreases, it will be as low as `5 m` or less.

- Check the Memory consumption

  ![Min Initialization Memory](./../images/global-hub-memory-initialization-min.png)
  ![Max Initialization Memory](./../images/global-hub-memory-initialization-max.png)

  Unlike the CPU consumption, the memory consumption remains relatively consistent without frequent fluctuations. After all the data was inserted into the database, the consumption of  `multicluster-global-hub-grafana` memory reaches its peak at `186 MiB` and subsequently remained around `143 MiB`. Without processing any data, the `multicluster-global-hub-manager` has a memory usage of approximately `52 MiB`, Afther caching all the necessary data from the managed hub, its memory usage increased to around `190 MiB`.

## Policy Status Rotation

- The CPU and Memory consumption of the `multicluster-global-hub`
 ![Policy rotation CPU](./../images/global-hub-cpu-policy-rotation.png)
 ![Policy rotation Memory](./../images/global-hub-memory-policy-rotation.png)

  We can see that the  `multicluster-global-hub-manager` need more CPU resource when updating all the policy status. The maximum value can be up to `800m`, the rest are fluctuating around `250m`. After processing the all the rotation policies, keep it to about `6m`.

  The memory consumption of `multicluster-global-hub-manager` stay around `370 MiB` when rotating the policies. And then fall back to about `180 MiB` after updating all the policy status.

- The CPU and Memory consumption of the Postgres Pods
  ![Postgres CPU](./../images/global-hub-cpu-postgres.png)
  ![Postgres Memory](./../images/global-hub-memory-postgres.png)

- The CPU and Memory consumption of the Kafka pods
  ![Kafka CPU](./../images/global-hub-cpu-kafka.png)
  ![Kafka Memory](./../images/global-hub-memory-kafka.png)

## Related Material

- [Red Hat Advanced Cluster Management Hub-of-hubs Scale and Performance Tests](https://docs.google.com/presentation/d/1z6hESoacKRHuBQ-7I8nqWBuMnw7Z6CAw/edit#slide=id.p1)
- [Replace Global Hub Transport with Cloudevents](https://github.com/stolostron/multicluster-global-hub/issues/310)
