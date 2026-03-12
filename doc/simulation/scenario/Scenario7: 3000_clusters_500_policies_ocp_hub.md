# Scenario 7: Manage 3,000 clusters and 500 policies on OCP Hub (main)

## Note

- Kafka in KRaft mode (same as Scenario 6).
- Commit Image: `e18d0320` (fix: upgrade Go version from 1.25.3 to 1.25.7)
- Date: 2026-03-04
- Global Hub cluster: OCP SNO (`obs-hub-of-hubs-aws-418-sno-c82l2`)
- Regional Hub (hub1): OCP SNO (`obs-hub-of-hubs-aws-418-sno-tsphv`) — real ACM hub, already imported
- Managed Hub Clusters (hub2–hub10): 9 KinD clusters running on a VM

## Scale

- 1 Real OCP Managed Hub (hub1) + 9 KinD Managed Hubs (hub2–hub10)
- 3,002 Managed Clusters total (300 mock clusters per hub + hub1's real `cluster1`)
- 500 Root Policies (50 per hub × 10 hubs), 135,000 Replicated Policies (hub2–hub10 only)

> **Note**: hub1 is a real OCP cluster with ACM/GRC running. The GRC policy propagator on hub1
> continuously reconciles and removes directly-created mock replicated policies. As a result,
> only hub2–hub10 (KinD, no GRC) contribute compliance data (9 × 15,000 = 135,000 replicated policies).
> hub1 contributes 300 mock managed clusters and 50 root policies to the global hub.

## Simulation Workflow

1. **Global Hub** already installed on OCP SNO with Kafka (KRaft), Manager, Grafana, Operator, PostgreSQL.

2. **hub1** (real OCP) already imported as a managed hub. Mock managed clusters 1–300 created using `mock-hub1-clusters.sh`. Cluster lease keep-alive required due to real MCE overwriting availability status.

3. **hub2–hub10** (KinD) created via `setup-cluster.sh 2:10 1:300`:
   - KinD clusters created and imported to global hub via MCE import secrets
   - OCP pull secrets propagated to KinD clusters for registry.redhat.io access
   - Global hub agent deployed via `global-hub.open-cluster-management.io/deploy-mode=default` label
   - 300 mock managed clusters created per hub with `setup-cluster.sh`

4. **DB counter started** at `2026-03-04 07:59 UTC` (02:59 local)

5. **50 policies created on all 10 hubs** via `setup-policy.sh 1:10 1:50` — initial state: NonCompliant

6. **All 135,000 replicated policies synced** to global hub (~35 minutes)

7. **Round 1 rotation**: NonCompliant → Compliant (`rotate-policy.sh 2:10 1:50 Compliant`)
   - Completed in ~2 minutes; DB sync took ~23 minutes

8. **Round 2 rotation**: Compliant → NonCompliant (`rotate-policy.sh 2:10 1:50 NonCompliant`)
   - Completed in ~2 minutes; DB sync took ~24 minutes

9. **Prometheus metrics collected** for window `2026-03-04 02:45 – 05:00 local` (UTC 07:45 – 10:00)

## Statistics and Analysis

### Database Count over Time

The global hub counters track managed clusters, compliance records, and policy events.

- **Cluster and Compliance Initialization**

  ![Cluster and Compliance Count](./images/7-count-initialization.png)

- **Compliance Status Changes (Compliant vs NonCompliant)**

  ![Compliance Changes](./images/7-count-compliance.png)

- **Policy Event Count**

  ![Policy Event Count](./images/7-count-event.png)

### Total Global Hub CPU and Memory

- **Total Global Hub Namespace CPU**

  ![Total CPU](./images/7-global-hub-total-cpu-usage.png)

- **Total Global Hub Namespace Memory**

  ![Total Memory](./images/7-global-hub-total-memory-usage.png)

### CPU and Memory per Global Hub Component

- **Multicluster Global Hub Manager**

  ![Manager CPU](./images/7-global-hub-manager-cpu-usage.png)
  ![Manager Memory](./images/7-global-hub-manager-memory-usage.png)

- **Multicluster Global Hub Grafana**

  ![Grafana CPU](./images/7-global-hub-grafana-cpu-usage.png)
  ![Grafana Memory](./images/7-global-hub-grafana-memory-usage.png)

- **Multicluster Global Hub Operator**

  ![Operator CPU](./images/7-global-hub-operator-cpu-usage.png)
  ![Operator Memory](./images/7-global-hub-operator-memory-usage.png)

- **Multicluster Global Hub Agent** (on hub1, real OCP hub)

  ![Agent CPU](./images/7-global-hub-agent-cpu-usage.png)
  ![Agent Memory](./images/7-global-hub-agent-memory-usage.png)

### Middleware CPU and Memory

- **PostgreSQL**

  ![PostgreSQL CPU](./images/7-global-hub-postgresql-cpu-usage.png)
  ![PostgreSQL Memory](./images/7-global-hub-postgresql-memory-usage.png)
  ![PostgreSQL PVC](./images/7-global-hub-postgresql-pvc-usage.png)

- **Kafka (KRaft — 3 brokers)**

  ![Kafka Broker CPU](./images/7-global-hub-kafka-broker-cpu-usage.png)
  ![Kafka Broker Memory](./images/7-global-hub-kafka-broker-memory-usage.png)
  ![Kafka PVC](./images/7-global-hub-kafka-pvc-usage.png)

  ![Kafka Operator CPU](./images/7-global-hub-kafka-operator-cpu-usage.png)
  ![Kafka Operator Memory](./images/7-global-hub-kafka-operator-memory-usage.png)

## CPU and Memory Summary

### 3,000 Clusters (10 hubs, 300 clusters each, 50 policies/hub)

- Overall namespace memory

  | Metric | Value |
  |:-------|:------|
  | Total Pods in Namespace | 12 pods (including 3 Kafka broker replicas) |
  | Total Memory — Average | 5.41 GB |
  | Total Memory — Peak | 5.88 GB |
  | Total Memory — Baseline (before test) | 4.91 GB |

- Memory usage per component

  | Component | Pods | Current Memory | Avg Memory (during test) | Peak Memory (during test) |
  |:----------|:----:|:--------------:|:------------------------:|:-------------------------:|
  | Manager | 2 | 159 Mi (22 + 137) | 187.5 Mi | 236.2 Mi |
  | Grafana | 2 | 316 Mi (169 + 147) | 285.2 Mi | 547.8 Mi |
  | Operator | 1 | 93 Mi | 82.6 Mi | 90.8 Mi |
  | PostgreSQL | 1 | 311 Mi | 212.8 Mi | 275.0 Mi |
  | Kafka Broker | 3 | 4,290 Mi (1400 + 1433 + 1457) | 4,100 Mi | 4,264 Mi |
  | Kafka Entity Operator | 1 | 618 Mi | — | — |
  | Strimzi Cluster Operator | 1 | 757 Mi | — | — |
  | Kafka Operator (total) | 2 | 1,375 Mi | 1,204 Mi | 1,382 Mi |
  | Agent (hub1, separate ns) | 1 | 219 Mi | 213 Mi | 314.5 Mi |

- Maximum CPU usage per component

  | Type | Manager | Agent | Operator | Grafana | PostgreSQL | Kafka Broker |
  |:-----|:-------:|:-----:|:--------:|:-------:|:----------:|:------------:|
  | Maximum CPU (m) | 105 | 63 | 27 | 31 | 9,068 * | 211 |

  > \* **PostgreSQL CPU spike**: Up to 9 cores during peak data ingestion (initial sync of 135,000
  > compliance records and ~270,000 events). Typical steady-state CPU is < 20 m.

- Recommended CPU and memory resource settings

  | Type               | Manager | Agent | Operator | Grafana | PostgreSQL | Kafka Broker |
  |:---                |:---     |:---   |:---      |:---     |:---        |:---          |
  | Request CPU (m)    | 5       | 10    | 2        | 5       | 100        | 20           |
  | Limit CPU (m)      | 200     | 100   | 60       | 60      | 16,000     | 400          |
  | Request Memory (Mi)| 60      | 300   | 70       | 150     | 60         | 2 Gi         |
  | Limit Memory (Mi)  | 512     | 512   | 256      | 800     | 512        | 6 Gi         |

### Observations

1. **PostgreSQL** is the dominant CPU consumer during large-scale initial data sync, peaking at ~9 cores
   when ingesting 135,000 compliance records and 270,000+ events simultaneously. Steady-state usage
   is well under 100 m. The PostgreSQL CPU limit should account for this burst.

2. **Kafka brokers** (3 × KRaft) consume ~1.4 GB each at peak — consistent with Scenario 6.
   JVM heap tuning (`-Xms 1G -Xmx 1G`) can reduce this to ~1.6 GB per broker if needed.

3. **Manager** memory peaked at ~237 Mi with 2 replicas processing 3,002 clusters and 135,000
   compliance events. Scales linearly with cluster+policy count.

4. **Agent** (hub1 real OCP) peaked at ~315 Mi while managing 300 mock clusters and syncing
   data for 50 root policies. Typical agent memory for 300 clusters is 200–315 Mi.

5. **Grafana** memory peaked at ~548 Mi (2 replicas, ~274 Mi each) which is higher than expected
   — may reflect caching of dashboards during active metric collection.

6. **Real OCP hub limitation**: The GRC policy propagator on hub1 removes directly-created mock
   replicated policies. Only 9 KinD hubs (hub2–hub10) contribute compliance data (135,000 of the
   planned 150,000 replicated policies). For future tests on real OCP hubs, consider suspending
   the GRC propagator or using proper PlacementBinding-based policy creation.
