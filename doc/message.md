This document is used to describe what kinds of message transfered via Kafka in multicluster global hub.
## Message Format
The message format is following the [CloudEvents](https://github.com/cloudevents/spec/blob/main/cloudevents/spec.md) specification. The message is encoded in JSON format. The following is an example of the message:
```json
{
    "specversion" : "1.0",
    "type" : "com.event",
    "source" : "source",
    "id" : "A234-1234-1234",
    "time" : "2018-04-05T17:31:00Z",
    "datacontenttype" : "application/json",
    "data" : {
        "message" : "Hello World!"
    }
}
```

## Topic: spec
This is for the global resources only. To propagate the resources from the global hub to the managed hubs.
Will list the supported resources later.

## Topic: status.$(managed_hub_cluster_name)
### Policy
#### LocalPolicySpec
```
{
    "specversion": "1.0",
    "id": "1c93bc16-bc17-11ee-9604-b7917fdadaab",
    "source": "managed_hub1",
    "type": "io.open-cluster-management.operator.multiclusterglobalhubs.policy.localspec",
    "datacontenttype": "application/json",
    "time": "2024-01-25T07:34:59.740351049Z",
    "data": {
        "objects": [
            {
                "kind": "Policy",
                "apiVersion": "policy.open-cluster-management.io/v1",
                "metadata": {
                    "name": "test-role-policy-1705906106",
                    "namespace": "kube-system",
                    "uid": "9b86154b-cc49-407e-b9f5-1112fb6a3e56",
                    "resourceVersion": "3364198",
                    "creationTimestamp": "2024-01-22T06:48:36Z",
                },
                "spec": {
                    "disabled": false,
                    "remediationAction": "inform",
                    "policy-templates": [
                        {
                            "objectDefinition": {
                                "apiVersion": "policy.open-cluster-management.io/v1",
                                "kind": "ConfigurationPolicy",
                                "metadata": {
                                    "name": "policy-role-1705906106"
                                },
                                "spec": {
                                    "namespaceSelector": {
                                        "exclude": [
                                            "kube-*"
                                        ],
                                        "include": [
                                            "default"
                                        ]
                                    },
                                    "object-templates": [
                                        {
                                            "complianceType": "musthave",
                                            "objectDefinition": {
                                                "apiVersion": "rbac.authorization.k8s.io/v1",
                                                "kind": "Role",
                                                "metadata": {
                                                    "name": "deployments-role-1705906106"
                                                },
                                                "rules": [
                                                    {
                                                        "apiGroups": [
                                                            "extensions",
                                                            "apps"
                                                        ],
                                                        "resources": [
                                                            "deployments"
                                                        ],
                                                        "verbs": [
                                                            "get"
                                                        ]
                                                    }
                                                ]
                                            }
                                        }
                                    ],
                                    "remediationAction": "inform",
                                    "severity": "high"
                                }
                            }
                        }
                    ]
                },
                "status": {}
            }
        ],
        "bundleVersion": {
            "Generation": 0,
            "Value": 2
        }
    },
    "kafkaoffset": "223",
    "kafkapartition": "0",
    "kafkatopic": "status.managed_hub1",
    "kafkamessagekey": "managed_hub1.LocalPolicySpec",
    "size": "4758",
    "offset": "0"
}
```
#### LocalCompliance
```
{
    "specversion": "1.0",
    "id": "2c13b7fe-bc17-11ee-95b7-b72b1f90ea91",
    "source": "managed_hub1",
    "type": "io.open-cluster-management.operator.multiclusterglobalhubs.policy.localcompliance",
    "datacontenttype": "application/json",
    "time": "2024-01-25T07:34:59.542609025Z",
    "data": {
        "objects": [
            {
                "policyId": "9cf86cb6-b8ca-4752-bad1-a481ae35aa07",
                "compliantClusters": [
                    "clc-iks-595-zcrzk-iks"
                ],
                "nonCompliantClusters": [],
                "unknownComplianceClusters": []
            },
            {
                "policyId": "9b86154b-cc49-407e-b9f5-1112fb6a3e56",
                "compliantClusters": [],
                "nonCompliantClusters": [
                    "clc-iks-595-zcrzk-iks"
                ],
                "unknownComplianceClusters": []
            }
        ],
        "bundleVersion": {
            "Generation": 0,
            "Value": 2
        }
    },
    "kafkatopic": "status.managed_hub1",
    "kafkamessagekey": "managed_hub1.LocalCompliance",
    "offset": "0",
    "kafkapartition": "0",
    "size": "390",
    "kafkaoffset": "221"
}
```
#### LocalCompleteCompliance
```
{
    "specversion": "1.0",
    "id": "509f6c76-bc17-11ee-b75f-73c1fcc519c1",
    "source": "managed_hub1",
    "type": "io.open-cluster-management.operator.multiclusterglobalhubs.policy.localcompletecompliance",
    "datacontenttype": "application/json",
    "time": "2024-01-25T07:34:59.689719771Z",
    "data": {
        "objects": [
            {
                "policyId": "9b86154b-cc49-407e-b9f5-1112fb6a3e56",
                "nonCompliantClusters": [
                    "clc-iks-595-zcrzk-iks"
                ],
                "unknownComplianceClusters": []
            }
        ],
        "baseBundleVersion": {
            "Generation": 1,
            "Value": 2
        },
        "bundleVersion": {
            "Generation": 0,
            "Value": 1
        }
    },
    "kafkapartition": "0",
    "kafkatopic": "status.managed_hub1",
    "kafkamessagekey": "managed_hub1.LocalCompleteCompliance",
    "size": "259",
    "offset": "0",
    "kafkaoffset": "222"
}
```
### ManagedCluster
```
{
    "specversion": "1.0",
    "id": "633e7b9c-bc17-11ee-af2d-df6d2ef823d8",
    "source": "managed_hub1",
    "type": "io.open-cluster-management.operator.multiclusterglobalhubs.managedcluster.localspec",
    "datacontenttype": "application/json",
    "time": "2024-01-25T07:34:54.540333196Z",
    "data": {
        "objects": [
            {
                "kind": "ManagedCluster",
                "apiVersion": "cluster.open-cluster-management.io/v1",
                "metadata": {
                    "name": "clc-iks-595-zcrzk-iks",
                    "uid": "4bee25e5-55e1-4751-98d0-06832957f271",
                    "resourceVersion": "3364861",
                    "creationTimestamp": "2024-01-25T04:06:16Z",
                    "labels": {
                        "vendor": "IKS"
                    },
                    "annotations": {
                        "global-hub.open-cluster-management.io/managed-by": "managed_hub1",
                        "open-cluster-management/created-via": "other"
                    }
                },
                "spec": {
                    "hubAcceptsClient": true,
                    "leaseDurationSeconds": 60
                },
                "status": {
                    "conditions": [
                        {
                            "type": "ManagedClusterImportSucceeded",
                            "status": "True",
                            "lastTransitionTime": "2024-01-25T04:06:51Z",
                            "reason": "ManagedClusterImported",
                            "message": "Import succeeded"
                        }
                    ],
                    "capacity": {
                        "core_worker": "6",
                        "cpu": "6",
                        "ephemeral-storage": "307829544Ki",
                        "hugepages-1Gi": "0",
                        "hugepages-2Mi": "0",
                        "memory": "11390960Ki",
                        "pods": "330",
                        "socket_worker": "0"
                    },
                    "allocatable": {
                        "cpu": "5760m",
                        "ephemeral-storage": "281739840075",
                        "hugepages-1Gi": "0",
                        "hugepages-2Mi": "0",
                        "memory": "8237040Ki",
                        "pods": "330"
                    },
                    "version": {
                        "kubernetes": "v1.28.4+IKS"
                    },
                    "clusterClaims": [
                        {
                            "name": "schedulable.open-cluster-management.io",
                            "value": "true"
                        }
                    ]
                }
            }
        ],
        "bundleVersion": {
            "Generation": 0,
            "Value": 1
        }
    },
    "kafkamessagekey": "managed_hub1.ManagedClusters",
    "kafkapartition": "0",
    "kafkatopic": "status.managed_hub1",
    "size": "2928",
    "offset": "0",
    "kafkaoffset": "219"
}
```
### ManagedHubCluster
#### Heartbeat
```
{
    "specversion": "1.0",
    "id": "8eda9722-bc17-11ee-812e-f7ec30debe33",
    "source": "managed_hub1",
    "type": "io.open-cluster-management.operator.multiclusterglobalhubs.managedhub.heartbeat",
    "datacontenttype": "application/json",
    "time": "2024-01-25T08:12:49.543071482Z",
    "data": {
        "bundleVersion": {
            "Generation": 37,
            "Value": 38
        }
    },
    "kafkatopic": "status.managed_hub1",
    "size": "70",
    "kafkapartition": "0",
    "kafkamessagekey": "managed_hub1.HubClusterHeartbeat",
    "offset": "0",
    "kafkaoffset": "262"
}
```
### HubClusterInfo
```
{
    "specversion": "1.0",
    "id": "946bba7c-bc17-11ee-9f8b-fbf106bac19c",
    "source": "managed_hub1",
    "type": "io.open-cluster-management.operator.multiclusterglobalhubs.managedhub.info",
    "datacontenttype": "application/json",
    "time": "2024-01-25T07:35:49.542703828Z",
    "data": {
        "objects": [
            {
                "consoleURL": "https://console-openshift-console.apps.xxx.com",
                "grafanaURL": "",
                "clusterId": "2adcefcb-b945-4cc5-8851-a0b3975a75ba"
            }
        ],
        "bundleVersion": {
            "Generation": 0,
            "Value": 4
        }
    },
    "kafkaoffset": "224",
    "size": "253",
    "kafkapartition": "0",
    "kafkatopic": "status.managed_hub1",
    "kafkamessagekey": "managed_hub1.HubClusterInfo",
    "offset": "0"
}
```
## Topic: event
Currently, the following resource events are supported:
- **Policy**: propagate, update
- **ManagedCluster**: provision, import, update, delete
- **ManagedHubCluster**: create, update, delete

### Policy
#### Propagate
```
{
  "specversion": "1.0",
  "id": "725892db-a1fc-4d5c-8626-4482f181b14e",
  "source": "kind-hub1",
  "type": "io.open-cluster-management.operator.multiclusterglobalhubs.local.policy.propagate",
  "datacontenttype": "application/json",
  "time": "2024-02-05T04:44:19.698187849Z",
  "data": [
    {
      "eventName": "policy-limitrange.17b0db23b941f40b",
      "eventNamespace": "local-policy-namespace",
      "message": "Policy local-policy-namespace/policy-limitrange was propagated to cluster kind-hub1-cluster1/kind-hub1-cluster1",
      "reason": "PolicyPropagation",
      "count": 1,
      "source": {
        "component": "policy-propagator"
      },
      "createdAt": "2024-02-05T03:53:25Z",
      "policyId": "13b2e003-2bdf-4c82-9bdf-f1aa7ccf608d",
      "compliance": "NonCompliant"
    }
  ],
  "kafkatopic": "event",
  "kafkaoffset": "8",
  "kafkapartition": "0",
  "kafkamessagekey": "kind-hub1",
  "extversion": "0.1"
}
```
#### Update
```
{
  "specversion": "1.0",
  "id": "3dc23865-fb4f-4b25-8d25-6f0e70424dc8",
  "source": "kind-hub1",
  "type": "io.open-cluster-management.operator.multiclusterglobalhubs.local.replicatedpolicy.update",
  "datacontenttype": "application/json",
  "time": "2024-02-05T04:44:19.698269266Z",
  "data": [
    {
      "eventName": "local-policy-namespace.policy-limitrange.17b0db2427432200",
      "eventNamespace": "kind-hub1-cluster1",
      "message": "NonCompliant; violation - limitranges [container-mem-limit-range] not found in namespace default",
      "reason": "PolicyStatusSync",
      "count": 1,
      "source": {
        "component": "policy-status-history-sync"
      },
      "createdAt": "2024-02-05T03:53:27Z",
      "policyId": "13b2e003-2bdf-4c82-9bdf-f1aa7ccf608d",
      "clusterId": "f302ce61-98e7-4d63-8dd2-65951e32fd95",
      "compliance": "NonCompliant"
    }
  ],
  "kafkapartition": "0",
  "kafkatopic": "event",
  "extversion": "0.1",
  "kafkamessagekey": "kind-hub1",
  "kafkaoffset": "9"
}
```

### ManagedCluster
#### Provision
```
{
    "specversion": "1.0",
    "id": "60d3e292-bc18-11ee-b1ad-e79256e13892",
    "source": "managed_hub1",
    "type": "io.open-cluster-management.operator.multiclusterglobalhubs.managedcluster.provision",
    "datacontenttype": "application/json",
    "time": "2024-01-25T04:06:16.542703828Z",
    "data": {
        "objects": [
            {
                "clusterId": "6b9b8545-1a84-4b55-8423-a9b28a1a4967",
                "eventName": "kube-system.provision.17ad7b80d4e6f6a4",
                "message": "The cluster (cluster1) is being provisioned now",
                "reason": "Provisioning",
                "count": 1,
                "source": "hive",
                "createdAt": "2024-01-25T04:07:07Z"
            }
        ]
    }
}
```
#### Import
```
{
    "specversion": "1.0",
    "id": "c006d9b2-bc2d-11ee-90e9-8feab4dd9214",
    "source": "managed_hub1",
    "type": "io.open-cluster-management.operator.multiclusterglobalhubs.managedcluster.import",
    "datacontenttype": "application/json",
    "time": "2024-01-25T04:06:16.542703828Z",
    "data": {
        "objects": [
            {
                "clusterId": "6b9b8545-1a84-4b55-8423-a9b28a1a4967",
                "eventName": "kube-system.import.17ad7b80d4e6f6a4",
                "message": "The cluster (cluster1) is being imported now",
                "reason": "Importing",
                "count": 1,
                "source": "import-controller",
                "createdAt": "2024-01-25T05:08:07Z"
            } 
        ]
    }
}
```
#### Update
```
{
    "specversion": "1.0",
    "id": "18b775ee-bc2e-11ee-a014-2f5782d1c2cc",
    "source": "managed_hub1",
    "type": "io.open-cluster-management.operator.multiclusterglobalhubs.managedcluster.update",
    "datacontenttype": "application/json",
    "time": "2024-01-25T04:06:16.542703828Z",
    "data": {
        "objects": [
            {
                "clusterId": "6b9b8545-1a84-4b55-8423-a9b28a1a4967",
                "eventName": "kube-system.status.17ad7b80d4e6f6a4",
                "message": "The cluster (cluster1) is available now",
                "reason": "Available",
                "count": 1,
                "source": "addon-framework",
                "createdAt": "2024-01-25T05:08:07Z"
            }  
        ]
    }
}
```
#### Detach
```
{
    "specversion": "1.0",
    "id": "a59375e4-bc2e-11ee-98bb-035b5cb373d3",
    "source": "managed_hub1",
    "type": "io.open-cluster-management.operator.multiclusterglobalhubs.managedcluster.detach",
    "datacontenttype": "application/json",
    "time": "2024-01-25T04:06:16.542703828Z",
    "data": {
        "objects": [
            {
                "clusterId": "6b9b8545-1a84-4b55-8423-a9b28a1a4967",
                "eventName": "kube-system.status.17ad7b80d4e6f6a4",
                "message": "The cluster (cluster1) is being detached now",
                "reason": "Detaching",
                "count": 1,
                "source": "klusterlet",
                "createdAt": "2024-01-25T05:08:07Z"
            } 
        ]
    }
}
```

#### Destory
```
{
    "specversion": "1.0",
    "id": "a12abe36-bc2e-11ee-98d9-a752ab52434d",
    "source": "managed_hub1",
    "type": "io.open-cluster-management.operator.multiclusterglobalhubs.managedcluster.destroy",
    "datacontenttype": "application/json",
    "time": "2024-01-25T04:06:16.542703828Z",
    "data": {
        "objects": [
            {
                "clusterId": "6b9b8545-1a84-4b55-8423-a9b28a1a4967",
                "eventName": "kube-system.status.17ad7b80d4e6f6a4",
                "message": "The cluster (cluster1) is destoryed successfully",
                "reason": "Destroyed",
                "count": 1,
                "source": "-",
                "createdAt": "2024-01-25T05:08:07Z"
            } 
        ]
    }
}
```