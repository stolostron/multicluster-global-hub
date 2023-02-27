# Getting Started

We have several examples for user to understand how global hub works and how to develop on global hub.

- **consumer**: create a separate consumer which can receive message from the regional hubs.

## Prerequisites

These instructions assume:

- You have a global hub environment whether created locally(`make e2e-setup-start`) or on OCP;

## Consumer 

Set environment variables.
```bash
export KUBECONFIG=</path/to/global_hub_cluster/kubeconfig>
export SECRET_NAMESPACE=<transport-secret-namespace> # default SECRET_NAMESPACE=open-cluster-management
export SECRET_NAME=<transport-secret-name> # default SECRET_NAME=transport-secret
```

Consume message from `status` topic with cloudevents client.
```
$ go test ./samples/consumer/cloudevents_test.go -v
=== RUN   TestCloudeventsConsumer
policy.policy.open-cluster-management.io/policy-limitrange created
placementbinding.policy.open-cluster-management.io/binding-policy-limitrange created
placementrule.apps.open-cluster-management.io/placement-policy-limitrange created

=====================
Context Attributes,
  specversion: 1.0
  type: StatusBundle
  source: global-hub-manager
  id: kind-hub1.PlacementRule
  time: 2023-02-27T08:47:20.511191625Z
  datacontenttype: application/json
Extensions,
  offset: 0
  size: 255
Data,
  {
    "destination": "",
    "key": "kind-hub1.PlacementRule",
    "id": "kind-hub1.PlacementRule",
    "msgType": "StatusBundle",
    "version": "0.4",
    "payload": "eyJvYmplY3RzIjpbXSwibGVhZkh1Yk5hbWUiOiJraW5kLWh1YjEiLCJidW5kbGVWZXJzaW9uIjp7ImluY2FybmF0aW9uIjowLCJnZW5lcmF0aW9uIjo0fX0="
  }
...
```

Consume message from `status` topic with kafka client.
```
$ go test ./samples/consumer/kafkas_test.go -v
=== RUN   TestKafkaConsumer
    kafkas_test.go:24: consumed msg 0 - 3: {"destination":"","key":"kind-hub1.ControlInfo","id":"kind-hub1.ControlInfo","msgType":"StatusBundle","version":"0.3","payload":"eyJsZWFmSHViTmFtZSI6ImtpbmQtaHViMSIsImJ1bmRsZVZlcnNpb24iOnsiaW5jYXJuYXRpb24iOjAsImdlbmVyYXRpb24iOjN9fQ=="}
--- PASS: TestKafkaConsumer (3.30s)
PASS
ok      command-line-arguments  3.315s
```
