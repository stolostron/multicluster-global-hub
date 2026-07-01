# Architecture — Multicluster Global Hub

**Multicluster Global Hub** enables ACM/OCM management at very high scale by aggregating multiple regional (managed) hub clusters under a single global hub. The global hub stores fleet-wide state in PostgreSQL and distributes workloads to managed hubs via Kafka.

Conceptual diagram: [doc/architecture/multicluster-global-hub-arch.png](../doc/architecture/multicluster-global-hub-arch.png)

---

## Components

| Component | Runs on | Role |
|---|---|---|
| **Operator** | Global hub cluster | Deploys manager, agent (via ManifestWork), Kafka, Postgres, Grafana; manages CRDs |
| **Manager** | Global hub cluster | Persists spec/status to PostgreSQL; publishes spec to Kafka; consumes status from Kafka |
| **Agent** | Managed hub clusters | Applies spec from Kafka to managed hub; reports status back via Kafka or Inventory API |
| **Grafana** | Global hub cluster | Observability dashboards backed by PostgreSQL |

---

## CRDs

### `MulticlusterGlobalHub` (`operator.open-cluster-management.io/v1alpha4`)

Short names: `mgh`, `mcgh`

| Field | Purpose |
|---|---|
| `spec.availabilityConfig` | `Basic` or `High` (default) — replica counts for HA |
| `spec.dataLayer.kafka.topics` | `specTopic` (default `gh-spec`), `statusTopic` (default `gh-status.*`) |
| `spec.dataLayer.kafka.consumerGroupPrefix` | Prefix for Kafka consumer group IDs |
| `spec.dataLayer.postgres.retention` | DB retention duration (default `18m`) |
| `spec.advanced` | Per-component resource overrides (manager, agent, kafka, postgres, grafana) |
| `spec.installAgentOnLocal` | Deploy agent on the local/global hub cluster (default `true`) |
| `spec.enableMetrics` | Enable metrics for built-in Kafka and Postgres |

Status tracks `phase`, `conditions`, and per-component readiness.

### `MulticlusterGlobalHubAgent` (`operator.open-cluster-management.io/v1alpha1`)

Short names: `mgha`, `mcgha`

Deployed on each managed hub cluster. Key field:

| Field | Purpose |
|---|---|
| `spec.transportConfigSecretName` | Secret containing `kafka.yaml` for connecting to global hub Kafka (default `transport-config`) |

### `ManagedClusterMigration` (`global-hub.open-cluster-management.io/v1alpha1`)

Short name: `mcm`

Migrates managed clusters from one regional hub to another. Specify clusters via `includedManagedClusters` or `includedManagedClustersPlacementRef` (mutually exclusive).

Phases: `Pending` → `Validating` → `Initializing` → `Deploying` → `Registering` → `Completed` (or `Failed` / `Rollbacking` / `Cleaning`).

---

## Data Flow

### Spec path (global hub → managed hubs)

1. ACM policies, placements, subscriptions, and other global resources are watched by **manager spec controllers**.
2. Controllers persist resource payloads to PostgreSQL (`local_spec.*` schemas).
3. **Spec syncers** read from the database and publish bundles to the `gh-spec` Kafka topic.
4. **Agent spec syncers** on each managed hub consume from Kafka and apply resources to the managed hub cluster.
5. **Agent workers** execute sync tasks with RBAC enforcement.

### Status path (managed hubs → global hub)

1. **Agent status controllers** watch managed-hub resources (policies, managed clusters, addons, etc.).
2. Status changes are bundled and emitted as CloudEvents to per-hub Kafka topics (`gh-status.<hub-name>`).
3. **Manager status dispatcher** routes bundles to the **conflator** (dedup/merge).
4. **Status handlers** persist to PostgreSQL (`local_status.*`, `status.*`, `event.*`, `history.*` schemas).
5. Nightly **compliance cronjob** summarizes policy compliance into `history.local_compliance`.

Dataflow diagram: [doc/architecture/mcgh-data-flow.png](../doc/architecture/mcgh-data-flow.png)

---

## PostgreSQL Schema

The operator applies SQL from `operator/pkg/controllers/storage/database/` on startup. Key schemas:

| Schema | Purpose | Example tables |
|---|---|---|
| `local_spec` | Desired-state resources from managed hubs | `policies`, placements, subscriptions |
| `local_status` | Current compliance/status per policy × cluster | `compliance` |
| `status` | Fleet inventory and hub health | `managed_clusters`, `leaf_hubs`, `leaf_hub_heartbeats` |
| `event` | Raw policy/cluster events (partitioned by date) | `local_policies`, `managed_clusters` |
| `history` | Time-series compliance summaries | `local_compliance`, `local_compliance_job_log` |

Retention is controlled by `spec.dataLayer.postgres.retention` on the `MulticlusterGlobalHub` CR. Built-in Postgres runs as `multicluster-global-hub-postgresql` in the global hub namespace.

---

## Kafka Topics

| Topic | Direction | Contents |
|---|---|---|
| `gh-spec` | Manager → all agents | Policy/placement/subscription spec bundles |
| `gh-status.*` | Agents → manager | Per-hub status CloudEvents (`gh-status.hub1`, etc.) |
| `gh-status` | Agents → manager (BYO Kafka) | Single shared status topic when not using built-in per-hub topics |

Consumer group IDs:

- Global hub manager: `<prefix>global_hub`
- Managed hub agent: `<prefix><hub-name>` (hyphens → underscores)

Generate transport config for agents: `tools/generate-kafka-config.sh`.

---

## Controller Flow

### Operator reconciler

1. Watches `MulticlusterGlobalHub` CR.
2. Deploys Postgres, Kafka (Strimzi), manager, Grafana on the global hub.
3. Creates `MulticlusterGlobalHubAgent` ManifestWorks for each managed hub (or local hub if `installAgentOnLocal`).
4. Applies database schema and upgrade SQL.
5. Updates CR status with component readiness.

### Manager spec controllers

1. Watch ACM/OCM resources on the global hub (policies, placements, etc.).
2. Upsert resource JSON into `local_spec.*` tables.
3. Spec syncers poll/notify on DB changes and publish to `gh-spec`.

### Manager status pipeline

1. Kafka consumer receives status bundles from `gh-status.*` topics.
2. Conflator merges partial bundles for the same resource.
3. Dispatcher routes to typed handlers (policies, managed clusters, addons, etc.).
4. Handlers upsert into `local_status.*`, `status.*`, and `event.*` tables.

### Agent spec syncers

1. Consume `gh-spec` topic.
2. Decode bundles and apply/create/update/delete resources on the managed hub.
3. RBAC module enforces least-privilege access.

### Agent status syncers

1. Watch managed-hub resources via controller-runtime.
2. Filter duplicate events.
3. Emit CloudEvents to the hub's status topic.

---

## Namespace Layout

| Namespace | Cluster | Contents |
|---|---|---|
| `multicluster-global-hub` | Global hub | Operator, manager, Postgres, Kafka, Grafana |
| `multicluster-global-hub-agent` | Managed hub | Agent deployment |
| `open-cluster-management` | Both | ACM hub components |
| `open-cluster-management-backup` | ACM hub | Backup operator resources (separate component) |

---

## Active/Passive and Migration

Global Hub supports hub-level migration via `ManagedClusterMigration` CR and integrates with ACM backup/restore for DR scenarios. For backup operator specifics (Velero schedules, passive vs activation data), see [cluster-backup-operator docs](https://github.com/stolostron/cluster-backup-operator/blob/main/docs/ARCHITECTURE.md).

Global Hub backup controllers live in `operator/pkg/controllers/backup` (operator-side) and `manager/pkg/controllers/backup_controller.go` (manager-side).

---

## Import Boundaries

Components must not cross-import except through `pkg/` and `operator/api/`:

- `pkg/` — shared transport, database, bundle utilities
- `operator/api/` — CRD types readable by manager and agent
- `manager/`, `agent/`, `operator/` — isolated; communicate via Kafka and Kubernetes APIs only

---

## Dependencies

| Dependency | Version | Purpose |
|---|---|---|
| `sigs.k8s.io/controller-runtime` | v0.23.3 | Operator and controller framework |
| `github.com/IBM/sarama` | v1.46.x | Kafka (Sarama transport) |
| `github.com/confluentinc/confluent-kafka-go/v2` | v2.14.x | Kafka (Confluent transport) |
| `github.com/cloudevents/sdk-go/v2` | v2.16.x | Status bundle transport |
| `gorm.io/gorm` / `github.com/jackc/pgx/v5` | v1.31.1 / v5.9.2 | PostgreSQL |
| `github.com/stolostron/...` (ACM/OCM) | various | Policy, placement, subscription APIs |
| Go | 1.26.3 | Toolchain |
