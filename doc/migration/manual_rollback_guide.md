# Manual Migration Rollback and Cleanup Guide

## Overview

The Multicluster Global Hub migration process consists of the following phases:
- **Pending** → **Validating** → **Initializing** → **Deploying** → **Registering** → **Cleaning** → **Completed**

### When Manual Rollback is Needed

- **Validating**: Migration hasn't started. No rollback needed - correct the CR and recreate.
- **Initializing/Deploying/Registering**: Automatic rollback is triggered on failure. If automatic rollback fails due to network issues or unexpected failures, manual intervention is required.
- **Cleaning**: Migration is essentially complete. Only manual cleanup of residual resources may be needed.

---

## Rollback Procedures by Phase

### 1. Initializing Phase Rollback

**Failure Scenario**: Network connectivity issues, ManagedServiceAccount creation failure, or bootstrap secret/KlusterletConfig configuration failure.

#### 1.1 Multicluster Global Hub Environment

Delete the ManagedServiceAccount created for the migration:

```bash
kubectl delete managedserviceaccount <migration-name> -n <target-hub-name>
```

#### 1.2 Source Hub

**Step 1: Delete the bootstrap secret** used for cluster registration

```bash
kubectl delete secret bootstrap-<target-hub-name> -n multicluster-engine
```

**Step 2: Delete the KlusterletConfig** created for migration

```bash
kubectl delete klusterletconfig migration-<target-hub-name>
```

**Step 3: Remove migration annotations** from each affected ManagedCluster

Annotations to remove:
- `global-hub.open-cluster-management.io/migrating`
- `agent.open-cluster-management.io/klusterlet-config`
- `import.open-cluster-management.io/disable-auto-import`

```bash
kubectl annotate managedcluster <cluster-name> \
  global-hub.open-cluster-management.io/migrating- \
  agent.open-cluster-management.io/klusterlet-config- \
  import.open-cluster-management.io/disable-auto-import-
```

<details>
<summary>Batch processing script</summary>

```bash
for CLUSTER in cluster1 cluster2 cluster3; do
  kubectl annotate managedcluster ${CLUSTER} \
    global-hub.open-cluster-management.io/migrating- \
    agent.open-cluster-management.io/klusterlet-config- \
    import.open-cluster-management.io/disable-auto-import-
done
```
</details>

**Step 4: Remove pause annotations from ZTP resources** (if applicable)

For clusters using ZTP (Zero Touch Provisioning), remove pause annotations that were added during migration:

| Resource Type | Pause Annotation |
|---------------|------------------|
| ClusterDeployment | `hive.openshift.io/reconcile-pause` |
| BareMetalHost | `baremetalhost.metal3.io/paused` |
| DataImage | `baremetalhost.metal3.io/paused` |

```bash
# Remove pause annotation from ClusterDeployment
kubectl annotate clusterdeployment <cluster-name> -n <cluster-name> \
  hive.openshift.io/reconcile-pause-

# Remove pause annotation from BareMetalHost
kubectl annotate baremetalhost <cluster-name> -n <cluster-name> \
  baremetalhost.metal3.io/paused-

# Remove pause annotation from DataImage (if exists)
kubectl annotate dataimage <cluster-name> -n <cluster-name> \
  baremetalhost.metal3.io/paused-
```

#### 1.3 Target Hub

**Step 1: Remove the auto-approve user** from ClusterManager's AutoApproveUsers list

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

#### 2.1 Multicluster Global Hub Environment

Same as [Initializing Phase - Global Hub](#11-multicluster-global-hub-environment)

#### 2.2 Source Hub

Perform all steps from [Initializing Phase - Source Hub](#12-source-hub):

- Step 1: Delete bootstrap secret
- Step 2: Delete KlusterletConfig
- Step 3: Remove migration annotations from ManagedClusters
- Step 4: Remove pause annotations from ZTP resources

#### 2.3 Target Hub

**Step 1: Delete deployed ManagedClusters and related resources**

The following resources are created during migration and need to be deleted:

| Category | Resource Type | Name/Pattern |
|----------|---------------|--------------|
| ACM Resources | ManagedCluster | `<cluster-name>` |
| | KlusterletAddonConfig | `<cluster-name>` |
| Secrets | Admin password | `<cluster-name>-admin-password` |
| | Admin kubeconfig | `<cluster-name>-admin-kubeconfig` |
| | Metadata JSON | `<cluster-name>-metadata-json` |
| | Seed reconfiguration | `<cluster-name>-seed-reconfiguration` |
| | Network secrets | Secrets with annotation `siteconfig.open-cluster-management.io/sync-wave` |
| | Preserve secrets | Secrets with label `siteconfig.open-cluster-management.io/preserve` |
| | BMC credentials | Referenced by BareMetalHost `spec.bmc.credentialsName` |
| | Pull secret | Referenced by ClusterDeployment `spec.pullSecretRef.name` |
| ConfigMaps | Preserve configmaps | ConfigMaps with label `siteconfig.open-cluster-management.io/preserve` |
| | Extra manifests | Referenced by ImageClusterInstall `spec.extraManifestsRefs` |
| Hive Resources | ClusterDeployment | `<cluster-name>` |
| | ImageClusterInstall | `<cluster-name>` |
| Metal3 Resources | BareMetalHost | In namespace `<cluster-name>` |
| | HostFirmwareSettings | In namespace `<cluster-name>` |
| | FirmwareSchema | In namespace `<cluster-name>` |
| | HostFirmwareComponents | In namespace `<cluster-name>` |
| | DataImage | In namespace `<cluster-name>` |

For each cluster, delete resources in the following order:

```bash
# Step 1: Add pause annotations to ZTP resources before deletion (to prevent deprovision)
kubectl annotate clusterdeployment <cluster-name> -n <cluster-name> \
  hive.openshift.io/reconcile-pause=true --overwrite 2>/dev/null || true
kubectl annotate baremetalhost <cluster-name> -n <cluster-name> \
  baremetalhost.metal3.io/paused=true --overwrite 2>/dev/null || true
kubectl annotate dataimage <cluster-name> -n <cluster-name> \
  baremetalhost.metal3.io/paused=true --overwrite 2>/dev/null || true

# Step 2: Remove finalizers from ZTP resources
kubectl patch clusterdeployment <cluster-name> -n <cluster-name> --type=json \
  -p='[{"op": "remove", "path": "/metadata/finalizers"}]' 2>/dev/null || true
kubectl patch baremetalhost <cluster-name> -n <cluster-name> --type=json \
  -p='[{"op": "remove", "path": "/metadata/finalizers"}]' 2>/dev/null || true
kubectl patch dataimage <cluster-name> -n <cluster-name> --type=json \
  -p='[{"op": "remove", "path": "/metadata/finalizers"}]' 2>/dev/null || true

# Step 3: Delete Hive resources
kubectl delete clusterdeployment <cluster-name> -n <cluster-name> 2>/dev/null || true
kubectl delete imageclusterinstall <cluster-name> -n <cluster-name> 2>/dev/null || true

# Step 4: Delete Metal3 resources
kubectl delete baremetalhost <cluster-name> -n <cluster-name> 2>/dev/null || true
kubectl delete hostfirmwaresettings -n <cluster-name> --all 2>/dev/null || true
kubectl delete firmwareschema -n <cluster-name> --all 2>/dev/null || true
kubectl delete hostfirmwarecomponents -n <cluster-name> --all 2>/dev/null || true
kubectl delete dataimage -n <cluster-name> --all 2>/dev/null || true

# Step 5: Delete secrets
kubectl delete secret <cluster-name>-admin-password -n <cluster-name> 2>/dev/null || true
kubectl delete secret <cluster-name>-admin-kubeconfig -n <cluster-name> 2>/dev/null || true
kubectl delete secret <cluster-name>-metadata-json -n <cluster-name> 2>/dev/null || true
kubectl delete secret <cluster-name>-seed-reconfiguration -n <cluster-name> 2>/dev/null || true
# Delete BMC credentials secret (get name from BareMetalHost spec.bmc.credentialsName)
kubectl delete secret <bmc-secret-name> -n <cluster-name> 2>/dev/null || true
# Delete pull secret (get name from ClusterDeployment spec.pullSecretRef.name)
kubectl delete secret <pull-secret-name> -n <cluster-name> 2>/dev/null || true
# Delete siteconfig secrets
kubectl delete secret -n <cluster-name> -l siteconfig.open-cluster-management.io/preserve 2>/dev/null || true

# Step 6: Delete configmaps
kubectl delete configmap -n <cluster-name> -l siteconfig.open-cluster-management.io/preserve 2>/dev/null || true

# Step 7: Delete ACM resources
kubectl delete klusterletaddonconfig <cluster-name> -n <cluster-name> 2>/dev/null || true
kubectl delete managedcluster <cluster-name>

# Step 8: Delete cluster namespace (if not deleted automatically)
kubectl delete namespace <cluster-name> 2>/dev/null || true
```

<details>
<summary>Batch processing script</summary>

```bash
CLUSTERS=("cluster1" "cluster2" "cluster3")

for CLUSTER in "${CLUSTERS[@]}"; do
  echo "Cleaning up cluster: $CLUSTER"

  # Add pause annotations to ZTP resources
  kubectl annotate clusterdeployment ${CLUSTER} -n ${CLUSTER} \
    hive.openshift.io/reconcile-pause=true --overwrite 2>/dev/null || true
  kubectl annotate baremetalhost ${CLUSTER} -n ${CLUSTER} \
    baremetalhost.metal3.io/paused=true --overwrite 2>/dev/null || true
  kubectl annotate dataimage ${CLUSTER} -n ${CLUSTER} \
    baremetalhost.metal3.io/paused=true --overwrite 2>/dev/null || true

  # Remove finalizers from ZTP resources
  kubectl patch clusterdeployment ${CLUSTER} -n ${CLUSTER} --type=json \
    -p='[{"op": "remove", "path": "/metadata/finalizers"}]' 2>/dev/null || true
  kubectl patch baremetalhost ${CLUSTER} -n ${CLUSTER} --type=json \
    -p='[{"op": "remove", "path": "/metadata/finalizers"}]' 2>/dev/null || true
  kubectl patch dataimage ${CLUSTER} -n ${CLUSTER} --type=json \
    -p='[{"op": "remove", "path": "/metadata/finalizers"}]' 2>/dev/null || true

  # Delete Hive resources
  kubectl delete clusterdeployment ${CLUSTER} -n ${CLUSTER} 2>/dev/null || true
  kubectl delete imageclusterinstall ${CLUSTER} -n ${CLUSTER} 2>/dev/null || true

  # Delete Metal3 resources
  kubectl delete baremetalhost ${CLUSTER} -n ${CLUSTER} 2>/dev/null || true
  kubectl delete hostfirmwaresettings -n ${CLUSTER} --all 2>/dev/null || true
  kubectl delete firmwareschema -n ${CLUSTER} --all 2>/dev/null || true
  kubectl delete hostfirmwarecomponents -n ${CLUSTER} --all 2>/dev/null || true
  kubectl delete dataimage -n ${CLUSTER} --all 2>/dev/null || true

  # Delete secrets
  kubectl delete secret ${CLUSTER}-admin-password -n ${CLUSTER} 2>/dev/null || true
  kubectl delete secret ${CLUSTER}-admin-kubeconfig -n ${CLUSTER} 2>/dev/null || true
  kubectl delete secret ${CLUSTER}-metadata-json -n ${CLUSTER} 2>/dev/null || true
  kubectl delete secret -n ${CLUSTER} -l siteconfig.open-cluster-management.io/preserve 2>/dev/null || true

  # Delete configmaps
  kubectl delete configmap -n ${CLUSTER} -l siteconfig.open-cluster-management.io/preserve 2>/dev/null || true

  # Delete ACM resources
  kubectl delete klusterletaddonconfig ${CLUSTER} -n ${CLUSTER} 2>/dev/null || true
  kubectl delete managedcluster ${CLUSTER}

  # Delete namespace
  kubectl delete namespace ${CLUSTER} 2>/dev/null || true
done
```
</details>

**Step 2: Delete RBAC resources** same as [Initializing Phase - Target Hub](#13-target-hub)

---

### 3. Registering Phase Rollback

**Failure Scenario**: Network issues preventing cluster re-registration, cluster connection failures to target hub, or ManifestWork application timeout.

> **Important**: Some clusters may have successfully registered to the target hub while others failed. The migration controller tracks cluster migration status in a ConfigMap. You need to get the list of failed clusters for rollback:
>
> ```bash
> # ConfigMap name: <migration-name> (same as ManagedClusterMigration CR name)
> # Default Namespace: multicluster-global-hub
> # Key: "failure" - contains comma-separated list of failed cluster names
> kubectl get configmap <migration-name> -n <multicluster-global-hub-namespace> -o jsonpath='{.data.failure}'
> ```

#### 3.1 Multicluster Global Hub Environment

Same as [Initializing Phase - Global Hub](#11-multicluster-global-hub-environment)

#### 3.2 Source Hub

**Step 1: Delete managed-cluster-lease** for each failed cluster (to avoid connectivity issues)

```bash
kubectl delete lease managed-cluster-lease -n <cluster-name>
```

**Step 2: Set HubAcceptsClient to true** for each failed cluster to restore connectivity to source hub

Single cluster:
```bash
kubectl patch managedcluster <cluster-name> --type=json \
  -p='[{"op": "replace", "path": "/spec/hubAcceptsClient", "value": true}]'
```

<details>
<summary>Batch processing script using ConfigMap</summary>

```bash
# Step 1: Get failed clusters list from Global Hub (run this on Global Hub kubeconfig)
# The ConfigMap only exists on Global Hub
FAILED_CLUSTERS=$(kubectl get configmap <migration-name> -n multicluster-global-hub -o jsonpath='{.data.failure}' --kubeconfig=<global-hub-kubeconfig>)

# Step 2: Run on Source Hub to restore connectivity (switch to Source Hub kubeconfig)
# Convert comma-separated list to array
IFS=',' read -ra CLUSTERS <<< "$FAILED_CLUSTERS"

for CLUSTER in "${CLUSTERS[@]}"; do
  echo "Restoring connectivity for cluster: $CLUSTER"

  # Delete managed-cluster-lease
  kubectl delete lease managed-cluster-lease -n ${CLUSTER} --kubeconfig=<source-hub-kubeconfig> 2>/dev/null || true

  # Restore HubAcceptsClient
  kubectl patch managedcluster ${CLUSTER} --type=json \
    -p='[{"op": "replace", "path": "/spec/hubAcceptsClient", "value": true}]' \
    --kubeconfig=<source-hub-kubeconfig>
done
```
</details>

**Step 3: Perform all Initializing rollback steps** (for failed clusters only)

Refer to [Initializing Phase - Source Hub](#12-source-hub):

- Step 1: Delete bootstrap secret
- Step 2: Delete KlusterletConfig
- Step 3: Remove migration annotations
- Step 4: Remove pause annotations from ZTP resources

#### 3.3 Target Hub

Same as [Deploying Phase - Target Hub](#23-target-hub) (for failed clusters only):

- Step 1: Delete ManagedClusters and related resources
- Step 2: Delete RBAC resources

---

### 4. Cleaning Phase Manual Cleanup

**Scenario**: Migration completed successfully, but the cleaning phase failed to remove residual resources.

> **Note**: The cleaning phase removes resources from the source hub for clusters that successfully migrated to the target hub. You need to get the list of successfully migrated clusters from the ConfigMap:
>
> ```bash
> # ConfigMap name: <migration-name> (same as ManagedClusterMigration CR name)
> # Default Namespace: multicluster-global-hub
> # Key: "success" - contains comma-separated list of successfully migrated cluster names
> kubectl get configmap <migration-name> -n multicluster-global-hub -o jsonpath='{.data.success}'
> ```

#### 4.1 Multicluster Global Hub Environment

Delete the ManagedServiceAccount:

```bash
kubectl delete managedserviceaccount <migration-name> -n <target-hub-name>
```

#### 4.2 Source Hub

**Step 1: Delete bootstrap secret and KlusterletConfig**

```bash
kubectl delete secret bootstrap-<target-hub-name> -n multicluster-engine
kubectl delete klusterletconfig migration-<target-hub-name>
```

**Step 2: Remove all finalizers from ZTP resources** (if applicable)

For ZTP clusters, remove finalizers to allow cleanup:

```bash
# Remove finalizers from ClusterDeployment
kubectl patch clusterdeployment <cluster-name> -n <cluster-name> --type=json \
  -p='[{"op": "remove", "path": "/metadata/finalizers"}]' 2>/dev/null || true

# Remove finalizers from BareMetalHost
kubectl patch baremetalhost <cluster-name> -n <cluster-name> --type=json \
  -p='[{"op": "remove", "path": "/metadata/finalizers"}]' 2>/dev/null || true

# Remove finalizers from BMC credentials secret
kubectl patch secret <bmc-secret-name> -n <cluster-name> --type=json \
  -p='[{"op": "remove", "path": "/metadata/finalizers"}]' 2>/dev/null || true
```

**Step 3: Delete migrated ManagedClusters**

Single cluster:
```bash
kubectl delete managedcluster <cluster-name>
```

<details>
<summary>Batch processing script using ConfigMap</summary>

```bash
# Step 1: Get successfully migrated clusters list from Global Hub (run this on Global Hub kubeconfig)
# The ConfigMap only exists on Global Hub
SUCCESS_CLUSTERS=$(kubectl get configmap <migration-name> -n <multicluster-global-hub-namespace> -o jsonpath='{.data.success}' --kubeconfig=<global-hub-kubeconfig>)

# Step 2: Run on Source Hub to delete migrated clusters (switch to Source Hub kubeconfig)
# Convert comma-separated list to array
IFS=',' read -ra CLUSTERS <<< "$SUCCESS_CLUSTERS"

# Delete each successfully migrated cluster from source hub
for CLUSTER in "${CLUSTERS[@]}"; do
  echo "Cleaning up migrated cluster: $CLUSTER"

  # Remove finalizers from ZTP resources
  kubectl patch clusterdeployment ${CLUSTER} -n ${CLUSTER} --type=json \
    -p='[{"op": "remove", "path": "/metadata/finalizers"}]' 2>/dev/null || true
  kubectl patch baremetalhost ${CLUSTER} -n ${CLUSTER} --type=json \
    -p='[{"op": "remove", "path": "/metadata/finalizers"}]' 2>/dev/null || true

  # Delete ManagedCluster
  kubectl delete managedcluster ${CLUSTER} --kubeconfig=<source-hub-kubeconfig>
done
```
</details>

> **Note**: These clusters have already migrated to the target hub and should be removed from the source hub.

#### 4.3 Target Hub

**Step 1: Remove auto-import disable annotation** from migrated clusters

```bash
kubectl annotate managedcluster <cluster-name> \
  import.open-cluster-management.io/disable-auto-import-
```

**Step 2: Remove velero restore label from ImageClusterInstall** (if applicable)

```bash
kubectl label imageclusterinstall <cluster-name> -n <cluster-name> \
  velero.io/restore-name-
```

**Step 3: Delete migration RBAC resources** same as [Initializing Phase - Target Hub](#13-target-hub)
