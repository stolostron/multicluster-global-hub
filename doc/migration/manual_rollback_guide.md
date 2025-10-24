# Manual Migration Rollback Guide

## Overview

The Global Hub cluster migration process consists of the following phases:
- **Pending** → **Validating** → **Initializing** → **Deploying** → **Registering** → **Cleaning** → **Completed**

### When Manual Rollback is Needed

- **Validating**: Migration hasn't started. No rollback needed - correct the CR and recreate.
- **Initializing/Deploying/Registering**: Automatic rollback is triggered on failure. If automatic rollback fails due to network issues or unexpected failures, manual intervention is required.
- **Cleaning**: Migration is essentially complete. Only manual cleanup of residual resources may be needed.

---

## Rollback Procedures by Phase

### 1. Initializing Phase Rollback

**Failure Scenario**: Network connectivity issues, ManagedServiceAccount creation failure, or bootstrap secret/KlusterletConfig configuration failure.

#### 1.1 Global Hub

**Step 1: Delete the ManagedServiceAccount** created for the migration

```bash
kubectl delete managedserviceaccount <migration-name> -n <target-hub-name>
```

**Step 2: Delete the migration status ConfigMap** (if exists)

```bash
kubectl delete configmap <migration-name>-clusters -n multicluster-global-hub-system --ignore-not-found=true
```

#### 1.2 Source Hub

**Step 1: Remove migration annotations** from each affected ManagedCluster

Annotations to remove:
- `global-hub.open-cluster-management.io/migrating`
- `agent.open-cluster-management.io/klusterlet-config`

```bash
kubectl annotate managedcluster <cluster-name> \
  global-hub.open-cluster-management.io/migrating- \
  agent.open-cluster-management.io/klusterlet-config-
```

<details>
<summary>Batch processing script</summary>

```bash
for CLUSTER in cluster1 cluster2 cluster3; do
  kubectl annotate managedcluster ${CLUSTER} \
    global-hub.open-cluster-management.io/migrating- \
    agent.open-cluster-management.io/klusterlet-config-
done
```
</details>

**Step 2: Delete the bootstrap secret** used for cluster registration

- Name format: `bootstrap-<target-hub-name>`
- Namespace: `multicluster-engine`

```bash
kubectl delete secret bootstrap-<target-hub-name> -n multicluster-engine
```

**Step 3: Delete the KlusterletConfig** created for migration

- Name format: `migration-<target-hub-name>`

```bash
kubectl delete klusterletconfig migration-<target-hub-name>
```

#### 1.3 Target Hub

**Step 1: Remove the auto-approve user** from ClusterManager's AutoApproveUsers list

- User format: `system:serviceaccount:<target-hub-name>:<migration-name>`

```bash
kubectl edit clustermanager cluster-manager
# Remove the line containing the MSA user from spec.registrationConfiguration.autoApproveUsers
```

**Step 2: Delete migration RBAC resources**

| Resource Type | Name Format |
|---------------|-------------|
| ClusterRole (SubjectAccessReview) | `global-hub-migration-<migration-name>-sar` |
| ClusterRoleBinding (SubjectAccessReview) | `global-hub-migration-<migration-name>-sar` |
| ClusterRoleBinding (Registration) | `global-hub-migration-<migration-name>-registration` |

```bash
kubectl delete clusterrole global-hub-migration-<migration-name>-sar
kubectl delete clusterrolebinding global-hub-migration-<migration-name>-sar
kubectl delete clusterrolebinding global-hub-migration-<migration-name>-registration
```

---

### 2. Deploying Phase Rollback

**Failure Scenario**: Kafka broker network issues, resource application failure on target hub, or deployment confirmation timeout.

#### 2.1 Global Hub

Same as [Initializing Phase - Global Hub](#11-global-hub)

#### 2.2 Source Hub

Perform all steps from [Initializing Phase - Source Hub](#12-source-hub):

- Step 1: Remove migration annotations from ManagedClusters
- Step 2: Delete bootstrap secret
- Step 3: Delete KlusterletConfig

#### 2.3 Target Hub

**Step 1: Delete deployed ManagedClusters** that were created during migration

```bash
kubectl delete managedcluster <cluster-name>
```

**Step 2: Delete KlusterletAddonConfigs**

- Name: `<cluster-name>`
- Namespace: `<cluster-name>`

```bash
kubectl delete klusterletaddonconfig <cluster-name> -n <cluster-name>
```

<details>
<summary>Batch processing script</summary>

```bash
CLUSTERS=("cluster1" "cluster2" "cluster3")

for CLUSTER in "${CLUSTERS[@]}"; do
  kubectl delete managedcluster ${CLUSTER}
  kubectl delete klusterletaddonconfig ${CLUSTER} -n ${CLUSTER}
done
```
</details>

**Step 3: Delete RBAC resources** same as [Initializing Phase - Target Hub](#13-target-hub)

---

### 3. Registering Phase Rollback

**Failure Scenario**: Network issues preventing cluster re-registration, cluster connection failures to target hub, or ManifestWork application timeout.

> **Important**: Some clusters may have successfully registered to the target hub while others failed. The migration controller tracks cluster migration status in a ConfigMap.
>
> **Get failed clusters list** (clusters that need rollback):
>
> ```bash
> # ConfigMap name format: <migration-name>-clusters
> # Namespace: multicluster-global-hub-system
> # Key: "failure" - contains comma-separated list of failed cluster names
> kubectl get configmap <migration-name>-clusters -n multicluster-global-hub-system -o jsonpath='{.data.failure}'
> ```
>
> **Get successful clusters list**:
>
> ```bash
> # Key: "success" - contains comma-separated list of successfully migrated cluster names
> kubectl get configmap <migration-name>-clusters -n multicluster-global-hub-system -o jsonpath='{.data.success}'
> ```

#### 3.1 Global Hub

Same as [Initializing Phase - Global Hub](#11-global-hub)

#### 3.2 Source Hub

**Step 1: Set HubAcceptsClient to true** for each failed cluster to restore connectivity to source hub

Single cluster:
```bash
kubectl patch managedcluster <cluster-name> --type=json \
  -p='[{"op": "replace", "path": "/spec/hubAcceptsClient", "value": true}]'
```

<details>
<summary>Batch processing script using ConfigMap</summary>

```bash
# Get failed clusters from ConfigMap
FAILED_CLUSTERS=$(kubectl get configmap <migration-name>-clusters -n multicluster-global-hub-system -o jsonpath='{.data.failure}')

# Convert comma-separated list to array
IFS=',' read -ra CLUSTERS <<< "$FAILED_CLUSTERS"

# Restore HubAcceptsClient for each failed cluster
for CLUSTER in "${CLUSTERS[@]}"; do
  echo "Restoring connectivity for cluster: $CLUSTER"
  kubectl patch managedcluster ${CLUSTER} --type=json \
    -p='[{"op": "replace", "path": "/spec/hubAcceptsClient", "value": true}]'
done
```
</details>

**Step 2: Perform all Initializing rollback steps**

Refer to [Initializing Phase - Source Hub](#12-source-hub):

- Step 1: Remove migration annotations (for failed clusters only)
- Step 2: Delete bootstrap secret
- Step 3: Delete KlusterletConfig

#### 3.3 Target Hub

Same as [Deploying Phase - Target Hub](#23-target-hub):

- Step 1: Delete ManagedClusters
- Step 2: Delete KlusterletAddonConfigs
- Step 3: Delete RBAC resources

---

### 4. Cleaning Phase Manual Cleanup

**Scenario**: Migration completed successfully, but the cleaning phase failed to remove residual resources.

> **Note**: The cleaning phase removes resources from the source hub for clusters that successfully migrated to the target hub. You can get the list of successfully migrated clusters from the ConfigMap:
>
> ```bash
> # Get the list of successfully migrated clusters
> kubectl get configmap <migration-name>-clusters -n multicluster-global-hub-system -o jsonpath='{.data.success}'
> ```

#### 4.1 Global Hub

**Step 1: Delete the ManagedServiceAccount**

```bash
kubectl delete managedserviceaccount <migration-name> -n <target-hub-name>
```

**Step 2: Delete the migration status ConfigMap**

```bash
kubectl delete configmap <migration-name>-clusters -n multicluster-global-hub-system
```

#### 4.2 Source Hub

**Step 1: Delete bootstrap secret and KlusterletConfig**

```bash
kubectl delete secret bootstrap-<target-hub-name> -n multicluster-engine
kubectl delete klusterletconfig migration-<target-hub-name>
```

**Step 2: Delete migrated ManagedClusters** where `spec.hubAcceptsClient=false`

Single cluster:
```bash
kubectl delete managedcluster <cluster-name>
```

<details>
<summary>Batch processing script using ConfigMap</summary>

```bash
# Get successfully migrated clusters from ConfigMap
SUCCESS_CLUSTERS=$(kubectl get configmap <migration-name>-clusters -n multicluster-global-hub-system -o jsonpath='{.data.success}')

# Convert comma-separated list to array
IFS=',' read -ra CLUSTERS <<< "$SUCCESS_CLUSTERS"

# Delete each successfully migrated cluster from source hub
for CLUSTER in "${CLUSTERS[@]}"; do
  echo "Deleting migrated cluster: $CLUSTER"
  kubectl delete managedcluster ${CLUSTER}
done
```
</details>

> **Note**: These clusters have already migrated to the target hub and should be removed from the source hub.

#### 4.3 Target Hub

Delete migration RBAC resources same as [Initializing Phase - Target Hub](#13-target-hub)

---

## Validation Checklist

After completing manual rollback, verify the following:

### Source Hub
- [ ] Migration annotations removed from all ManagedClusters
- [ ] Bootstrap secret `bootstrap-<target-hub-name>` deleted
- [ ] KlusterletConfig `migration-<target-hub-name>` deleted
- [ ] For Registering rollback: `spec.hubAcceptsClient=true` for failed clusters
- [ ] Clusters showing as Available in source hub

### Target Hub
- [ ] For Deploying/Registering rollback: ManagedClusters deleted
- [ ] For Deploying/Registering rollback: KlusterletAddonConfigs deleted
- [ ] MSA user removed from ClusterManager AutoApproveUsers
- [ ] All migration RBAC resources deleted (ClusterRole and ClusterRoleBindings)

### Global Hub
- [ ] ManagedServiceAccount deleted
- [ ] Migration status ConfigMap deleted (if applicable)
- [ ] ManagedClusterMigration CR shows phase as "Failed" (for rollback) or "Completed" (for cleaning)

---

## Common Issues and Troubleshooting

### Issue 1: Cannot Remove Annotations from ManagedCluster

**Symptom**

Conflict errors when removing annotations

**Solution**

Retry with exponential backoff:

```bash
for i in {1..5}; do
  kubectl annotate managedcluster <cluster-name> \
    global-hub.open-cluster-management.io/migrating- \
    agent.open-cluster-management.io/klusterlet-config- && break
  sleep $((i * 2))
done
```

### Issue 2: ManagedCluster Stuck in Terminating State

**Symptom**

`kubectl delete managedcluster` hangs

**Solution**

Remove finalizers:

```bash
kubectl patch managedcluster <cluster-name> --type=json \
  -p='[{"op": "remove", "path": "/metadata/finalizers"}]'
```

### Issue 3: Cannot Update ClusterManager

**Symptom**

Validation errors when editing ClusterManager

**Solution**

Use strategic merge patch:

```bash
kubectl patch clustermanager cluster-manager --type=merge -p '{
  "spec": {
    "registrationConfiguration": {
      "autoApproveUsers": []
    }
  }
}'
```

---

## Resource Reference

### Key Resources and Naming Conventions

| Resource Type | Name Format | Location |
|---------------|-------------|----------|
| ManagedServiceAccount | `<migration-name>` | Global Hub, namespace: `<target-hub-name>` |
| Bootstrap Secret | `bootstrap-<target-hub-name>` | Source Hub, namespace: `multicluster-engine` |
| KlusterletConfig | `migration-<target-hub-name>` | Source Hub |
| ConfigMap (Migration Status) | `<migration-name>-clusters` | Global Hub, namespace: `multicluster-global-hub-system` |
| ClusterRole (SubjectAccessReview) | `global-hub-migration-<migration-name>-sar` | Target Hub |
| ClusterRoleBinding (SubjectAccessReview) | `global-hub-migration-<migration-name>-sar` | Target Hub |
| ClusterRoleBinding (Registration) | `global-hub-migration-<migration-name>-registration` | Target Hub |

### Migration Annotations

| Annotation Key | Applied To | Purpose |
|----------------|------------|---------|
| `global-hub.open-cluster-management.io/migrating` | ManagedCluster (Source Hub) | Mark cluster as migrating |
| `agent.open-cluster-management.io/klusterlet-config` | ManagedCluster (Source Hub) | Reference to KlusterletConfig |

### ConfigMap Data Keys

The migration status ConfigMap (`<migration-name>-clusters`) contains two keys that track cluster migration status:

| Key | Purpose | Value Format |
|-----|---------|--------------|
| `success` | Stores clusters that successfully completed migration | Comma-separated list of cluster names |
| `failure` | Stores clusters that failed during migration | Comma-separated list of cluster names |

**Example**:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: migration-example-clusters
  namespace: multicluster-global-hub-system
data:
  success: "cluster1,cluster2,cluster3"
  failure: "cluster4,cluster5"
```
