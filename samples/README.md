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

Consume message from `status` topic with kafka client. Here we start a background program to submit the offset to brokers periodically, and each time a message is consumed successfully we just mark the message with `sess.MarkMessage(msg, "")`
```
$ go test ./samples/consumer/kafkas_test.go -v
=== RUN   TestKafkaConsumer
>>> commit offset
>>> mark offset
    kafkas_test.go:51: consumed msg 0 - 27: {"destination":"","key":"kind-hub1.PolicyCompleteCompliance","id":"kind-hub1.PolicyCompleteCompliance","msgType":"StatusBundle","version":"0.9","payload":"eyJvYmplY3RzIjpbeyJwb2xpY3lJZCI6Ijg2OGFkN2Q4LTRkMGUtNDEzOC04ZjFiLWEzYTQxOTJkZDFmZSIsIm5vbkNvbXBsaWFudENsdXN0ZXJzIjpbImtpbmQtaHViMS1jbHVzdGVyMSIsImtpbmQtaHViMS1jbHVzdGVyMiJdLCJ1bmtub3duQ29tcGxpYW5jZUNsdXN0ZXJzIjpbXX1dLCJsZWFmSHViTmFtZSI6ImtpbmQtaHViMSIsImJhc2VCdW5kbGVWZXJzaW9uIjp7ImluY2FybmF0aW9uIjowLCJnZW5lcmF0aW9uIjo4fSwiYnVuZGxlVmVyc2lvbiI6eyJpbmNhcm5hdGlvbiI6MCwiZ2VuZXJhdGlvbiI6OX19"}
>>> last commit offset
--- PASS: TestKafkaConsumer (3.64s)
PASS
ok      command-line-arguments  3.654s
```
