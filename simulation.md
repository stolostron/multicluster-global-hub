### Simulation at high scale

To simulate at high scale `leaf-hub-status-sync` component must be replaced with
[leaf-hub-simulator](https://github.com/open-cluster-management/leaf-hub-status-sync/tree/leaf-hub-simulator).

Please follow the instructions at [leaf-hub-simulator](https://github.com/open-cluster-management/leaf-hub-status-sync/tree/leaf-hub-simulator) to deploy the component.

Pay attention to `NUMBER_OF_SIMULATED_LEAF_HUBS` environment variable - it defines the number of **additional** (simulated) LHs. If the environment variable is not provided or equal to 0, the simulator will behave as the original `leaf-hub-status-sync`.

The simulation uses the following tools:
* [CLC simulator](https://github.com/hanqiuzh/acm-clc-scale) - creates, deletes or keeps alive mock managed clusters (ManagedCluster CR).
* [GRC simulator](https://github.com/open-cluster-management/grc-simulator) - patches policy (Policy CR) compliance statuses.

**Recommendation:** install the above tools on a separate VM and run them with the `nohup` command (`nohup <YOUR COMMAND> &`) to ensure the tools are running even when the terminal to the VM is disconnected.

Before starting the simulation ensure the environment is "clean":
1. stop sync service at LH - `kubectl scale deployment sync-service-ess -n sync-service --replicas 0`
1. stop hoh-status-transport-bridge at HoH - `kubectl scale deployment hub-of-hubs-status-transport-bridge -n open-cluster-management --replicas 0`
1. stop leaf-hub-status-sync at LH - `kubectl scale deployment leaf-hub-status-sync -n open-cluster-management --replicas 0`
1. clean database tables - `delete from status.managed_clusters`, `delete from status.compliance`
1. clean sync service storage - you can use the tool [sync-service-simulation-cleaner](https://github.com/open-cluster-management/hub-of-hubs-sync-service/tree/main/tools/simulation-cleaner) which uses internally [edge-sync-service-client](https://github.com/open-horizon/edge-sync-service-client) to clean objects that were created during simulation.
1. start sync service at LH - `kubectl scale deployment sync-service-ess -n sync-service --replicas 1`
1. start hoh-status-transport-bridge at HoH - `kubectl scale deployment hub-of-hubs-status-transport-bridge -n open-cluster-management --replicas 1`
1. start leaf-hub-status-sync at LH - `kubectl scale deployment leaf-hub-status-sync -n open-cluster-management --replicas 1`

You have two options to run CLC simulator:
1. The "create mock-cluster" mode - it creates the mock managed clusters and will keep them alive in case they weren't created before.
1. The "keep-alive mode" - it brings back the mock managed clusters to the `Ready` status in case they were created before.

Once all managed clusters are ready you can run GRC simulator to patch policy compliance status periodically.
