# Global Hub CloudEvent API

The multicluster global hub agent publishes status and event data from a managed hub to the global hub manager as [CloudEvents](https://cloudevents.io/). Consumers should use the CloudEvent `type` attribute to select the payloads they process.

This document describes the CloudEvents that the current agent publishes on the status topic. It does not describe spec synchronization or managed-cluster migration messages, which use different topics and workflows. The examples show representative payloads; they are not JSON schemas.

## Deployment modes

The same status path is used in both deployment modes:

- **Global hub mode:** The managed hub is imported into the global hub, using either default or hosted import mode. In both import modes, the agent publishes status and events toward the global hub and synchronizes specifications toward the managed hub. The import mode does not change the CloudEvent types or payloads.
- **Standalone mode:** The managed hub is not imported into the global hub. The global hub agent runs on that hub as a status and event exporter; specification synchronization is not started.

Default and hosted are import modes for a managed hub in a Global Hub installation. Standalone is a separate agent deployment mode.

Availability still depends on the resources and integrations installed on the managed hub. For example, `event.clustergroupupgrade` requires Topology Aware Lifecycle Manager (TALM), and `security.alertcounts` requires the Red Hat Advanced Cluster Security (RHACS) integration.

## Status topic

All CloudEvent types in this document are sent from the agent to the manager on the configured Kafka status topic.
There is no separate topic for the `event.*` types.

- With the built-in Kafka configuration, the default topic pattern is `gh-status.*`; the agent replaces `*` with its managed hub name, producing a topic such as `gh-status.hub1`.
- With bring-your-own Kafka, the default is the shared topic `gh-status`. A custom status topic can also be configured.
- In standalone deployments, the topic is read from the agent transport configuration.

## CloudEvent type reference

Every type below flows from the managed-hub agent to the global hub manager on the status topic. A forwarded `event.*` payload is one object in single mode or an array of objects in batch mode.

| CloudEvent `type` | `data` shape | Availability |
| --- | --- | --- |
| `io.open-cluster-management.operator.multiclusterglobalhubs.managedcluster` | Generic object bundle of `ManagedCluster` resources | Core status path; managed-cluster APIs required |
| `io.open-cluster-management.operator.multiclusterglobalhubs.policy.localspec` | Generic object bundle of root `Policy` specifications, with status removed | Local policy collection enabled |
| `io.open-cluster-management.operator.multiclusterglobalhubs.policy.localcompliance` | Array of root-policy records listing clusters in all four compliance states | Local policy collection and full aggregation enabled |
| `io.open-cluster-management.operator.multiclusterglobalhubs.policy.localcompletecompliance` | Array of root-policy records listing only noncompliant, unknown, and pending clusters | Local policy collection and full aggregation enabled |
| `io.open-cluster-management.operator.multiclusterglobalhubs.managedhub.info` | Managed hub information object | Core status path; hub APIs required |
| `io.open-cluster-management.operator.multiclusterglobalhubs.managedhub.heartbeat` | Empty array | Core status path; hub APIs required |
| `io.open-cluster-management.operator.multiclusterglobalhubs.security.alertcounts` | Security alert-count object | RHACS integration, Central CRD, and an annotated Central CR required |
| `io.open-cluster-management.operator.multiclusterglobalhubs.event.localrootpolicy` | Kubernetes event details for a root `Policy`, including policy ID and compliance | Local policy collection enabled |
| `io.open-cluster-management.operator.multiclusterglobalhubs.event.localreplicatedpolicy` | Replicated `Policy` status-history entry, including policy and cluster IDs and compliance | Local policy collection enabled |
| `io.open-cluster-management.operator.multiclusterglobalhubs.event.managedcluster` | Managed-cluster or qualifying provisioning-Job event details | Core status path; qualifying Kubernetes event required |
| `io.open-cluster-management.operator.multiclusterglobalhubs.event.clustergroupupgrade` | `ClusterGroupUpgrade` event details | TALM installed and emitting Kubernetes events |

The shorter names used in the remaining headings are the suffixes of these full type values.

### Local policy collection and full aggregation

The five policy types in the table describe different information: `policy.localspec` contains root `Policy` specifications; `policy.localcompliance` lists every cluster and its compliance category for each root policy; `policy.localcompletecompliance` sends only the noncompliant, unknown, and pending exceptions. `event.localrootpolicy` forwards Kubernetes events involving root Policies, while `event.localreplicatedpolicy` derives events from replicated Policy status history for individual managed clusters.

The agent ConfigMap setting `enableLocalPolicies: "true"` enables these local policy types. The two compliance types additionally require `aggregationLevel: full`, which makes the agent aggregate the compliant, noncompliant, unknown, and pending cluster lists for each root policy. With `aggregationLevel: minimal`, those two local compliance types are not published. The operator-generated agent ConfigMaps and the standalone event-exporter template in this repository set local policies to `true` and aggregation to `full`. These settings are independent of default or hosted hub import mode; matching Policy resources must also exist for policy data to be published.

## Common CloudEvent envelope

Every message is a CloudEvents 1.0 event with a JSON data payload. The agent sets:

- `source` to the managed hub name.
- `type` to one of the exact values in the type reference.
- `extversion` to a version associated with the emitted data. It is not a unique event sequence: in single mode, multiple `event.*` messages can share a version, and the version resets when the agent restarts.
- `datacontenttype` to `application/json`.

The CloudEvents SDK supplies attributes such as `specversion`, `id`, and `time`. By default, the agent uses binary mode. Each attribute is a Kafka header, with the `ce_` prefix and `datacontenttype` mapped to `content-type`. The Kafka message value contains only the event data. Consumers that need the attributes must read the headers or decode the message with a CloudEvents SDK. The following JSON shows the full event envelope as a structured-mode representation:

```json
{
  "specversion": "1.0",
  "id": "2ed16618-88b0-4a9d-a484-fb87a18a17f2",
  "source": "hub1",
  "type": "io.open-cluster-management.operator.multiclusterglobalhubs.managedhub.info",
  "datacontenttype": "application/json",
  "time": "2026-10-09T08:30:00Z",
  "extversion": "12.0",
  "data": {
    "consoleURL": "https://console-openshift-console.apps.hub1.example.com",
    "grafanaURL": "https://grafana-multicluster-global-hub.apps.hub1.example.com",
    "mchVersion": "2.15.0",
    "clusterId": "4d3d7f30-2f67-4aa2-91e7-93ae6d75fc6c"
  }
}
```

To keep the remaining examples focused on each payload contract, SDK-generated `id` and `time` attributes are omitted.

`policy.localcompletecompliance` also includes `extdependencyversion`. The manager compares its value component with the value component of the last processed `policy.localcompliance` version; the generation components do not need to match.

### Event send modes

Only the four `event.*` CloudEvent types include the `sendmode` extension. State synchronization types do not.

- `"sendmode": "batch"` is the default. `data` is an array of event objects.
- `"sendmode": "single"` sends one CloudEvent per event object. `data` is one object.

In a Global Hub installation, set this annotation on `MulticlusterGlobalHub`. For a standalone agent managed by `MulticlusterGlobalHubAgent`, set it on that resource instead:

```yaml
metadata:
  annotations:
    global-hub.open-cluster-management.io/event-send-mode: single
```

The agent command-line flag is `--event-send-mode`; it also defaults to `batch`.

## State synchronization types

State synchronization CloudEvents report current resource or aggregate state. They do not include `sendmode`.

### `managedcluster`

Reports `ManagedCluster` resources. Its `data` is a generic object bundle with any applicable operation fields:

- `create`, `update`, and `resync` contain Kubernetes resources.
- `delete` and `resync_metadata` contain object identity metadata.

For a nonempty periodic resync, the agent can send multiple `resync` messages with the same `extversion`, followed by a `resync_metadata` message listing the identities of all objects in that resync. An individual `resync` message may contain only part of the snapshot; consumers must use the final inventory to reconcile objects that are no longer present. If the periodic list is empty, the agent sends no inventory.

Fields with no entries are omitted. For example:

```json
{
  "specversion": "1.0",
  "source": "hub1",
  "type": "io.open-cluster-management.operator.multiclusterglobalhubs.managedcluster",
  "datacontenttype": "application/json",
  "extversion": "25.0",
  "data": {
    "update": [
      {
        "apiVersion": "cluster.open-cluster-management.io/v1",
        "kind": "ManagedCluster",
        "metadata": {
          "name": "cluster1",
          "uid": "468c2a75-b24c-4c9a-b504-3d449bcf1a11"
        },
        "spec": {
          "hubAcceptsClient": true
        }
      }
    ],
    "delete": [
      {
        "id": "bbf81567-7f2f-4dc0-8976-29cb79e6d621",
        "name": "cluster2"
      }
    ]
  }
}
```

### `policy.localspec`

Reports root `Policy` specifications created on the managed hub when local policy collection is enabled. The agent removes Policy status from this payload; replicated policies are not included. Its `data` uses the same generic object-bundle operations and resync inventory sequence as `managedcluster`:

```json
{
  "specversion": "1.0",
  "source": "hub1",
  "type": "io.open-cluster-management.operator.multiclusterglobalhubs.policy.localspec",
  "datacontenttype": "application/json",
  "extversion": "7.0",
  "data": {
    "update": [
      {
        "apiVersion": "policy.open-cluster-management.io/v1",
        "kind": "Policy",
        "metadata": {
          "name": "require-labels",
          "namespace": "local-policies",
          "uid": "d42aba68-3266-4516-9056-d797a5f2cc6f"
        },
        "spec": {
          "disabled": false
        }
      }
    ]
  }
}
```

### `policy.localcompliance`

Reports the full cluster membership and compliance categories for each root policy. It contains `compliantClusters`, `nonCompliantClusters`, `unknownComplianceClusters`, and `pendingComplianceClusters`. The agent publishes this full list when a policy is added or removed or its cluster membership changes; a compliance-only change is carried by `policy.localcompletecompliance`. Both local policy collection and full aggregation must be enabled.

```json
{
  "specversion": "1.0",
  "source": "hub1",
  "type": "io.open-cluster-management.operator.multiclusterglobalhubs.policy.localcompliance",
  "datacontenttype": "application/json",
  "extversion": "31.0",
  "data": [
    {
      "policyId": "d42aba68-3266-4516-9056-d797a5f2cc6f",
      "compliantClusters": ["cluster1"],
      "nonCompliantClusters": ["cluster2"],
      "unknownComplianceClusters": [],
      "pendingComplianceClusters": []
    }
  ]
}
```

### `policy.localcompletecompliance`

Reports the exception lists for each root policy as compliance changes. It omits `compliantClusters`; among the clusters known for that policy from `policy.localcompliance`, those absent from the exception lists are compliant. Policies with no exceptions are omitted from this bundle. The `extdependencyversion` value ties this compact representation to the corresponding `policy.localcompliance` stream version.

```json
{
  "specversion": "1.0",
  "source": "hub1",
  "type": "io.open-cluster-management.operator.multiclusterglobalhubs.policy.localcompletecompliance",
  "datacontenttype": "application/json",
  "extversion": "31.0",
  "extdependencyversion": "31.0",
  "data": [
    {
      "policyId": "d42aba68-3266-4516-9056-d797a5f2cc6f",
      "nonCompliantClusters": ["cluster2"],
      "unknownComplianceClusters": [],
      "pendingComplianceClusters": []
    }
  ]
}
```

### `managedhub.info`

Reports identifying and UI information for the managed hub. See the [common envelope example](#common-cloudevent-envelope) for a complete message. The `data` fields are:

| Field | Description |
| --- | --- |
| `consoleURL` | OpenShift console URL for the managed hub |
| `grafanaURL` | Grafana URL associated with the managed hub |
| `mchVersion` | MultiClusterHub version |
| `clusterId` | Managed hub cluster identifier |

### `managedhub.heartbeat`

Signals that the managed hub agent is alive. The payload carries no resource state:

```json
{
  "specversion": "1.0",
  "source": "hub1",
  "type": "io.open-cluster-management.operator.multiclusterglobalhubs.managedhub.heartbeat",
  "datacontenttype": "application/json",
  "extversion": "85.0",
  "data": []
}
```

### `security.alertcounts`

Reports summarized RHACS alert counts. The agent starts this publisher when the RHACS integration is enabled and the `centrals.platform.stackrox.io` CRD is detected. It publishes counts only for Central CRs with the `global-hub.open-cluster-management.io/with-stackrox-credentials-secret` annotation.

```json
{
  "specversion": "1.0",
  "source": "hub1",
  "type": "io.open-cluster-management.operator.multiclusterglobalhubs.security.alertcounts",
  "datacontenttype": "application/json",
  "extversion": "4.0",
  "data": {
    "low": 12,
    "medium": 8,
    "high": 3,
    "critical": 1,
    "detail_url": "https://central-rhacs-operator.apps.hub1.example.com/main/violations",
    "source": "stackrox/central"
  }
}
```

## Forwarded Kubernetes event types

The `event.*` types carry discrete operational events and support the shared `sendmode` behavior. The outer CloudEvent `type` selects the payload contract. Where a payload contains its own `type` field, that inner field is the Kubernetes event classification, normally `Normal` or `Warning`.

The agent filters these streams by event time to avoid forwarding the same event repeatedly. Except where noted, it does not restrict events to a fixed list of Kubernetes event reasons.

### `event.localrootpolicy`

Forwards core Kubernetes `Event` objects whose involved object is a root `Policy` on the managed hub. It is available when local policy collection is enabled. Each payload contains the Kubernetes event details, the root policy UID, and the compliance value derived from the policy or event message.

```json
{
  "specversion": "1.0",
  "source": "hub1",
  "type": "io.open-cluster-management.operator.multiclusterglobalhubs.event.localrootpolicy",
  "datacontenttype": "application/json",
  "extversion": "18.0",
  "sendmode": "single",
  "data": {
    "eventName": "require-labels.18bc3d2ad73c45e1",
    "eventNamespace": "local-policies",
    "message": "Policy require-labels status was updated to NonCompliant",
    "reason": "PolicyStatusSync",
    "count": 1,
    "source": {
      "component": "governance-policy-status-sync"
    },
    "createdAt": "2026-10-09T08:35:00Z",
    "policyId": "d42aba68-3266-4516-9056-d797a5f2cc6f",
    "compliance": "NonCompliant"
  }
}
```

The managed hub identity is in the outer CloudEvent `source`; it is not repeated as `leafHubName` in this payload.

### `event.localreplicatedpolicy`

Forwards status-history entries from replicated policies on managed clusters. Unlike `event.localrootpolicy`, these records come from the replicated `Policy` status rather than directly from core Kubernetes `Event` objects. Each includes the root policy UID, cluster ID and name, and compliance value. The agent sets `reason` to `PolicyStatusSync` and `source.component` to `policy-status-history-sync`.

```json
{
  "specversion": "1.0",
  "source": "hub1",
  "type": "io.open-cluster-management.operator.multiclusterglobalhubs.event.localreplicatedpolicy",
  "datacontenttype": "application/json",
  "extversion": "42.0",
  "sendmode": "batch",
  "data": [
    {
      "eventName": "require-labels.18bc3d2ad73c45e1",
      "eventNamespace": "cluster1",
      "message": "NonCompliant; violation - required label is missing",
      "reason": "PolicyStatusSync",
      "count": 1,
      "source": {
        "component": "policy-status-history-sync"
      },
      "createdAt": "2026-10-09T08:36:00Z",
      "policyId": "d42aba68-3266-4516-9056-d797a5f2cc6f",
      "clusterId": "468c2a75-b24c-4c9a-b504-3d449bcf1a11",
      "clusterName": "cluster1",
      "compliance": "NonCompliant"
    }
  ]
}
```

The managed hub identity is in the outer CloudEvent `source`; it is not repeated as `leafHubName` in this payload.

### `event.managedcluster`

Some provisioning controllers attach Kubernetes events to a `Job` rather than directly to a `ManagedCluster`. The agent forwards both sources under `event.managedcluster` when the event's involved object is:

- A `ManagedCluster`.
- A provisioning `Job` named `<cluster-namespace>-imageset`, used by ClusterInstance bare-metal provisioning.
- A provisioning `Job` whose name starts with `<cluster-namespace>-` and ends with `-provision`, used by Cluster API cloud provisioning.

For a Job event, the event namespace identifies the managed cluster. The agent looks up that `ManagedCluster` to obtain its cluster ID; it does not forward the event if the cluster cannot be found or has no ID. It sets `clusterName` to the namespace and normalizes the message for `SuccessfulCreate` and `Completed` reasons. Other reasons are forwarded without a reason allowlist.

```json
{
  "specversion": "1.0",
  "source": "hub1",
  "type": "io.open-cluster-management.operator.multiclusterglobalhubs.event.managedcluster",
  "datacontenttype": "application/json",
  "extversion": "53.0",
  "sendmode": "single",
  "data": {
    "eventNamespace": "cluster1",
    "eventName": "cluster1-import.18bc3d2ad73c45e1",
    "clusterName": "cluster1",
    "clusterId": "468c2a75-b24c-4c9a-b504-3d449bcf1a11",
    "leafHubName": "hub1",
    "message": "Managed cluster cluster1 is available",
    "reason": "ManagedClusterAvailable",
    "reportingController": "managedcluster-import-controller",
    "reportingInstance": "managedcluster-import-controller-7b6cd8c56b-x9k2m",
    "type": "Normal",
    "createdAt": "2026-10-09T08:40:00Z"
  }
}
```

### `event.clustergroupupgrade`

Forwards Kubernetes events whose involved object is a `ClusterGroupUpgrade`. This resource is supplied by TALM, so the event type is available only when TALM is installed and produces those Kubernetes events. The agent does not apply a reason allowlist.

```json
{
  "specversion": "1.0",
  "source": "hub1",
  "type": "io.open-cluster-management.operator.multiclusterglobalhubs.event.clustergroupupgrade",
  "datacontenttype": "application/json",
  "extversion": "9.0",
  "sendmode": "single",
  "data": {
    "eventNamespace": "ztp-install",
    "eventName": "example-cgu.18bc3d2ad73c45e1",
    "eventAnnotations": {
      "cgu.openshift.io/event-type": "global",
      "cgu.openshift.io/total-clusters-count": "2"
    },
    "cguName": "example-cgu",
    "leafHubName": "hub1",
    "message": "ClusterGroupUpgrade example-cgu succeeded remediating policies",
    "reason": "CguSuccess",
    "reportingController": "cgu-controller",
    "reportingInstance": "cgu-controller-6b8d4fdf95-hq5nw",
    "type": "Normal",
    "createdAt": "2026-10-09T08:45:00Z"
  }
}
```
