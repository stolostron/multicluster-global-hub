# ZTP ClusterInstance + GitOps Cluster Migration Guide

## Table of Contents

- [Overview](#overview)
- [Prerequisites](#prerequisites)
- [Migration Process](#migration-process)
- [Rollback Procedures](#rollback-procedures)
- [Summary](#summary)

## Overview

This guide provides comprehensive instructions for migrating Zero Touch Provisioning (ZTP) clusters managed by ClusterInstance and GitOps from one ACM hub to another using Multicluster Global Hub's migration feature.

### What is ZTP Migration?

ZTP migration enables you to transfer fully provisioned, bare-metal clusters (typically SNO - Single Node OpenShift) from one ACM hub cluster to another while maintaining cluster state, configuration, and GitOps management.

### What Gets Migrated

The migration process **automatically** transfers the following resources from source hub to target hub:

#### Core Resources
- **ManagedCluster**: Cluster registration and management
- **KlusterletAddonConfig**: Add-on configurations for managed clusters

#### Deployment Resources
- **ClusterDeployment**: Hive deployment configuration (including status)
- **ImageClusterInstall**: Image-based installation configuration (including status)

#### Secrets and Configmaps
- Cluster admin password (`<Cluster Name>-admin-password`)
- Cluster kubeconfig (`<Cluster Name>-admin-kubeconfig`)
- Pull secrets and other ClusterInstance-annotated secrets
- **Referenced Secrets** (automatically collected via `collectReferencedResources`):
  - **BMC Credentials**: Secrets referenced in `BareMetalHost.spec.bmc.credentialsName` for baseboard management controller access
  - **Pull Secrets**: Secrets referenced in `ClusterDeployment.spec.provisioning.pullSecretRef.name` for image registry authentication
  - **Extra Manifests ConfigMaps**: ConfigMaps referenced in `ImageClusterInstall.spec.extraManifestsRefs` for additional cluster configuration

#### Bare Metal Resources (with status preserved)
- **BareMetalHost**: Physical server inventory and state
- **HostFirmwareSettings**: BIOS/firmware configurations
- **FirmwareSchema**: Firmware setting schemas
- **HostFirmwareComponents**: Individual firmware components
- **DataImage**: OS images for bare metal hosts

### What is NOT Migrated

> **Important**: The following resources must be handled separately:

- **ClusterInstance Resources**: Managed by GitOps (Argo CD). You must manually apply GitOps applications to the target hub after cluster migration.
- **Policy Resources**: Must be redeployed via GitOps applications on target hub
- **Application Resources**: GitOps applications managing cluster configuration

See [GitOps Configuration Migration](#4-migrate-gitops-configuration-to-target-hub) for detailed instructions.

---

## Prerequisites

Before starting the migration, ensure both source and target hubs meet the following requirements.

### 1. Version Requirements

| Component | Source Hub | Target Hub | Notes |
|-----------|------------|------------|-------|
| **ACM** | Version N | Version N to N+1, and EUS Version support |  |
| **OpenShift** | 4.x | 4.x | Should be compatible with ACM version |
| **Global Hub** | Latest stable | N/A | Installed on source hub only |

### 2. Target Hub Configuration

The target hub must be configured with the **same components** in `MultiClusterEngine` and `MultiClusterHub` as the source hub to ensure successful cluster migration.

#### 2.1 Install and Configure ACM

**Step 1**: Install ACM Operator from OperatorHub

**Step 2**: Enable Siteconfig Operator

The siteconfig operator is required for managing ClusterInstance resources on the target hub.

```bash
cat <<EOF | oc apply -f -
apiVersion: operator.open-cluster-management.io/v1
kind: MultiClusterHub
metadata:
  name: multiclusterhub
  namespace: open-cluster-management
spec:
  overrides:
    components:
      # Enable siteconfig operator for ClusterInstance management
      - name: siteconfig
        enabled: true
        configOverrides: {}
EOF
```

**Step 3**: Verify Installation

```bash
# Check siteconfig operator deployment
oc get deployment -n open-cluster-management siteconfig-controller-manager

# Expected output:
# NAME                            READY   UP-TO-DATE   AVAILABLE   AGE
# siteconfig-controller-manager   1/1     1            1           5m
```

#### 2.2 Prepare Target Hub for ZTP

Follow the official [ZTP hub preparation guide](https://github.com/openshift-kni/telco-reference/blob/main/telco-ran/configuration/argocd/README.md#preparation-of-hub-cluster-for-ztp) to:

#### 2.3 Enable Observability (Optional)

> **Note**: Only required if your source hub has observability enabled.

If your source hub uses ACM Observability, configure it on the target hub:
Follow: https://github.com/stolostron/multicluster-observability-operator


### 3. Multicluster Global Hub Setup

#### 3.1 Install Global Hub on Source Hub

Install the Multicluster Global Hub operator on your **source hub**:
Follow the installation guide:
https://github.com/stolostron/multicluster-global-hub#run-the-operator-in-the-cluster

```bash
# Verify Global Hub is running
oc get mgh -n multicluster-global-hub
oc get pods -n multicluster-global-hub
```

#### 3.2 Import Target Hub to Global Hub

Import the target hub as a managed hub

> **Important**: The label `global-hub.open-cluster-management.io/deploy-mode: hosted` is **required** for migration to work.

For detailed import instructions, see the [ACM Import Documentation](https://docs.redhat.com/en/documentation/red_hat_advanced_cluster_management_for_kubernetes/2.15/html/clusters/cluster_mce_overview#import-intro).

---

## Migration Process

The migration process consists of six main steps: pre-migration verification, applying policy applications, creating the migration resource, monitoring progress, applying ClusterInstance applications, and cleanup.

### Migration Flow Overview

The following diagram illustrates the complete migration workflow:

```
┌────────────────────────────────────────────────────────────────────┐
│                    ZTP Cluster Migration Flow                      │
└────────────────────────────────────────────────────────────────────┘

Source Hub                  Global Hub                  Target Hub
    │                           │                            │
    │ 1. Pre-Migration          │                            │
    │    Verification           │                            │
    │    (Check clusters)       │                            │
    │                           │                            │
    │ 2. Apply Policy Apps ─────────────────────────────────>│
    │    (Manual)               │                            │
    │                           │                            │
    │ 3. Create Migration       │                            │
    │    Resource ──────────────>                            │
    │    (Manual)               │                            │
    │                           │                            │
    │ 4. Migration Execution    │                            │
    │    (Automatic) ───────────────────────────────────────>│
    │    - Transfer resources   │                            │
    │    - Register clusters    │                            │
    │                           │                            │
    │ 5. Apply ClusterInstance ─────────────────────────────>│
    │    Apps (Manual)          │                            │
    │                           │                            │
    │ 6. Verify (Manual)  ──────────────────────────────────>│
    │                           │                            │
    │ 7. Cleanup (Manual)       │                            │
    │                           │                            │
    ▼                           ▼                            ▼

Legend:
  ────> Manual step (user action required)
  ───> Automatic step (handled by Global Hub)

Key Points:
  • Apply Policy applications BEFORE creating migration resource
  • Migration automatically transfers cluster resources
  • Apply ClusterInstance applications AFTER migration completes
  • Cleanup only after full verification
```

### 1. Pre-Migration Verification

Before initiating migration, verify the current state of your clusters on the **source hub**.

#### 1.1 Check Cluster Status

```bash
# List all managed clusters
oc get managedcluster

# Expected output showing clusters to be migrated:
# NAME   HUB ACCEPTED   MANAGED CLUSTER URLS   JOINED   AVAILABLE   AGE
# sno1   true                                  True     True        30d
# sno2   true                                  True     True        25d
# sno3   true                                  True     True        20d
```

#### 1.2 Verify ClusterInstance Status

```bash
# Check ClusterInstance resources
oc get clusterinstance -A

# Expected output:
# NAMESPACE   NAME   PAUSED   PROVISIONSTATUS   PROVISIONDETAILS         AGE
# sno1        sno1            Completed         Provisioning completed   30d
# sno2        sno2            Completed         Provisioning completed   25d
# sno3        sno3            Completed         Provisioning completed   20d
```

> **Important**: Only migrate clusters with `PROVISIONSTATUS: Completed`. Clusters still provisioning should complete before migration.

### 2. Apply Policy Applications to Target Hub

> **Critical Step**: Policy applications must be applied to the target hub **BEFORE** creating the migration resource. This ensures that policies are ready to apply to clusters as soon as they are migrated.

#### 2.1 Understanding Policy Applications in ZTP

In ZTP deployments, ACM policies are typically managed through Argo CD applications. These policies control cluster configuration, compliance, and governance. Policies must be available on the target hub before clusters arrive.

#### 2.2 Export Policy Applications from Source Hub

Execute these commands on the **source hub**:

```bash
# Export policy applications
oc get application -n openshift-gitops -l app=policies -o yaml > policies-app.yaml
```

#### 2.3 Apply Policy Applications to Target Hub

Execute these commands on the **target hub**:

> **Note**: Ensure you are connected to the target hub cluster context before running these commands.

```bash
# Apply policy applications to target hub
oc apply -f policies-app.yaml
```

#### 2.4 Verify Policy Applications

```bash
# Check that policy applications are synced
oc get application -n openshift-gitops -l app=policies

# Check that policies are created
oc get policy -A

# Expected output should show policies in a healthy state
```

> **Important**: Wait for all policy applications to sync successfully before proceeding to create the migration resource. This typically takes 1-2 minutes.

### 3. Create Migration Resource

Create a `ManagedClusterMigration` custom resource to initiate the migration.

#### 3.1 Static Cluster List Migration

Use this approach when you have a specific list of clusters to migrate:

```bash
cat <<EOF | oc apply -f -
apiVersion: global-hub.open-cluster-management.io/v1alpha1
kind: ManagedClusterMigration
metadata:
  name: ztp-sno-migration
  namespace: multicluster-global-hub
spec:
  # Source hub (local-cluster refers to the source hub in brownfield mode)
  from: local-cluster

  # Target hub (name of the managed hub cluster)
  to: hub2

  # Explicitly list clusters to migrate
  includedManagedClusters:
    - sno1
    - sno2
    - sno3

  # Optional: Configure timeout for each migration phase
  # Default is 10 minutes per phase
  supportedConfigs:
    stageTimeout: 15m
EOF
```

#### 3.2 Dynamic Cluster Selection with Placement

Use this approach to select clusters dynamically based on labels:

**Step 1**: Create a Placement resource

```bash
cat <<EOF | oc apply -f -
apiVersion: cluster.open-cluster-management.io/v1beta1
kind: Placement
metadata:
  name: ztp-sno-placement
  namespace: multicluster-global-hub
spec:
  predicates:
    - requiredClusterSelector:
        labelSelector:
          matchLabels:
            cluster-type: sno
            environment: production
            region: us-east
  # Optional: Limit number of clusters
  numberOfClusters: 10
EOF
```

**Step 2**: Reference Placement in Migration

```bash
cat <<EOF | oc apply -f -
apiVersion: global-hub.open-cluster-management.io/v1alpha1
kind: ManagedClusterMigration
metadata:
  name: ztp-sno-migration
  namespace: multicluster-global-hub
spec:
  from: local-cluster
  to: hub2
  # Use Placement for dynamic cluster selection
  includedManagedClustersPlacementRef: ztp-sno-placement
  supportedConfigs:
    stageTimeout: 15m
EOF
```

### 4. Monitor Migration Progress

#### 4.1 Check Migration Phase

The migration progresses through several phases:

```bash
# View current migration status
oc get managedclustermigration ztp-sno-migration -n multicluster-global-hub

# Example output:
# NAME                PHASE        AGE
# ztp-sno-migration   Deploying    5m
```

**Migration Phases**:

1. **Initializing**: Preparing resources for migration
2. **Deploying**: Transferring resources to target hub
3. **Registering**: Registering clusters with target hub
4. **Completed**: Migration finished successfully
5. **RollingBack**: Automatic rollback in progress (if failure detected)
6. **Failed**: Migration failed, manual intervention required

#### 4.2 Monitor Individual Cluster Status

The migration controller automatically creates a ConfigMap tracking per-cluster status:

```bash
# View migration tracking ConfigMap
oc get configmap ztp-sno-migration -n multicluster-global-hub -o yaml
```

**Example ConfigMap Output**:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: ztp-sno-migration
  namespace: multicluster-global-hub
  ownerReferences:
    - apiVersion: global-hub.open-cluster-management.io/v1alpha1
      kind: ManagedClusterMigration
      name: ztp-sno-migration
data:
  # Successfully migrated clusters
  success: '["sno1","sno3"]'

  # Failed clusters (require investigation)
  failure: '["sno2"]'
```

### 5. Apply ClusterInstance Applications to Target Hub

> **Critical Step**: After migration completes successfully, you must apply ClusterInstance applications to the target hub. ClusterInstance resources are managed by GitOps (Argo CD) and are **NOT automatically migrated**.

#### 5.1 Understanding ClusterInstance Applications

ClusterInstance applications manage the declarative configuration of your ZTP clusters, including:
- ClusterInstance custom resources
- Cluster-specific secrets
- Infrastructure configurations

These applications must be applied **AFTER** the migration completes to ensure proper cluster management on the target hub.

#### 5.2 Export ClusterInstance Applications from Source Hub

Execute these commands on the **source hub**:

```bash
# Export ClusterInstance-related applications
# These manage ClusterInstance resources, secrets, and cluster configs
oc get application -n openshift-gitops <clusterinstance-sno-clusters> -o yaml > clusterinstance-app.yaml
```

#### 5.3 Apply ClusterInstance Applications to Target Hub

> **Note**: Execute these commands on the **target hub** cluster.

> **Important**: Only apply ClusterInstance applications AFTER the migration has reached the **Completed** phase. Check migration status with: `oc get managedclustermigration -n multicluster-global-hub`

```bash
# Apply ClusterInstance applications to target hub
oc apply -f clusterinstance-app.yaml
```

#### 5.4 Verify ClusterInstance Status on Target Hub

After GitOps sync completes, verify all ZTP resources are properly reconciled.

**Step 1**: Check ClusterInstance Resources

```bash
# Check ClusterInstance status on target hub
oc get clusterinstance -A

# Expected output:
# NAMESPACE   NAME   PAUSED   PROVISIONSTATUS   PROVISIONDETAILS         AGE
# sno1        sno1            Completed         Provisioning completed   1h
# sno2        sno2            Completed         Provisioning completed   1h
# sno3        sno3            Completed         Provisioning completed   1h

```

#### 5.5 Verify Policy Compliance

Ensure ACM policies are properly applied to migrated clusters.

```bash
# Check policy status on target hub
oc get policy -A
```

#### 5.6 Verify Managed Cluster Connection

```bash
# Check all migrated clusters are available
oc get managedcluster sno1,sno2,sno3
```

### 6. Cleanup Source Hub

> **Warning**: Only proceed with cleanup after confirming successful migration and verifying all clusters are fully operational on the target hub.

After verifying successful migration and GitOps reconciliation, clean up resources from the source hub to avoid resource waste.

#### 6.1 Remove GitOps Applications

```bash
# Delete specific ClusterInstance application
oc delete application clusterinstance-sno-clusters -n openshift-gitops

# Or delete multiple applications
for app in $(oc get application -n openshift-gitops -o name | grep -E 'sno1|sno2|sno3'); do
  echo "Deleting application: $app"
  oc delete $app -n openshift-gitops
done
```

#### 6.2 Delete ClusterInstance Resources

> **Important**: Deleting ClusterInstance resources triggers the siteconfig operator to clean up all related resources (ClusterDeployment, ImageClusterInstall, BareMetalHost, Secrets, and ClusterNamespace.). For the clusters which migrated successfully, Global Hub has added pause annotations to ClusterDeployment and ImageClusterInstall resources, preventing the deletion of these resources from affecting the actual running clusters.

```bash
# Delete individual ClusterInstances
oc delete clusterinstance sno1 -n sno1 --wait=false
oc delete clusterinstance sno2 -n sno2 --wait=false
oc delete clusterinstance sno3 -n sno3 --wait=false

# Or use a batch script
for cluster in sno1 sno2 sno3; do
  echo "Deleting ClusterInstance: ${cluster}"
  oc delete clusterinstance ${cluster} -n ${cluster} --wait=false || echo "Already deleted or not found"
done
```

#### 6.3 Verify Related Resources are Cleaned Up

```bash
# Wait for ClusterNamespace cleanup
oc get namespace -A | grep -E 'sno1|sno2|sno3'
```

---

## Rollback Procedures

If migration encounters issues, you can roll back to restore clusters to the source hub.

### Understanding Rollback

**Automatic Rollback**: If migration fails, Global Hub automatically attempts to rollback changes.
**Manual Rollback**: If automatic rollback fails, manual intervention is required.

### Automatic Rollback

Monitor automatic rollback:

```bash
# Check if migration entered rollback phase
oc get managedclustermigration ztp-sno-migration -n multicluster-global-hub'
```

If automatic rollback completes successfully:
- Clusters remain on source hub
- Resources on target hub are cleaned up
- Migration resource shows `Failed` phase

### Manual Rollback

If automatic rollback fails, follow these steps.

#### Manual Rollback: Target Hub Cleanup

Execute these steps on the **target hub** to remove partially migrated resources.

**Step 1**: Pause Reconciliation

```bash
# Switch to target hub
# Pause ClusterDeployment reconciliation
for cluster in sno1 sno2 sno3; do
  oc annotate clusterdeployment ${cluster} -n ${cluster} \
    hive.openshift.io/reconcile-pause=true --overwrite || echo "Not found: ${cluster}"
done

# Pause ImageClusterInstall reconciliation
for cluster in sno1 sno2 sno3; do
  oc annotate imageclusterinstall ${cluster} -n ${cluster} \
    hive.openshift.io/reconcile-pause=true --overwrite || echo "Not found: ${cluster}"
done
```

**Step 2**: Remove Resource Finalizers

```bash
# Remove ClusterDeployment finalizers
for cluster in sno1 sno2 sno3; do
  oc patch clusterdeployment ${cluster} -n ${cluster} --type json \
    -p='[{"op": "remove", "path": "/metadata/finalizers"}]' || echo "Not found: ${cluster}"
done

# Remove ImageClusterInstall finalizers
for cluster in sno1 sno2 sno3; do
  oc patch imageclusterinstall ${cluster} -n ${cluster} --type json \
    -p='[{"op": "remove", "path": "/metadata/finalizers"}]' || echo "Not found: ${cluster}"
done
```

**Step 3**: Force Delete Resources

```bash
# Delete ClusterDeployment
for cluster in sno1 sno2 sno3; do
  oc delete clusterdeployment ${cluster} -n ${cluster} --wait=false || echo "Not found: ${cluster}"
done

# Delete ImageClusterInstall
for cluster in sno1 sno2 sno3; do
  oc delete imageclusterinstall ${cluster} -n ${cluster} --wait=false || echo "Not found: ${cluster}"
done

# Delete ManagedCluster
for cluster in sno1 sno2 sno3; do
  oc delete managedcluster ${cluster} --wait=false || echo "Not found: ${cluster}"
done

# Delete namespaces
oc delete namespace sno1 sno2 sno3 --wait=false
```

#### Manual Rollback: Source Hub Restoration

Execute these steps on the **source hub** to restore cluster connectivity.

**Step 1**: Remove Pause Annotations

```bash
# Switch to source hub
# Remove pause annotation from ClusterDeployment
for cluster in sno1 sno2 sno3; do
  oc annotate clusterdeployment ${cluster} -n ${cluster} \
    hive.openshift.io/reconcile-pause- || echo "Annotation not found: ${cluster}"
done

# Remove pause annotation from ImageClusterInstall
for cluster in sno1 sno2 sno3; do
  oc annotate imageclusterinstall ${cluster} -n ${cluster} \
    hive.openshift.io/reconcile-pause- || echo "Annotation not found: ${cluster}"
done
```

**Step 2**: Remove Migration Annotations

```bash
# Remove migration-related annotations from ManagedCluster
for cluster in sno1 sno2 sno3; do
  oc annotate managedcluster ${cluster} \
    global-hub.open-cluster-management.io/migrating- \
    agent.open-cluster-management.io/klusterlet-config- || echo "Annotations not found: ${cluster}"
done
```

**Step 3**: Restore Hub Connectivity

```bash
# Re-enable hub connectivity
for cluster in sno1 sno2 sno3; do
  oc patch managedcluster ${cluster} --type json \
    -p='[{"op": "replace", "path": "/spec/hubAcceptsClient", "value": true}]' || echo "Failed to patch: ${cluster}"
done
```

**Step 4**: Verify Cluster Restoration

```bash
# Check cluster availability
for cluster in sno1 sno2 sno3; do
  echo "=== Cluster: $cluster ==="
  oc get managedcluster ${cluster} -o jsonpath='{.status.conditions[?(@.type=="ManagedClusterConditionAvailable")].status}'
  echo ""
done

# Wait for clusters to reconnect (may take 1-2 minutes)
watch oc get managedcluster sno1,sno2,sno3
```

**Step 5**: Verify ClusterInstance Status

```bash
# Check ClusterInstance resources are healthy
oc get clusterinstance -A

# Verify ClusterDeployment reconciliation resumed
oc get clusterdeployment -A -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.metadata.annotations.hive\.openshift\.io/reconcile-pause}{"\n"}{end}'

# Should show no pause annotations
```
---

## Summary

This comprehensive guide covered all aspects of ZTP cluster migration using Multicluster Global Hub:

### What You Accomplished

- ✅ **Prerequisites**: Configured target hub with ACM, siteconfig, and ZTP components
- ✅ **Global Hub Setup**: Installed Global Hub and imported target hub with hosted mode
- ✅ **Migration Execution**: Created migration resource and monitored progress through phases
- ✅ **GitOps Configuration**: Migrated Argo CD applications for ClusterInstance and policies
- ✅ **Verification**: Validated cluster health, ClusterInstance status, and policy compliance
- ✅ **Cleanup**: Removed resources from source hub post-migration
- ✅ **Rollback Knowledge**: Learned automatic and manual rollback procedures

### Related Documentation

- **[General Cluster Migration Guide](./global_hub_cluster_migration.md)**: For non-ZTP cluster migrations
- **[Manual Rollback Guide](./manual_rollback_guide.md)**: Detailed rollback procedures
- **[Migration Performance Guide](./migration_performance.md)**: Performance tuning and best practices
- **[ZTP Hub Preparation](https://github.com/openshift-kni/telco-reference/blob/main/telco-ran/configuration/argocd/README.md)**: Official ZTP setup documentation

---

