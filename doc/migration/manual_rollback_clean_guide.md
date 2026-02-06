# Manual Migration Rollback Guide

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

- Name format: same as provided in the migration event from Global Hub
- Namespace: `multicluster-engine`

```bash
kubectl delete secret <bootstrap-secret-name> -n multicluster-engine
```

**Step 2: Delete the KlusterletConfig** created for migration

- Name format: `migration-<target-hub-name>`

```bash
kubectl delete klusterletconfig migration-<target-hub-name>
```

**Step 3: Remove migration annotations** from each affected ManagedCluster

Annotations to remove:
- `global-hub.open-cluster-management.io/migrating`
- `agent.open-cluster-management.io/klusterlet-config`
- `open-cluster-management/disable-auto-import`

```bash
kubectl annotate managedcluster <cluster-name> \
  global-hub.open-cluster-management.io/migrating- \
  agent.open-cluster-management.io/klusterlet-config- \
  open-cluster-management/disable-auto-import-
```

<details>
<summary>Batch processing script</summary>

```bash
for CLUSTER in cluster1 cluster2 cluster3; do
  kubectl annotate managedcluster ${CLUSTER} \
    global-hub.open-cluster-management.io/migrating- \
    agent.open-cluster-management.io/klusterlet-config- \
    open-cluster-management/disable-auto-import-
done
```
</details>

**Step 4: Remove pause annotations from ZTP resources**

Remove pause annotations from the following resources for each cluster:
- **ClusterDeployment**: Remove `cluster.open-cluster-management.io/paused: "true"`
- **BareMetalHost**: Remove `baremetalhost.metal3.io/paused: "true"`
- **DataImage**: Remove `baremetalhost.metal3.io/paused: "true"`

```bash
# For ClusterDeployment
kubectl annotate clusterdeployment <cluster-name> -n <cluster-name> \
  cluster.open-cluster-management.io/paused-

# For BareMetalHost resources in cluster namespace
kubectl get baremetalhost -n <cluster-name> -o name | while read BMH; do
  kubectl annotate -n <cluster-name> ${BMH} \
    baremetalhost.metal3.io/paused-
done

# For DataImage resources in cluster namespace
kubectl get dataimage -n <cluster-name> -o name | while read DI; do
  kubectl annotate -n <cluster-name> ${DI} \
    baremetalhost.metal3.io/paused-
done
```

#### 1.3 Target Hub

**Step 1: Remove the auto-approve user** from ClusterManager's AutoApproveUsers list

- User format: `system:serviceaccount:<target-hub-name>:<migration-name>`

```bash
# Get the MSA user string
MSA_USER="system:serviceaccount:<target-hub-name>:<migration-name>"

# Edit ClusterManager and remove the MSA user from spec.registrationConfiguration.autoApproveUsers
kubectl edit clustermanager cluster-manager
# Manually remove the line containing $MSA_USER from the autoApproveUsers list
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

For each cluster, delete all migration resources including:
- ManagedCluster
- KlusterletAddonConfig
- ClusterDeployment
- BareMetalHost (and dependent BMC credentials secrets)
- DataImage
- ImageClusterInstall
- AgentClusterInstall
- InfraEnv
- NMStateConfig
- Secrets and ConfigMaps referenced by the above resources

> **Important**: Before deleting ZTP resources (ClusterDeployment, BareMetalHost, DataImage), pause annotations should be added and resource-specific finalizers should be removed to prevent reconciliation loops.

**Add pause annotations and remove finalizers for ZTP resources:**

```bash
# For ClusterDeployment - add pause annotation and remove /deprovision finalizer
kubectl annotate clusterdeployment <cluster-name> -n <cluster-name> \
  cluster.open-cluster-management.io/paused=true --overwrite
kubectl patch clusterdeployment <cluster-name> -n <cluster-name> --type json \
  -p='[{"op": "remove", "path": "/metadata/finalizers"}]'

# For BareMetalHost - add pause annotation and remove /deprovision finalizer
kubectl get baremetalhost -n <cluster-name> -o name | while read BMH; do
  kubectl annotate ${BMH} -n <cluster-name> \
    baremetalhost.metal3.io/paused=true --overwrite
  kubectl patch ${BMH} -n <cluster-name> --type json \
    -p='[{"op": "remove", "path": "/metadata/finalizers"}]'
done

# For DataImage - add pause annotation and remove /deprovision finalizer
kubectl get dataimage -n <cluster-name> -o name | while read DI; do
  kubectl annotate ${DI} -n <cluster-name> \
    baremetalhost.metal3.io/paused=true --overwrite
  kubectl patch ${DI} -n <cluster-name> --type json \
    -p='[{"op": "remove", "path": "/metadata/finalizers"}]'
done

# For AgentClusterInstall - remove /deprovision finalizer
kubectl patch agentclusterinstall <cluster-name> -n <cluster-name> --type json \
  -p='[{"op": "remove", "path": "/metadata/finalizers"}]' 2>/dev/null || true

# For ImageClusterInstall - remove /deprovision finalizer
kubectl patch imageclusterinstall <cluster-name> -n <cluster-name> --type json \
  -p='[{"op": "remove", "path": "/metadata/finalizers"}]' 2>/dev/null || true

# Remove finalizers from BMC credentials secrets
kubectl get baremetalhost -n <cluster-name> -o json 2>/dev/null | \
  jq -r '.items[].spec.bmc.credentialsName' | \
  while read SECRET_NAME; do
    if [ ! -z "$SECRET_NAME" ]; then
      kubectl patch secret $SECRET_NAME -n <cluster-name> --type json \
        -p='[{"op": "remove", "path": "/metadata/finalizers"}]'
    fi
  done
```

**Then delete the ManagedCluster and namespace:**

```bash
# Delete the ManagedCluster (this will cascade delete most resources)
kubectl delete managedcluster <cluster-name>

# If the cluster namespace is not automatically deleted, remove it manually
kubectl delete namespace <cluster-name>
```

<details>
<summary>Batch processing script</summary>

```bash
CLUSTERS=("cluster1" "cluster2" "cluster3")

for CLUSTER in "${CLUSTERS[@]}"; do
  echo "Processing cluster: ${CLUSTER}"

  # Add pause annotations and remove finalizers for ZTP resources
  # ClusterDeployment
  kubectl annotate clusterdeployment ${CLUSTER} -n ${CLUSTER} \
    cluster.open-cluster-management.io/paused=true --overwrite 2>/dev/null || true
  kubectl patch clusterdeployment ${CLUSTER} -n ${CLUSTER} --type json \
    -p='[{"op": "remove", "path": "/metadata/finalizers"}]' 2>/dev/null || true

  # BareMetalHost
  kubectl get baremetalhost -n ${CLUSTER} -o name 2>/dev/null | while read BMH; do
    kubectl annotate ${BMH} -n ${CLUSTER} \
      baremetalhost.metal3.io/paused=true --overwrite
    kubectl patch ${BMH} -n ${CLUSTER} --type json \
      -p='[{"op": "remove", "path": "/metadata/finalizers"}]'
  done

  # DataImage
  kubectl get dataimage -n ${CLUSTER} -o name 2>/dev/null | while read DI; do
    kubectl annotate ${DI} -n ${CLUSTER} \
      baremetalhost.metal3.io/paused=true --overwrite
    kubectl patch ${DI} -n ${CLUSTER} --type json \
      -p='[{"op": "remove", "path": "/metadata/finalizers"}]'
  done

  # AgentClusterInstall
  kubectl patch agentclusterinstall ${CLUSTER} -n ${CLUSTER} --type json \
    -p='[{"op": "remove", "path": "/metadata/finalizers"}]' 2>/dev/null || true

  # ImageClusterInstall
  kubectl patch imageclusterinstall ${CLUSTER} -n ${CLUSTER} --type json \
    -p='[{"op": "remove", "path": "/metadata/finalizers"}]' 2>/dev/null || true

  # BMC credentials secrets
  kubectl get baremetalhost -n ${CLUSTER} -o json 2>/dev/null | \
    jq -r '.items[].spec.bmc.credentialsName' | \
    while read SECRET_NAME; do
      if [ ! -z "$SECRET_NAME" ]; then
        kubectl patch secret $SECRET_NAME -n ${CLUSTER} --type json \
          -p='[{"op": "remove", "path": "/metadata/finalizers"}]'
      fi
    done

  # Delete ManagedCluster and namespace
  echo "Deleting ManagedCluster and namespace for: ${CLUSTER}"
  kubectl delete managedcluster ${CLUSTER} --ignore-not-found=true
  kubectl delete namespace ${CLUSTER} --ignore-not-found=true
done
```
</details>

**Step 2: Delete RBAC resources**

Same as [Initializing Phase - Target Hub](#13-target-hub):

- Remove auto-approve user from ClusterManager
- Delete ClusterRole and ClusterRoleBindings

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

Perform all Deploying rollback steps for failed clusters, plus additional steps:

- **Step 1-4**: Same as [Deploying Phase - Source Hub](#22-source-hub)
  - Delete bootstrap secret
  - Delete KlusterletConfig
  - Remove migration annotations
  - Remove pause annotations from ZTP resources

**Step 5: Delete managed-cluster-lease** for each failed cluster

```bash
kubectl delete lease managed-cluster-lease -n <cluster-name>
```

**Step 6: Set HubAcceptsClient to true** for each failed cluster to restore connectivity to source hub

```bash
kubectl patch managedcluster <cluster-name> --type=json \
  -p='[{"op": "replace", "path": "/spec/hubAcceptsClient", "value": true}]'
```

**Step 7: Wait for clusters to become available** on source hub

Monitor cluster status until all failed clusters report as Available:

```bash
kubectl get managedcluster <cluster-name> -o jsonpath='{.status.conditions[?(@.type=="ManagedClusterConditionAvailable")].status}'
```

Expected output: `True`

<details>
<summary>Batch processing script using ConfigMap</summary>

```bash
# Get failed clusters list from Global Hub
FAILED_CLUSTERS=$(kubectl get configmap <migration-name> -n multicluster-global-hub -o jsonpath='{.data.failure}' --kubeconfig=<global-hub-kubeconfig>)

# Switch to Source Hub kubeconfig and convert to array
IFS=',' read -ra CLUSTERS <<< "$FAILED_CLUSTERS"

# Perform Steps 1-4: Delete bootstrap secret, KlusterletConfig, and clean up cluster annotations
# (Use the same commands as in Deploying Phase - Source Hub section)
kubectl delete secret <bootstrap-secret-name> -n multicluster-engine
kubectl delete klusterletconfig migration-<target-hub-name>

for CLUSTER in "${CLUSTERS[@]}"; do
  echo "Processing cluster: $CLUSTER"

  # Remove migration annotations
  kubectl annotate managedcluster ${CLUSTER} \
    global-hub.open-cluster-management.io/migrating- \
    agent.open-cluster-management.io/klusterlet-config- \
    open-cluster-management/disable-auto-import-

  # Remove pause annotations from ZTP resources
  kubectl annotate clusterdeployment ${CLUSTER} -n ${CLUSTER} \
    cluster.open-cluster-management.io/paused- --ignore-not-found=true

  kubectl get baremetalhost -n ${CLUSTER} -o name 2>/dev/null | while read BMH; do
    kubectl annotate -n ${CLUSTER} ${BMH} baremetalhost.metal3.io/paused-
  done

  kubectl get dataimage -n ${CLUSTER} -o name 2>/dev/null | while read DI; do
    kubectl annotate -n ${CLUSTER} ${DI} baremetalhost.metal3.io/paused-
  done

  # Step 5: Delete managed-cluster-lease
  kubectl delete lease managed-cluster-lease -n ${CLUSTER} --ignore-not-found=true

  # Step 6: Restore HubAcceptsClient
  kubectl patch managedcluster ${CLUSTER} --type=json \
    -p='[{"op": "replace", "path": "/spec/hubAcceptsClient", "value": true}]'
done

# Step 7: Wait for clusters to become available (timeout: 10 minutes)
echo "Waiting for clusters to become available..."
TIMEOUT=600
ELAPSED=0
while [ $ELAPSED -lt $TIMEOUT ]; do
  ALL_AVAILABLE=true
  for CLUSTER in "${CLUSTERS[@]}"; do
    STATUS=$(kubectl get managedcluster ${CLUSTER} -o jsonpath='{.status.conditions[?(@.type=="ManagedClusterConditionAvailable")].status}' 2>/dev/null)
    if [ "$STATUS" != "True" ]; then
      ALL_AVAILABLE=false
      echo "Cluster ${CLUSTER} not yet available..."
      break
    fi
  done

  if [ "$ALL_AVAILABLE" = true ]; then
    echo "All clusters are available!"
    break
  fi

  sleep 5
  ELAPSED=$((ELAPSED + 5))
done

if [ $ELAPSED -ge $TIMEOUT ]; then
  echo "Timeout waiting for clusters to become available"
  exit 1
fi
```
</details>

#### 3.3 Target Hub

Same as [Deploying Phase - Target Hub](#23-target-hub):

- Step 1: Delete ManagedClusters and related resources for failed clusters
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
kubectl delete secret <bootstrap-secret-name> -n multicluster-engine
kubectl delete klusterletconfig migration-<target-hub-name>
```

**Step 2: Forcefully delete ObservabilityAddon resources**

For each successfully migrated cluster, the ObservabilityAddon must be forcefully deleted because the cluster's `hubAcceptsClient` was set to false:

```bash
# For each cluster namespace
kubectl delete observabilityaddon observability-addon -n <cluster-name>

# Remove finalizers if deletion is stuck
kubectl patch observabilityaddon observability-addon -n <cluster-name> \
  --type json -p='[{"op": "remove", "path": "/metadata/finalizers"}]'
```

**Step 3: Remove finalizers from ZTP resources**

Remove `/deprovision` finalizers from ZTP resources to allow proper cleanup:

```bash
# For ClusterDeployment
kubectl patch clusterdeployment <cluster-name> -n <cluster-name> --type json \
  -p='[{"op": "test", "path": "/metadata/finalizers", "value": ["clusterdeployment.hive.openshift.io/deprovision"]},
      {"op": "remove", "path": "/metadata/finalizers", "value": ["clusterdeployment.hive.openshift.io/deprovision"]}]'

# For BareMetalHost
kubectl get baremetalhost -n <cluster-name> -o name | while read BMH; do
  kubectl patch ${BMH} -n <cluster-name> --type json \
    -p='[{"op": "test", "path": "/metadata/finalizers", "value": ["baremetalhost.metal3.io/deprovision"]},
        {"op": "remove", "path": "/metadata/finalizers", "value": ["baremetalhost.metal3.io/deprovision"]}]'
done

# For AgentClusterInstall
kubectl patch agentclusterinstall <cluster-name> -n <cluster-name> --type json \
  -p='[{"op": "test", "path": "/metadata/finalizers", "value": ["agentclusterinstall.extensions.hive.openshift.io/deprovision"]},
      {"op": "remove", "path": "/metadata/finalizers", "value": ["agentclusterinstall.extensions.hive.openshift.io/deprovision"]}]'

# For ImageClusterInstall
kubectl patch imageclusterinstall <cluster-name> -n <cluster-name> --type json \
  -p='[{"op": "test", "path": "/metadata/finalizers", "value": ["imageclusterinstall.extensions.hive.openshift.io/deprovision"]},
      {"op": "remove", "path": "/metadata/finalizers", "value": ["imageclusterinstall.extensions.hive.openshift.io/deprovision"]}]'
```

**Step 4: Remove finalizers from BMC credentials secrets**

For clusters with BareMetalHost resources, remove finalizers from their BMC credentials secrets:

```bash
# List all BareMetalHost resources and get their BMC credentials secret names
kubectl get baremetalhost -n <cluster-name> -o json | \
  jq -r '.items[].spec.bmc.credentialsName' | \
  while read SECRET_NAME; do
    if [ ! -z "$SECRET_NAME" ]; then
      echo "Removing finalizers from BMC secret: $SECRET_NAME"
      kubectl patch secret $SECRET_NAME -n <cluster-name> --type json \
        -p='[{"op": "remove", "path": "/metadata/finalizers"}]'
    fi
  done
```

**Step 5: Delete migrated ManagedClusters**

Delete clusters where `spec.hubAcceptsClient=false` (successfully migrated to target hub):

```bash
kubectl delete managedcluster <cluster-name>
```

<details>
<summary>Batch processing script using ConfigMap</summary>

```bash
# Step 1: Get successfully migrated clusters list from Global Hub
SUCCESS_CLUSTERS=$(kubectl get configmap <migration-name> -n <multicluster-global-hub-namespace> -o jsonpath='{.data.success}' --kubeconfig=<global-hub-kubeconfig>)

# Step 2: Switch to Source Hub kubeconfig and process each cluster
IFS=',' read -ra CLUSTERS <<< "$SUCCESS_CLUSTERS"

# Step 3: Delete bootstrap secret and KlusterletConfig (once for all clusters)
kubectl delete secret <bootstrap-secret-name> -n multicluster-engine
kubectl delete klusterletconfig migration-<target-hub-name>

# Step 4: For each successfully migrated cluster
for CLUSTER in "${CLUSTERS[@]}"; do
  echo "Cleaning up migrated cluster: $CLUSTER"

  # Delete ObservabilityAddon
  kubectl delete observabilityaddon observability-addon -n ${CLUSTER} --ignore-not-found=true
  kubectl patch observabilityaddon observability-addon -n ${CLUSTER} \
    --type json -p='[{"op": "remove", "path": "/metadata/finalizers"}]' 2>/dev/null || true

  # Remove /deprovision finalizers from ZTP resources
  kubectl patch clusterdeployment ${CLUSTER} -n ${CLUSTER} --type json \
    -p='[{"op": "remove", "path": "/metadata/finalizers"}]' 2>/dev/null || true

  kubectl get baremetalhost -n ${CLUSTER} -o name 2>/dev/null | while read BMH; do
    kubectl patch ${BMH} -n ${CLUSTER} --type json \
      -p='[{"op": "remove", "path": "/metadata/finalizers"}]' 2>/dev/null || true
  done

  kubectl patch agentclusterinstall ${CLUSTER} -n ${CLUSTER} --type json \
    -p='[{"op": "remove", "path": "/metadata/finalizers"}]' 2>/dev/null || true

  kubectl patch imageclusterinstall ${CLUSTER} -n ${CLUSTER} --type json \
    -p='[{"op": "remove", "path": "/metadata/finalizers"}]' 2>/dev/null || true

  # Remove finalizers from BMC credentials secrets
  kubectl get baremetalhost -n ${CLUSTER} -o json 2>/dev/null | \
    jq -r '.items[].spec.bmc.credentialsName' | \
    while read SECRET_NAME; do
      if [ ! -z "$SECRET_NAME" ]; then
        kubectl patch secret $SECRET_NAME -n ${CLUSTER} --type json \
          -p='[{"op": "remove", "path": "/metadata/finalizers"}]' 2>/dev/null || true
      fi
    done

  # Delete the ManagedCluster
  kubectl delete managedcluster ${CLUSTER}
  echo "Deleted migrated cluster: $CLUSTER"
done
```
</details>

> **Note**: These clusters have already migrated to the target hub and should be removed from the source hub.

#### 4.3 Target Hub

**Step 1: Remove auto-import disable annotation** from successfully migrated clusters

```bash
kubectl annotate managedcluster <cluster-name> \
  open-cluster-management/disable-auto-import-
```

**Step 2: Remove velero restore label from ImageClusterInstall**

```bash
kubectl label imageclusterinstall <cluster-name> -n <cluster-name> \
  velero.io/restore-name-
```

**Step 3: Delete migration RBAC resources**

Same as [Initializing Phase - Target Hub](#13-target-hub):

- Remove auto-approve user from ClusterManager
- Delete ClusterRole and ClusterRoleBindings
