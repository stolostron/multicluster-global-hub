# Simulation at High Scale

In order to better perceive the workload that the global hub can support, we need to do the performance test to get more specific numerical metrics.

Here we use two main metrics to express the scalability of global hubs:
- Initialization: time to load all MCs and Policy to Global Hub.
- Policy status rotation: time to perform compliance status rotation on all polices.

## Initialization

The meaning is how much time did it take to send from regional hub to global hub all MCs and policies in the first time. Assuming the database is empty, then 
we connect global hub manager to agent(e.g., 1000 agents connect around the same time). We test it by running SQL query using `COUNT` and verify we have the expected MCs and polices in the database.

What we need to make sure is that the initialization time is the time the pass until the expected result is returned. We can run queries to count the numbers of MCs and policies. So time between the first data appearance in the database and the expected `COUNT` is the test result. 

We provide a [count script](cluster-stopwatch.sh) for the cluster, if you want to count polices, just change the `status.managed_clusters` to `status.compliance`. Welcome you to optimize and enhance it to make it more reasonable.

## 