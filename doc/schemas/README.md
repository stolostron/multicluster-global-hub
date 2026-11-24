# Forwarded event schema draft

This is a proposed schema for [ACM-12506](https://redhat.atlassian.net/browse/ACM-12506), derived from what the current Global Hub agent serializes and the examples in [events-api.md](../events-api.md). It has not been approved as a customer contract. The four `event.*` CloudEvent types are covered by [forwarded-events.schema.json](forwarded-events.schema.json). State synchronization types are outside this draft's scope.

The schema validates a **structured CloudEvent representation**. The current Kafka producer uses CloudEvents binary mode: CloudEvent attributes are in Kafka headers and the message value is the `data` payload. Large payloads can be split across transport messages, so consumers must reassemble those chunks before validation. A consumer can reconstruct the event with a CloudEvents SDK and validate the result. Alternatively, it can validate a single-mode message value, or each item of a batch-mode value, against the corresponding payload definition under `$defs`.

The `type` attribute selects one of four proposed payload shapes. `sendmode: single` carries one object in `data`; `sendmode: batch` carries a nonempty array of the same object type. Fields without `omitempty` in the Go payload types appear in the current serialized output, so this draft marks them as required. That observation does not establish a support guarantee, and a required field may still contain an empty string. Additional properties are allowed to accommodate additive fields; the compatibility policy also needs stakeholder approval.

| CloudEvent `type` suffix | Payload definition | Producer source |
| --- | --- | --- |
| `event.localrootpolicy` | `rootPolicyPayload` | `agent/pkg/status/syncers/events/local_root_policy_event_emitter.go` |
| `event.localreplicatedpolicy` | `replicatedPolicyPayload` | `agent/pkg/status/syncers/policies/replicated_policy_event_emitter.go` |
| `event.managedcluster` | `managedClusterPayload` | `agent/pkg/status/syncers/events/managedcluster_event_emitter.go` |
| `event.clustergroupupgrade` | `clusterGroupUpgradePayload` | `agent/pkg/status/syncers/events/clustergroupupgrade_event_emitter.go` |

Before publishing this as a supported schema, confirm with the ACM and Management Fabric stakeholders:

1. Which fields must be guaranteed nonempty, especially the Kubernetes reason, message, and reporting fields.
2. Whether Fabric events need the same payload fields and CloudEvent attributes, and how differences are versioned.
3. Whether consumers need a JSON representation of the full CloudEvent or only a schema for the Kafka message value.
4. Which observed fields and compatibility rules ACM will guarantee across releases.

The current guide, [events-api.md](../events-api.md), describes examples and availability. It should link to the approved schema after stakeholder review.
