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
| Execution Model  | **Completed**: 100 clusters (6 rounds), 200 clusters (2 rounds), 300 clusters (2 rounds) |
| Key Metrics      | Total time, per-cluster average time, success rate, phase breakdown |
| Test Tools       | Migration CR status tracking, timestamp analysis |

---

## 4. Test Environment (Completed Setup)

> **Note:** The test environment has been set up with 2 ACM Hub clusters and 300+ ManagedClusters.

### 4.1 Actual Test Environment Configuration

#### ACM Hub Cluster: Hub1 (Source Hub)
- **Status**: ✅ **DEPLOYED** - Production hub with 300 managed clusters
- **Infrastructure**: 3 bare metal machines
- **ACM Version**: 2.14.0 (default configuration)
- **GlobalHub Version**: [Globalhub 1.6 daily build](https://github.com/stolostron/multicluster-global-hub-operator-catalog) installed with default config
- **Node Configuration (3 nodes)**:
  - **CPU**: 112 cores per node
  - **Memory**: 500 GB per node
  - **Storage**: 446 GB per node
- **Access**: Refer to team documentation for access details and image mirroring instructions
- **Role**: Source hub for migration testing
- **Managed Clusters**: 300 clusters already imported and managed

#### ACM Hub Cluster: Hub2 (Target Hub)
- **Status**: ✅ **DEPLOYED** - Target hub for migration testing
- **Infrastructure**: 3 virtual machines (hardware limitations)
- **ACM Version**: 2.14.0 (default configuration)
- **Node Configuration (3 nodes)**:
  - **CPU**: 30 cores per node
  - **Memory**: 360 GB per node
  - **Storage**: 128 GB per node
- **Role**: Target hub for migration testing

#### Managed Clusters (vm00001 → vm00297)
- **Total Clusters**: 297 managed clusters (vm00001 to vm00297)
- **Cluster Type**: Single Node OpenShift (SNO) clusters
- **Node Configuration (per cluster)**:
  - **CPU**: 8 cores
  - **Memory**: 17 GB
  - **Storage**: 48 GB
- **Deployment Method**: Zero-touch Provisioning (ZTP) with GitOps automation
- **Current State**: Imported and managed by Hub1
- **Test Coverage**: Up to 300 clusters tested in migration scenarios

#### GitOps Application Management
- **OpenShift GitOps Applications**:
  - **ztp-clusters-01**: Manages ~100 SNO clusters (vm00001-vm00100)
  - **ztp-clusters-02**: Manages ~100 SNO clusters (vm00101-vm00200) 
  - **ztp-clusters-03**: Manages ~97 SNO clusters (vm00201-vm00297)
- **Automation**: GitOps applications handle cluster deployment, configuration, and management
- **Network**: All clusters have connectivity to both Hub1 and Hub2

Reference: 
[ACM Performance and Scalability Guide](https://docs.redhat.com/en/documentation/red_hat_advanced_cluster_management_for_kubernetes/2.13/html/install/installing#performance-and-scalability)
[Globalhub Installation doc](https://github.com/stolostron/multicluster-global-hub-operator-catalog)
---

## 5. Test Execution Plan (Do test by Globalhub team)

### 5.1 Test Scenarios - **COMPLETED**

Migration performance tests completed at different scales:

| Scenario     | Cluster Count | Completed Migrations | Results |
|--------------|---------------|---------------------|---------|
| Scenario 1   | 100 clusters  | 6 rounds (3 forward + 3 backward) | ✅ 100% success |
| Scenario 2   | 200 clusters  | 2 rounds (1 forward + 1 backward) | ✅ 100% success |
| Scenario 3   | 300 clusters  | 2 rounds (1 forward + 1 backward) | ✅ 100% success |

> **Captured metrics:**
> - ✅ Migration time and phase breakdown
> - ✅ Success rate (100% across all scenarios)
> - ✅ Per-cluster performance analysis

---

## 6. Migration and Placement Configuration Examples

### 6.1 Placement Configuration

The migration tests use Placement resources to select target clusters for migration. Here's the placement configuration used for the 100-cluster test:

```yaml
apiVersion: cluster.open-cluster-management.io/v1beta1
kind: Placement
metadata:
  name: migration-100
spec:
  numberOfClusters: 100
  clusterSets:
  - global
  tolerations:
  - key: cluster.open-cluster-management.io/unreachable
    operator: Exists
  - key: cluster.open-cluster-management.io/unavailable
    operator: Exists
  predicates:
  - requiredClusterSelector:
      labelSelector:
        matchExpressions:
          - key: local-cluster
            operator: NotIn
            values:
              - "true"
          - key: global-hub.open-cluster-management.io/deploy-mode
            operator: DoesNotExist
```

**Key Configuration Details:**
- **numberOfClusters**: Limits selection to specified number of clusters
- **clusterSets**: Uses the 'global' clusterset
- **tolerations**: Allows migration of unreachable/unavailable clusters
- **predicates**: Excludes local-cluster and global-hub deployment clusters

### 6.2 Migration CR Examples

#### 100 Cluster Migration (Forward)
```yaml
apiVersion: global-hub.open-cluster-management.io/v1alpha1
kind: ManagedClusterMigration
metadata:
  name: migrate-100-cluster-round-1
  namespace: multicluster-global-hub
spec:
  from: local-cluster
  includedManagedClustersPlacementRef: migration-100
  supportedConfigs:
    stageTimeout: 1h0m0s
  to: mh1
```

#### 200 Cluster Migration (Forward)
```yaml
apiVersion: global-hub.open-cluster-management.io/v1alpha1
kind: ManagedClusterMigration
metadata:
  name: migrate-200-cluster-round-1
  namespace: multicluster-global-hub
spec:
  from: local-cluster
  includedManagedClustersPlacementRef: migration-200
  to: hub2
```

#### 300 Cluster Migration (Forward)
```yaml
apiVersion: global-hub.open-cluster-management.io/v1alpha1
kind: ManagedClusterMigration
metadata:
  name: migrate-300-cluster-round-1
  namespace: multicluster-global-hub
spec:
  from: local-cluster
  includedManagedClustersPlacementRef: migration-300
  supportedConfigs:
    stageTimeout: 10m0s
  to: hub2
```

#### Backward Migration Example (300 → local-cluster)
```yaml
apiVersion: global-hub.open-cluster-management.io/v1alpha1
kind: ManagedClusterMigration
metadata:
  name: migrate-300-cluster-round-1-bk
  namespace: multicluster-global-hub
spec:
  from: hub2
  includedManagedClustersPlacementRef: migration-300
  supportedConfigs:
    stageTimeout: 10m0s
  to: local-cluster
```

**Migration CR Configuration Notes:**
- **includedManagedClustersPlacementRef**: References the Placement resource for cluster selection
- **supportedConfigs.stageTimeout**: Configures timeout for each migration stage (10m default, 1h for 100-cluster tests)
- **from/to**: Specifies source and target hub clusters
- **namespace**: All migrations run in the `multicluster-global-hub` namespace

## 7. Test Results and Metrics
More details about migration resource usage and migration CR can be see [here](https://drive.google.com/drive/folders/1XGLJ0XoaQw6nSgJn48pvgDQEV5vIEHf3?usp=drive_link)

### 6.1 Test Summary

Based on the migration result files, we have conducted extensive testing with the following scenarios:

| Scenario | Cluster Count | Migration Rounds Completed | Success Rate |
|----------|---------------|----------------------------|--------------|
| 100 clusters | 100 | 3 forward + 3 backward = 6 total | 100% |
| 200 clusters | 200 | 1 forward + 1 backward = 2 total | 100%* |
| 300 clusters | 300 | 1 forward + 1 backward = 2 total | 100% |

*Note: 200 cluster migrations completed successfully but experienced cleanup timeout issues.

### 6.2 Scenario 1: 100 Cluster Migration Performance Results

#### Migration Time Metrics

| Round | Direction | Total Time (min:sec) | Success Rate (%) | Notes |
|-------|-----------|---------------------|------------------|-------|
| Round 1 | Forward (local-cluster → mh1) | 2:35 | 100% | Full completion |
| Round 2 | Forward (local-cluster → mh1) | 4:40 | 100% | Full completion |
| Round 3 | Forward (local-cluster → mh1) | 2:55 | 100% | Full completion |
| Round 1-BK | Backward (mh1 → local-cluster) | 2:05 | 100% | Full completion |
| Round 2-BK | Backward (mh1 → local-cluster) | 2:30 | 100% | Full completion |
| Round 3-BK | Backward (mh1 → local-cluster) | 2:05 | 100% | Full completion |

**Average Migration Time**: 2:52 (forward), 2:13 (backward)
**Stage Timeout Configuration**: 60 minutes

#### Phase Breakdown for 100 Clusters
- **Migration Start**: ~5 seconds
- **Resource Validation**: ~5 seconds  
- **Resource Initialization**: ~5-10 seconds
- **Resource Deployment**: ~20-30 seconds
- **Cluster Registration**: ~2-4 minutes (longest phase)
- **Resource Cleanup**: ~5-20 seconds

### 6.3 Scenario 2: 200 Cluster Migration Performance Results

#### Migration Time Metrics

| Round | Direction | Total Time (min:sec) | Success Rate (%) | Issues |
|-------|-----------|---------------------|------------------|--------|
| Round 1 | Forward (local-cluster → hub2) | 5:00 | 100% | Cleanup timeout |
| Round 1-BK | Backward (hub2 → local-cluster) | 5:40 | 100% | Cleanup timeout |

**Average Migration Time**: 5:20
**Stage Timeout Configuration**: Default (10 minutes)

#### Phase Breakdown for 200 Clusters
- **Migration Start**: ~5 seconds
- **Resource Validation**: ~5 seconds
- **Resource Initialization**: ~5-15 seconds  
- **Resource Deployment**: ~45 seconds
- **Cluster Registration**: ~4-5 minutes
- **Resource Cleanup**: Timeout (but migration marked as completed)

### 6.4 Scenario 3: 300 Cluster Migration Performance Results

#### Migration Time Metrics

| Round | Direction | Total Time (min:sec) | Success Rate (%) | Notes |
|-------|-----------|---------------------|------------------|-------|
| Round 1 | Forward (local-cluster → hub2) | 5:46 | 100% | Full completion |
| Round 1-BK | Backward (hub2 → local-cluster) | 7:45 | 100% | Full completion |

**Average Migration Time**: 6:46
**Stage Timeout Configuration**: 10 minutes

#### Phase Breakdown for 300 Clusters
- **Migration Start**: ~1 second
- **Resource Validation**: ~1 second
- **Resource Initialization**: ~10-20 seconds
- **Resource Deployment**: ~45-50 seconds
- **Cluster Registration**: ~4:45-5:35 minutes (longest phase)
- **Resource Cleanup**: ~5-65 seconds

### 6.5 Key Performance Insights

1. **Scalability**: Successfully tested up to 300 clusters with linear time scaling
2. **Cluster Registration Phase**: This is consistently the longest phase, taking 60-80% of total migration time
3. **Backward Migration**: Generally faster than forward migration (100 cluster scenario)
4. **Cleanup Issues**: 200 cluster scenario experienced cleanup timeouts but migrations completed successfully
5. **Configuration Impact**: Extended timeout (60min vs 10min) didn't significantly impact 100 cluster performance

---

## 7. Success Criteria Analysis

### 7.1 Success Criteria Met

✅ **Migration success rate**: **100%** (exceeds ≥ 90% target)
- All migrations across 100, 200, and 300 cluster scenarios completed successfully
- No failed migrations observed in any test scenario

✅ **Average time per cluster**: **Within acceptable performance baselines**
- 100 clusters: ~1.7 seconds per cluster (forward), ~1.3 seconds per cluster (backward)
- 200 clusters: ~1.6 seconds per cluster
- 300 clusters: ~1.4 seconds per cluster
- Performance improves slightly with scale due to parallel processing efficiency

✅ **No major performance degradation**: Confirmed
- Hub clusters remained stable throughout all migration tests
- No resource exhaustion or performance issues reported

✅ **No critical errors**: Confirmed
- All migrations completed without critical errors
- Minor cleanup timeout issues in 200-cluster scenario did not affect migration success

---


## 9. Timeline

| Phase      | Description                                         | Duration   |  
|------------|-----------------------------------------------------|------------|
| Round 1    | Initial validation, baseline performance collection | 1 week     |
| Round 2    | Fix and retest for performance improvements         | 1 week     |
| Round 3    | Final full-scale migration before ACM 2.15 GA       | 1 week     |

## 10. Issues

### 10.1 Identified Issues

1. **Klusterlet agent startup issues during migration** 
   - Issue: [ACM-23842](https://issues.redhat.com/browse/ACM-23842)
   - Status: No longer observed in current test results

2. **Registration-controller restart during migration**
   - Issue: [ACM-23840](https://issues.redhat.com/browse/ACM-23840)

3. **Message size too large error (200 clusters)** - ✅ **RESOLVED**
   - Previous Error: `Broker: Message size too large`
   - Fixed in pr: https://github.com/stolostron/multicluster-global-hub/pull/1940
   - Status: Successfully completed 200 and 300 cluster migrations without this error
