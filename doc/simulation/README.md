# Simulation at High Scale

## Environment

This simulation requires **two Red Hat OpenShift Container Platform clusters**. One cluster serves as the global hub, and the other serves as a regional hub with agent. In the regional hub, the [CLC simulator](https://github.com/hanqiuzh/acm-clc-scale) simulates a specific number of managed clusters. The [GRC simulator](https://github.com/stolostron/grc-simulator) continuously flips the compliant or noncompliant status of the policies. 

![Scale Test Environment](doc/architecture/scale-tests-environment-arch.png)

You can use the scale test environment to simulate a large-scale cluster. Since the transport status path is very sensitive to the performance and represents a scale larger than the spec path, we simulate this path. The following two ways are combined to achieve a large-scale global hub environment.

- Use the [CLC simulator](https://github.com/hanqiuzh/acm-clc-scale) to generate batches of managed clusters on the regional hub, for example, 1000.
- Simulate the global hub agent by changing the `leafHubName` when it sends messages to the global hub manager, or status path. Use [the snippet](https://github.com/stolostron/leaf-hub-status-sync/blob/51cffef679da0a38a2bb888bd3828b9782dfbb4c/pkg/controller/generic/generic_status_sync_controller.go#L255-L272) of the agent sending bundle to manger code for reference.

There are two main metrics to express the scalability of global hubs:
- Initialization: Time  to load all managed clusters and Policy to the Global Hub.
- Policy status rotation: Time to complete the compliance status rotation on all policies.

## Initialization

This scenario specifies how much time it took to send all of the managed clusters and policies in the first time. When the database is empty, 
you can connect the global hub manager to the agent, for example, 1000 agents connect around the same time. You can test it by running an SQL query using `COUNT` and verify the expected managed clusters and policies in the database.

Make sure that the initialization time is the time that passes until the expected result is returned. We can run queries to count the numbers of managed clusters and policies. The time between the first data appearance in the database and the expected `COUNT` is the test result. 

You can provide a [count script](cluster-stopwatch.sh) for the cluster, if you want to count policies. To enable a count script, change the `status.managed_clusters` to `status.compliance`. You can optimize and enhance it to make it more of what you need.

## Policy Status Rotation

Suppose that 1M policies on 100K managed clusters with 100 regional hubs are simulated to get the Policy status Rotation. You can simulate this by completing the following steps:

1. Build a regional hub, and then generate 1000 managed clusters by using the CLC simulator.
2. Create 10 policies on Global, and wait for them to be scheduled to the 1000 managed clusters, up until here we get 10K policies.
3. Update the code to simulate 100 Regional Hub by changing the `leafHubName` to 100 different values when sending the bundle.
4. Wait until both 100K managed clusters and 1M Policies are sent to the database. 
5. Start running the GRC simulator on Regional Hub (as the `startTime`) and change the 10K policies status from `non-compliant` to `compliant`. the `endTime` should be 1M `compliant` policies found in the database of global hub.
6. The time of Policies status rotation = `endTime` - `starTime`.

## Related Material
- [Red Hat Advanced Cluster Management Hub-of-hubs Scale and Performance Tests](https://docs.google.com/presentation/d/1z6hESoacKRHuBQ-7I8nqWBuMnw7Z6CAw/edit#slide=id.p1)
- [Replace Global Hub Transport with Cloudevents](https://github.com/stolostron/multicluster-global-hub/issues/310)

**Note:** Thanks to [Nir Rozenbaum](https://github.com/nirrozenbaum) and [Maroon Ayoub](https://github.com/vMaroon) for their support.
