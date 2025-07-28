# 🧭 Managed Cluster Migration (Tech Preview)

## What Is It?

**Multicluster Global Hub** introduces **Managed Cluster Migration**, a feature that allows you to move managed clusters from one ACM hub cluster to another.

This provides a unified way to reorganize or rebalance workloads across multiple hub clusters without manual reconfiguration.

![alt text](../images/migration-overview.png)

---

## Why Use It?

Multicluster Global Hub is built to manage large-scale fleets of clusters using an event-driven architecture. Traditionally, the Global Hub acts as a bridge between itself and managed hubs. With Managed Cluster Migration, it can now also act as a communication and orchestration layer between multiple hub clusters.

Because Global Hub is event-based, it can efficiently track, sync, and transfer cluster state across hubs, making it an ideal tool for cross-hub migration.

---

## How It Works?

### 🏗️ Architecture

The migration process involves coordination between a source hub and a target hub:

- The **Global Hub Manager** (`multicluster-global-hub-manager`) monitors `ManagedClusterMigration` resources and controls the migration flow.

- **The Global Hub Agent** (`multicluster-global-hub-agent`) performs migration tasks on the source and target hubs.

<details>
<summary> Migration Workflow </summary>

>![arch](../images/migration-workflow.jpg)

</details>


### 🔄 Migration Phases

Each migration goes through several phases, visible in the resource `status.phase` and `conditions`:

| Phase        | Description                                                                 |
|--------------|-----------------------------------------------------------------------------|
| Pending      | Only one migration can be handled at a time; others will remain pending     |
| Validating   | Verifies clusters and hubs are valid. Failures go directly to Failed.      |
| Initializing | Prepares target hub (kubeconfig, RBAC) and source hub (`KubeletConfig`).    |
| Deploying    | Migrates selected clusters.                                                 |
| Registering  | Re-registers the cluster to the target hub.                                 |
| Rollbacking  | Attempts to restore system to original state when migration fails. Always transitions to Failed. |
| Cleaning     | Cleans up resources from both hubs. Always transitions to Completed, even if cleanup fails (with warnings). |
| Completed    | Migration completed successfully.                                           |
| Failed       | Migration failed; error message included in status.                         |

### 🔄 Migration Flow Diagram

#### Normal Flow
```
Pending → Validating → Initializing → Deploying → Registering → Cleaning → Completed
```

#### Failure Handling Flow
```
- Validating (failure) → Failed
- Initializing/Deploying/Registering (failure) → Rollbacking → Failed  
- Rollbacking (success/failure) → Failed
- Cleaning (success/failure) → Completed (with warnings if failed)
```

### 🛠️ Error Handling & Rollback Mechanism

The migration system implements sophisticated error handling with different strategies for each phase:

#### Validation Phase Failures
- **Behavior**: Direct transition to `Failed` state
- **Rationale**: Early validation failures indicate fundamental issues that cannot be recovered
- **Examples**: Hub not found, cluster name conflicts, invalid configurations

#### Core Migration Phase Failures (Initializing/Deploying/Registering)
- **Behavior**: Transition to `Rollbacking` phase, then to `Failed`
- **Rationale**: These phases may have created resources that need cleanup
- **Rollback Actions**:
  - Restore original cluster configurations on source hubs
  - Clean up partially created resources on target hubs  
  - Remove migration-related configurations

#### Rollback Phase
- **Behavior**: Always transitions to `Failed` regardless of rollback success/failure
- **Rationale**: 
  - Rollback indicates original migration already failed
  - Even successful rollback means the intended migration didn't complete
  - Provides clear signal that manual intervention may be needed

#### Cleaning Phase
- **Behavior**: Always transitions to `Completed`, even if cleanup fails
- **Rationale**:
  - Core migration functionality is complete (clusters successfully migrated)
  - Cleanup failures only affect resource cleanup, not migration success
  - Warnings in conditions alert administrators to manual cleanup needs

#### Condition Types and Messages

| Condition Type | Success Reason | Failure Behavior |
|----------------|----------------|------------------|
| ResourceValidated | ResourceValidated | Direct to Failed |
| ResourceInitialized | ResourceInitialized | Triggers Rollback |
| ResourceDeployed | ResourcesDeployed | Triggers Rollback |
| ClusterRegistered | ClusterRegistered | Triggers Rollback |
| ResourceRolledBack | ResourceRolledBack | Always to Failed |
| ResourceCleaned | ResourceCleaned | Always to Completed (with warnings) |

---

## Deployment Modes

### Global Hub Installation Modes

- 🟢 Greenfield Mode

  ![alt text](../images/migration-deployment-greenfield-mode.png)

  - Deploy the **Global Hub** in a **separate ACM hub cluster**

- 🟤 Brownfield Mode

  ![alt text](../images/migration-deployment-brownfield-mode.png)

  - Deploy the Global Hub in **source** hub (hub1)
  - Deploy the Global Hub in the **target** hub (hub2)

### Importing Managed Hub Modes

Starting from Global Hub version 1.5.0, when importing an ACM hub into Global Hub, you must add the label `global-hub.open-cluster-management.io/deploy-mode` to indicate it is a Managed Hub. Without this label, the imported cluster will be treated as a standard managed cluster, meaning the Global Hub agent will not be installed.

Currently, the label supports two values: `default` and `hosted`. each suited for different scenarios:

* **`default`**: Use this when the ACM hub being imported does not have self-management enabled. In this case, both the Klusterlet and the Global Hub agent will be installed directly in the importing ACM hub cluster.

* **`hosted`**: Use this when the ACM hub being imported **has** self-management enabled. In default mode, the klusterlet from the `local-cluster` of Managed Hub may conflict with Global Hub. To avoid this, `hosted` mode should be used. However, there are two limitations within such mode:

  * The `kubeconfig` of the importing ACM hub cluster must never expire.
  * The Global Hub cluster must also have self-management enabled, so that the hosted Klusterlet can be properly reconciled by the operator from the `local-cluster` of Global Hub.

---

## 🧪 Recommended Way to Migrate (Brownfield & Hosted Mode)

**Recommended:** The preferred way to migrate is to install in a brownfield environment and import the cluster in hosted mode. The following example demonstrates this recommended approach.

![arch](../images/migration-sample.png)

### Step 1 – Deploy the Global Hub in Brownfield Mode

Install the Global Hub on `hub1`, and enable the agent running locally:

```yaml
  apiVersion: operator.open-cluster-management.io/v1alpha4
  kind: MulticlusterGlobalHub
  metadata:
    name: multiclusterglobalhub
    namespace: multicluster-global-hub
  spec:
    availabilityConfig: High
    installAgentOnLocal: true
  ```

> In this setup, the **Global Hub**, **source hub**, and `local-cluster` are all on `hub1`.

### Step 2 – Import the Managed Hub in Hosted Mode

Label `hub2` with the `hosted` deployment mode during importing:

```bash
global-hub.open-cluster-management.io/deploy-mode=hosted
```

### Step 3 – Create Migration Resource

Define the `ManagedClusterMigration` resource to move the cluster:

```yaml
apiVersion: global-hub.open-cluster-management.io/v1alpha1
kind: ManagedClusterMigration
metadata:
  name: migration-sample
spec:
  from: local-cluster
  includedManagedClusters:
    - cluster1
  to: hub2
```

#### Field Explanations:

- `from`: The source hub (in this case, `local-cluster` = `hub1`)
- `to`: Target hub (`hub2`)
- `includedManagedClusters`: Lists the clusters to be migrated. All cluster names must be unique across hubs.

---

### Step 4 – Sample Migration Status

#### Successful Migration
```yaml
status:
  conditions:
    - type: ResourceValidated
      status: "True"
      message: Migration resources have been validated
    - type: ResourceInitialized
      status: "True"
      message: All source and target hubs have been initialized
    - type: ResourceDeployed
      status: "True"
      message: Resources have been successfully deployed to the target hub cluster
    - type: ClusterRegistered
      status: "True"
      message: All migrated clusters have been successfully registered
    - type: ResourceCleaned
      status: "True"
      message: Resources have been successfully cleaned up from the hub clusters
  phase: Completed
```

#### Failed Migration with Successful Rollback
```yaml
status:
  conditions:
    - type: ResourceValidated
      status: "True"
      message: Migration resources have been validated
    - type: ResourceInitialized
      status: "True"
      message: All source and target hubs have been initialized
    - type: ResourceDeployed
      status: "False"
      message: Failed to deploy resources to target hub due to network timeout
    - type: ResourceRolledBack
      status: "True"
      message: Migration rollback completed. Migration marked as failed due to original failure.
  phase: Failed
```

#### Migration with Cleanup Warnings
```yaml
status:
  conditions:
    - type: ResourceValidated
      status: "True"
      message: Migration resources have been validated
    - type: ResourceInitialized
      status: "True"
      message: All source and target hubs have been initialized
    - type: ResourceDeployed
      status: "True"
      message: Resources have been successfully deployed to the target hub cluster
    - type: ClusterRegistered
      status: "True"
      message: All migrated clusters have been successfully registered
    - type: ResourceCleaned
      status: "True"
      message: "[Warning - Cleanup Issues] Failed to delete some temporary resources. Migration completed despite cleanup issues. Manual cleanup may be required."
  phase: Completed
```

---

## ✅ Summary

Managed Cluster Migration helps you:

- Reorganize cluster ownership between ACM hub clusters
- Move clusters
- Automate re-registration and cleanup
- Track every step with detailed status updates

> ⚠️ **Note:** This feature is currently in **Tech Preview**. Feedback and contributions are welcome!