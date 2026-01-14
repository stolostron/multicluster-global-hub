# Managed Cluster Migration Workflow

This document describes the detailed workflow of managed cluster migration, including responsibilities of each component (Manager, Source Hub Agent, Target Hub Agent) at each phase.

> See also [migration-sequence-diagram.md](./migration-sequence-diagram.md) for the sequence diagram.

---

## Overview

Migration allows moving managed clusters from one hub to another. The process involves three components working together:

- **Manager**: Orchestrates the migration phases, sends events to agents, and tracks overall status
- **Source Hub Agent**: Handles operations on the source hub (collecting resources, triggering re-registration)
- **Target Hub Agent**: Handles operations on the target hub (applying resources, accepting cluster registration)

---

## Phase 1: Validating

**Purpose**: Ensure both hubs are ready and clusters are valid for migration.

### Manager Responsibilities

1. Validate that the source hub cluster exists and is in ready state
2. Validate that the target hub cluster exists and is in ready state
3. Send validation event to source hub with the list of clusters to migrate (or placement reference)
4. Send validation event to target hub with the list of clusters
5. Wait for status reports from both agents
6. On success, transition to Initializing phase
7. On failure, transition directly to Failed phase (no rollback needed)

### Source Hub Agent Responsibilities

1. Receive validation event from manager
2. If using placement reference, resolve the placement to get actual cluster names from PlacementDecisions
3. Verify each cluster exists in the source hub
4. Report validation result back to manager with the final cluster list
5. Report any errors for clusters that don't exist

### Target Hub Agent Responsibilities

1. Receive validation event from manager
2. Check that none of the clusters already exist in the target hub (to avoid conflicts)
3. Report validation result back to manager
4. Report any errors for clusters that already exist

---

## Phase 2: Initializing

**Purpose**: Set up authentication and configuration for the migration.

### Manager Responsibilities

1. Create a ManagedServiceAccount (MSA) resource in the target hub namespace
2. Wait for the MSA controller to generate a bootstrap secret containing the target hub kubeconfig
3. Retrieve the bootstrap secret and extract the kubeconfig data
4. Send initialization event to target hub with MSA information
5. Send initialization event to source hub with the bootstrap secret (kubeconfig for target hub)
6. Wait for status reports from both agents
7. On success, transition to Deploying phase
8. On failure, transition to Rollbacking phase

### Source Hub Agent Responsibilities

1. Receive initialization event from manager with bootstrap secret
2. Store the bootstrap secret in the source hub cluster
3. Create a KlusterletConfig resource named "migration-{target-hub}" with the bootstrap secret reference
4. Add migration annotations to each managed cluster:
   - Add klusterlet-config annotation pointing to the new KlusterletConfig
   - Add migrating annotation to mark clusters as being migrated
5. Report initialization complete to manager

### Target Hub Agent Responsibilities

1. Receive initialization event from manager with MSA information
2. Set up necessary RBAC permissions for the MSA
3. Configure auto-approval for incoming cluster registrations
4. Report initialization complete to manager

---

## Phase 3: Deploying

**Purpose**: Transfer cluster resources from source hub to target hub.

### Manager Responsibilities

1. Send deploying event to source hub to trigger resource collection
2. Send deploying event to target hub to prepare for receiving resources
3. Wait for source hub to report that all resources have been sent
4. Wait for target hub to report that all resources have been applied
5. On success, transition to Registering phase
6. On failure, transition to Rollbacking phase

### Source Hub Agent Responsibilities

1. Receive deploying event from manager
2. Add pause annotation to ClusterDeployment resources to prevent ZTP reconciliation during migration
3. Collect all resources for each managed cluster:
   - Cluster secrets: admin-password, admin-kubeconfig, metadata-json, seed-reconfiguration
   - Network secrets with sync-wave annotation
   - Preserved secrets and configmaps with preserve label
   - ManagedCluster resource
   - KlusterletAddonConfig resource
   - BareMetalHost and related firmware resources (metal3.io)
   - ImageClusterInstall resource (Hive)
   - ClusterDeployment resource (Hive)
4. Clean resource metadata (remove finalizers, resourceVersion, managedFields, etc.)
5. Batch resources into bundles (maximum 800 KiB per bundle to fit Kafka message size limit)
6. Send resource bundles to Kafka migration topic with totalclusters header for batch tracking
7. Report deploying complete to manager after all batches are sent

### Target Hub Agent Responsibilities

1. Receive deploying event from manager
2. Consume resource bundles from Kafka migration topic
3. Validate batch completeness using totalclusters header
4. Apply each resource to the target hub cluster:
   - Create secrets and configmaps in appropriate namespaces
   - Create ManagedCluster resources
   - Create KlusterletAddonConfig resources
   - Create ZTP resources (BareMetalHost, ClusterDeployment, etc.)
5. Handle conflicts and report per-cluster errors if any
6. Report deploying complete to manager after all clusters are processed

---

## Phase 4: Registering

**Purpose**: Re-register clusters with the target hub.

### Manager Responsibilities

1. Send registering event to source hub to trigger re-registration
2. Send registering event to target hub with registration timeout
3. Wait for target hub to report cluster registration status
4. Track successful and failed cluster registrations
5. On all success, transition to Cleaning phase
6. On partial success, skip rollback and transition to Cleaning (only clean successful clusters)
7. On all failure, transition to Rollbacking phase

### Source Hub Agent Responsibilities

1. Receive registering event from manager
2. For each managed cluster:
   - Set the bootstrap secret on the cluster (from KlusterletConfig)
   - Update ManagedCluster spec to set hubAcceptsClient = true
   - This triggers the klusterlet agent to reconnect to the target hub using the new bootstrap secret
3. Report registering complete to manager

### Target Hub Agent Responsibilities

1. Receive registering event from manager with timeout configuration
2. Monitor cluster registration status by watching ManagedCluster resources
3. Wait for each cluster to:
   - Have its CSR (Certificate Signing Request) auto-approved
   - Become available (agent connected and reporting)
4. Track successful and failed registrations
5. Report registration results to manager with per-cluster status

---

## Phase 5: Rollbacking (Error Path)

**Purpose**: Restore original state when migration fails.

### Manager Responsibilities

1. Determine which phase failed and set the rollback stage
2. Send rollback event to source hub with the failed stage information
3. Send rollback event to target hub with the failed stage information
4. Wait for both agents to complete rollback
5. Always transition to Failed phase after rollback (regardless of rollback success)

### Source Hub Agent Responsibilities

1. Receive rollback event from manager with the failed stage
2. Perform rollback operations based on how far the migration progressed:
   - If failed during initializing: Remove migration annotations, delete bootstrap secret, delete KlusterletConfig
   - If failed during deploying: Perform initializing rollback, remove pause annotations from ClusterDeployment
   - If failed during registering: Restore hubAcceptsClient to original value, perform deploying rollback
3. Report rollback complete to manager

### Target Hub Agent Responsibilities

1. Receive rollback event from manager with the failed stage
2. Perform rollback operations based on how far the migration progressed:
   - If failed during deploying: Delete any partially applied resources
   - If failed during registering: Delete ManagedCluster resources that were not successfully registered
3. Clean up MSA-related resources
4. Report rollback complete to manager

---

## Phase 6: Cleaning

**Purpose**: Clean up migration resources after successful migration.

### Manager Responsibilities

1. Delete the ManagedServiceAccount resource (this also revokes the bootstrap kubeconfig)
2. Send cleaning event to source hub to trigger resource cleanup
3. Send cleaning event to target hub to trigger resource cleanup
4. Wait for both agents to complete cleanup
5. Always transition to Completed phase (with warnings if cleanup fails)

### Source Hub Agent Responsibilities

1. Receive cleaning event from manager with the list of successfully migrated clusters
2. For each successfully migrated cluster:
   - Delete the ManagedCluster resource from source hub
   - Delete the associated KlusterletAddonConfig
   - Delete the ObservabilityAddon if present
   - Remove deprovision finalizers from ZTP resources (BareMetalHost, ClusterDeployment, etc.) to allow deletion
3. Delete the bootstrap secret
4. Delete the KlusterletConfig resource
5. Report cleaning complete to manager

### Target Hub Agent Responsibilities

1. Receive cleaning event from manager
2. Remove migration-related annotations from successfully migrated clusters
3. Remove pause annotations from ClusterDeployment to resume ZTP reconciliation
4. Clean up any temporary migration resources
5. Report cleaning complete to manager

---

## Communication Mechanism

### Event Types

| Event Type | Direction | Purpose |
|------------|-----------|---------|
| MigrationSourceBundle | Manager to Source Hub | Send migration commands to source hub |
| MigrationTargetBundle | Manager to Target Hub | Send migration commands to target hub |
| MigrationStatusBundle | Agent to Manager | Report phase status back to manager |
| MigrationResourceBundle | Source Hub to Target Hub | Transfer cluster resources via Kafka |

### Status Tracking

Manager maintains in-memory status per migration:
- Hub state map keyed by "hub-phase" (e.g., "source-hub-validating")
- Each state tracks: started flag, finished flag, error message, per-cluster errors
- Status persisted to ConfigMap for pod restart recovery

### Timeouts

| Phase | Default Timeout |
|-------|-----------------|
| Validating | 5 minutes |
| Initializing | 5 minutes |
| Deploying | 5 minutes |
| Registering | 12 minutes |
| Rollbacking | 12 minutes |
| Cleaning | 5 minutes |

Each phase is divided into 3 retry intervals. If the phase is not finished within an interval, manager resends the event.

---

## Key Annotations

| Annotation | Purpose |
|------------|---------|
| global-hub.open-cluster-management.io/migrating | Mark cluster as being migrated |
| agent.open-cluster-management.io/klusterlet-config | Reference to KlusterletConfig for migration |
| hive.openshift.io/reconcile-pause=true | Pause ZTP reconciliation during migration |

---

## Error Handling Summary

| Error Scenario | Behavior |
|----------------|----------|
| Hub not found or not ready | Direct to Failed (no rollback) |
| Cluster not in source hub | Direct to Failed (no rollback) |
| Cluster already in target hub | Direct to Failed (no rollback) |
| Initialization failure | Rollback then Failed |
| Deployment failure | Rollback then Failed |
| All registrations failed | Rollback then Failed |
| Partial registration success | Skip rollback, Clean successful only, Completed |
| Cleanup failure | Completed with warning |

---

## Code References

| Component | Location |
|-----------|----------|
| CRD Definition | operator/api/migration/v1alpha1/managedclustermigration_types.go |
| Manager Controller | manager/pkg/migration/migration_controller.go |
| Manager Phase Handlers | manager/pkg/migration/migration_validating.go, migration_initializing.go, etc. |
| Manager Status Handler | manager/pkg/status/handlers/clustermigration/managedclustermigration_handler.go |
| Agent Source Syncer | agent/pkg/spec/migration/migration_from_syncer.go |
| Agent Target Syncer | agent/pkg/spec/migration/migration_to_syncer.go |
| Bundle Definitions | pkg/bundle/migration/managedcluster_migration.go |
