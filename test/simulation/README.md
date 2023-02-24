# Simulation at High Scale

## Environment

Basically we need **two REAL clusters**, one that serves as global hub, and the other serve as reginal hub with agent. In the single regional hub, we use the [CLC simulator](https://github.com/hanqiuzh/acm-clc-scale) to simulate a specific number of managed clusters. We also use [GRC simulator](https://github.com/stolostron/grc-simulator), so that it will continuously flip the compliant/non-compliant status of the policies. 

![Sale Test Environment](doc/architecture/scale-tests-environment-arch.png)

So far we have a Regional Hub, but how do we use it to simulate a large scale cluster? Since the transport status path is very sensitive to performance issues, and is the bottleneck that affects scalability performance. We will focus on the status path test, and combine the following two ways to achieve the large-scale global hub environment.

- Use [CLC simulator](https://github.com/hanqiuzh/acm-clc-scale) to generate batches of managed clusters on the regional hub, e.g. 1000.
- Simulate the global hub agent by changing the `leafHubName` when sends messages to the global hub manager(status path). [the snippet](https://github.com/stolostron/leaf-hub-status-sync/blob/51cffef679da0a38a2bb888bd3828b9782dfbb4c/pkg/controller/generic/generic_status_sync_controller.go#L255-L272) of the agent sending bundle to manger code for reference .

There are two main metrics to express the scalability of global hubs:
- Initialization: time to load all MCs and Policy to Global Hub.
- Policy status rotation: time to perform compliance status rotation on all polices.

## Initialization

The meaning is how much time did it take to send from regional hub to global hub all MCs and policies in the first time. Assuming the database is empty, then 
we connect global hub manager to agent(e.g., 1000 agents connect around the same time). We test it by running SQL query using `COUNT` and verify we have the expected MCs and polices in the database.

What we need to make sure is that the initialization time is the time the pass until the expected result is returned. We can run queries to count the numbers of MCs and policies. So time between the first data appearance in the database and the expected `COUNT` is the test result. 

We provide a [count script](cluster-stopwatch.sh) for the cluster, if you want to count polices, just change the `status.managed_clusters` to `status.compliance`. Welcome you to optimize and enhance it to make it more reasonable.

## Policy Status Rotation

Suppose that 1M policies on 100K MCs with 100 regional hubs are to be simulated to get the Policy status Rotation. We can simulate this in the following steps:

1. Build 1 regional hub and then generates 1000 MCs using the CLC simulator.
2. Create 10 policies on Global, and wait for them to be scheduled to the 1000 MCs, up until here we get 10K policies.
3. Update the code to simulate 100 Regional Hub by changing the leafHubName to 100 different values when sending the bundle.
4. Wait until both 100K MCs and 1M Policies are sent to the database. 
5. Start running the GRC simulator on Regional Hub (as the `startTime`) and change the 10K policies status from `non-compliant` to `compliant`. the `endTime` should be 1M `compliant` policies found in the database of global hub.
6. The time of Policies status rotation = `endTime` - `starTime`.

## Related Material
- [ACM Hub-of-hubs Scale and Performance Tests](https://docs.google.com/presentation/d/1z6hESoacKRHuBQ-7I8nqWBuMnw7Z6CAw/edit#slide=id.p1)
- [Replace Global Hub Transport with Cloudevents](https://github.com/stolostron/multicluster-global-hub/issues/310)

Note: Thanks to [Nir Rozenbaum](https://github.com/nirrozenbaum) and [Maroon Ayoub](https://github.com/vMaroon) for their support.