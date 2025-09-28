# Event Send Modes Guide

## Overview

Global Hub supports two event sending modes for policy events:

- **Batch Mode** (default): Groups events together for efficient processing
- **Single Mode**: Sends each event individually for real-time processing

## Configuration

### Enable Single Mode

Add this annotation to your MulticlusterGlobalHub resource:

```yaml
apiVersion: operator.open-cluster-management.io/v1alpha4
kind: MulticlusterGlobalHub
metadata:
  name: multiclusterglobalhub
  namespace: multicluster-global-hub
  annotations:
    global-hub.open-cluster-management.io/event-send-mode: "single"
```

### Enable Batch Mode (Default)

```yaml
annotations:
  global-hub.open-cluster-management.io/event-send-mode: "batch"
```

Or simply remove the annotation (batch is default).

## CloudEvent Examples

### Batch Mode Example
Multiple events bundled in one CloudEvent (payload is an array):

```json
{
  "specversion": "1.0",
  "type": "io.open-cluster-management.operator.multiclusterglobalhubs.event.localreplicatedpolicy",
  "source": "hub1",
  "id": "35803b94-0118-4a81-b29f-04f167989ebb",
  "time": "2025-09-22T09:58:27.501078954Z",
  "datacontenttype": "application/json",
  "extensions": {
    "sendmode": "batch"
  },
  "data": [
    {
      "eventName": "open-cluster-management-global-set.test-policy.18664febe9bcb53c",
      "eventNamespace": "cluster1",
      "message": "Compliant; notification - namespaces [default] found as specified",
      "reason": "PolicyStatusSync",
      "policyId": "1eed206b-3b6a-4fde-9814-8ebe0a8e6ce1",
      "clusterId": "f642f37e-6788-4ce7-b7e7-76e689079df7",
      "clusterName": "cluster1",
      "compliance": "Compliant"
    },
    {
      "eventName": "open-cluster-management-global-set.test1.18664febe3baec3c",
      "eventNamespace": "cluster1",
      "message": "NonCompliant; violation - namespaces [default1] not found",
      "reason": "PolicyStatusSync",
      "policyId": "743467c9-b9ef-44f1-8239-efe7108d7dde",
      "clusterId": "f642f37e-6788-4ce7-b7e7-76e689079df7",
      "clusterName": "cluster1",
      "compliance": "NonCompliant"
    }
  ]
}
```

### Single Mode Example
Each event sent separately (payload is a single object):

```json
{
  "specversion": "1.0",
  "type": "io.open-cluster-management.operator.multiclusterglobalhubs.event.localreplicatedpolicy",
  "source": "hub1",
  "id": "single-event-12345",
  "time": "2025-09-22T09:58:27.501078954Z",
  "datacontenttype": "application/json",
  "extensions": {
    "sendmode": "single"
  },
  "data": {
    "eventName": "open-cluster-management-global-set.test-policy.18664febe9bcb53c",
    "eventNamespace": "cluster1",
    "message": "Compliant; notification - namespaces [default] found as specified",
    "reason": "PolicyStatusSync",
    "policyId": "1eed206b-3b6a-4fde-9814-8ebe0a8e6ce1",
    "clusterId": "f642f37e-6788-4ce7-b7e7-76e689079df7",
    "clusterName": "cluster1",
    "compliance": "Compliant"
  }
}
```

### Key Difference
- **Batch Mode**: `data` field contains an **array** `[...]` of events
- **Single Mode**: `data` field contains a **single object** `{...}` per event

## When to Use Each Mode

### Use Single Mode When:
- You need real-time policy compliance updates
- Debugging policy issues
- Testing environments
- Low policy volume (< 50 policies)
- **Integrating with systems designed for individual event processing** (e.g., Ansible, event-driven automation tools)
- **Working with legacy systems that cannot handle batch/array data structures**
- **Building event-driven workflows that require atomic event processing**

### Use Batch Mode When:
- Production environments
- High policy volume (> 100 policies)
- Network bandwidth is limited
- Processing efficiency is more important than latency

## Verification

Check current mode:
```bash
kubectl get multiclusterglobalhub multiclusterglobalhub \
  -n multicluster-global-hub \
  -o jsonpath='{.metadata.annotations.global-hub\.open-cluster-management\.io/event-send-mode}'
```

After changing the mode, verify the agent deployment was updated:

```bash
kubectl get deployment multicluster-global-hub-agent \
  -n multicluster-global-hub-agent \
  -o jsonpath='{.spec.template.spec.containers[0].args}' | grep event-mode
```

You should see: `--event-send-mode=single` or `--event-send-mode=batch`

## Important Notes

- Changes require agent pod restart (happens automatically)
- Both modes can run simultaneously in mixed environments
- No data loss occurs when switching modes
- Batch mode is more efficient for high-volume scenarios
- Single mode provides better real-time responsiveness