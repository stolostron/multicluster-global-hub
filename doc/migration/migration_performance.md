# GlobalHub Migration Performance Test Plan

## 1. Background

GlobalHub provides a `Migration` feature that enables migrating a batch of `ManagedClusters` from one ACM Hub cluster (Hub1) to another (Hub2). This test plan is designed to evaluate the performance and scalability of this feature at various scales (50, 100, 200 clusters).

This is the [migration doc](./global_hub_cluster_migration.md), you can see more details about migration.

In this doc, the migration follows the [recommended approach](./global_hub_cluster_migration.md#-recommended-way-to-migrate-brownfield--hosted-mode) to test the performance

- **Source Hub (Hub1)**: ACM version **2.13**
- **Target Hub (Hub2)**: ACM version **2.15**
- **Test Strategy**: Three rounds of testing are planned:
  - **Round 1**: Functional verification, identify and fix performance issues.
  - **Round 2**: Re-test with fixes to evaluate performance improvements.
  - **Round 3**: Full-scale test before the ACM 2.15 GA release.

---

## 2. Objectives

- Validate the functionality and stability of the GlobalHub migration process at different cluster scales.
- Quantitatively measure key performance metrics: total migration time, per-cluster duration, CPU and memory usage, and success rate.
- Identify bottlenecks and areas for future optimization.

---

## 3. Scope of Testing

### 3.1 Performance Testing Scope

| Item             | Description                                  |
|------------------|----------------------------------------------|
| Migration Type   | Batch migration from Hub1 to Hub2            |
| Execution Model  | One-time migrations of 50 / 100 / 200 clusters |
| Key Metrics      | Total time, per-cluster average time, CPU/memory usage, success rate |
| Test Tools       | OpenShift Metrics, Prometheus, Grafana       |

---

## 4. Test Environment(Need performance team help to prepare the test env)

> **Note:** The test requires 2 ACM Hub clusters and up to 200 ManagedClusters. 

### 4.1 Cluster & Resource Requirements

#### ACM Hub Cluster: Hub1
- OpenShift Version: 4.18
- ACM Version: 2.13.x (default configuration)
- GlobalHub Version: [Globalhub 1.6 daily build](https://github.com/stolostron/multicluster-global-hub-operator-catalog) installed with default config
- Minimum resources:
  - 3 nodes
  - 30 vCPUs per node
  - 128 GB memory per node
  - 1 TB SSD storage

#### ACM Hub Cluster: Hub2
- OpenShift Version: 4.19 or 4.20
- ACM Version: 2.15.x (default configuration)
- Minimum resources:
  - 3 nodes
  - 30 vCPUs per node
  - 128 GB memory per node
  - 1 TB SSD storage

#### Managed Clusters (×200)
- Cluster type: Single-node OpenShift (SNO)
- OpenShift Versions: 4.18 to 4.20
- Resources per cluster:
  - 8 vCPUs
  - 18 GB memory
- Initially imported to Hub1

#### Additional Requirements
1. Hub1 and Hub2 must be able to communicate via kubeconfig.
2. All ManagedClusters must have network connectivity to both Hub1 and Hub2.

Reference: 
[ACM Performance and Scalability Guide](https://docs.redhat.com/en/documentation/red_hat_advanced_cluster_management_for_kubernetes/2.13/html/install/installing#performance-and-scalability)
[Globalhub Installation doc](https://github.com/stolostron/multicluster-global-hub-operator-catalog)
---

## 5. Test Execution Plan (Do test by Globalhub team)

### 5.1 Test Scenarios

We will run migration performance tests at different migration scales:

| Scenario     | Cluster Count | Number of Migrations |
|--------------|----------------|------------------------|
| Scenario 1   | 50 clusters     | 4 rounds               |
| Scenario 2   | 100 clusters    | 2 rounds               |
| Scenario 3   | 200 clusters    | 1 round                |

> For each scenario, we will capture:
> - Migration time
> - Success rate
> - CPU and memory usage across key components

---

## 6. Metrics Collection

### 6.1 Scenario 1: Migrate 50 clusters × 4 rounds

The migration CR should like:

```yaml
apiVersion: global-hub.open-cluster-management.io/v1alpha1
kind: ManagedClusterMigration
metadata:
  name: migration-1
spec:
  from: local-cluster
  includedManagedClusters:
    - cluster1
    - cluster2
    - ... ## List cluster1-cluster50 here
    - cluster 50
  to: hub2
```

#### Migration Time Metrics

| Round | Cluster Count  | Success Rate (%) | Total Time (sec) | Max Time (sec) | Min Time (sec) | Avg Time (sec) |
|-------|----------------|------------------|------------------|----------------|----------------|----------------|
|       |                |                  |                  |                |                |                |

#### Resource Usage Metrics
- Node-level CPU & Memory usage
- GlobalHub manager: CPU & Memory
- Hub1 components:
  - `open-cluster-management-hub` namespace
  - `multicluster-engine` namespace
  - GlobalHub agent pod
- Hub2 components:
  - `open-cluster-management-hub` namespace
  - `multicluster-engine` namespace
  - GlobalHub agent pod

### 6.2 Scenario 2: Migrate 100 clusters × 2 rounds
_(Same metrics as above)_

### 6.3 Scenario 3: Migrate 200 clusters × 1 round
_(Same metrics as above)_

---

## 7. Success Criteria

- **Migration success rate**: ≥ 90%
- **Average time per cluster**: Within acceptable baseline (to be defined in Round 1)
- **No major performance degradation** on Hub clusters or managed clusters during/after migration
- **No critical errors** or stuck resources post-migration

---

## 8. Out of Scope

The following scenarios are not covered in this test plan:

1. **Cluster rollback after migration failure**: Rollback or recovery of clusters in case of a failed migration is out of scope for this performance test and will be handled in a separate story.

---

## 9. Timeline

| Phase      | Description                                         | Duration   |
|------------|-----------------------------------------------------|------------|
| Round 1    | Initial validation, baseline performance collection | 1 week     |
| Round 2    | Fix and retest for performance improvements         | 1 week     |
| Round 3    | Final full-scale migration before ACM 2.15 GA       | 1 week     |
