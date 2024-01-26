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
    "id": "reg-112.LocalPolicySpec",
    "source": "reg-112",
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
    "kafkatopic": "status.reg-112",
    "kafkamessagekey": "reg-112.LocalPolicySpec",
    "size": "4758",
    "offset": "0"
}
```
#### LocalCompliance
```
{
    "specversion": "1.0",
    "id": "reg-112.LocalCompliance",
    "source": "reg-112",
    "type": "io.open-cluster-management.operator.multiclusterglobalhubs.policy.status",
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
    "kafkatopic": "status.reg-112",
    "kafkamessagekey": "reg-112.LocalCompliance",
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
    "id": "reg-112.LocalCompleteCompliance",
    "source": "reg-112",
    "type": "io.open-cluster-management.operator.multiclusterglobalhubs.policy.status",
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
    "kafkatopic": "status.reg-112",
    "kafkamessagekey": "reg-112.LocalCompleteCompliance",
    "size": "259",
    "offset": "0",
    "kafkaoffset": "222"
}
```
### ManagedCluster
```
{
    "specversion": "1.0",
    "id": "reg-112.ManagedClusters",
    "source": "reg-112",
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
                        "global-hub.open-cluster-management.io/managed-by": "reg-112",
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
    "kafkamessagekey": "reg-112.ManagedClusters",
    "kafkapartition": "0",
    "kafkatopic": "status.reg-112",
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
    "id": "reg-112.HubClusterHeartbeat",
    "source": "reg-112",
    "type": "io.open-cluster-management.operator.multiclusterglobalhubs.managedhub.status",
    "datacontenttype": "application/json",
    "time": "2024-01-25T08:12:49.543071482Z",
    "data": {
        "bundleVersion": {
            "Generation": 37,
            "Value": 38
        }
    },
    "kafkatopic": "status.reg-112",
    "size": "70",
    "kafkapartition": "0",
    "kafkamessagekey": "reg-112.HubClusterHeartbeat",
    "offset": "0",
    "kafkaoffset": "262"
}
```
### HubClusterInfo
```
{
    "specversion": "1.0",
    "id": "reg-112.HubClusterInfo",
    "source": "reg-112",
    "type": "io.open-cluster-management.operator.multiclusterglobalhubs.managedhub.status",
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
    "kafkatopic": "status.reg-112",
    "kafkamessagekey": "reg-112.HubClusterInfo",
    "offset": "0"
}
```
## Topic: event
Currently, the following resource events are supported:
- **Policy**: create, update, delete
- **ManagedCluster**: provision, import, update, delete
- **ManagedHubCluster**: create, update, delete

### Policy
#### Create
```
{
    "specversion": "1.0",
    "id": "reg-112.PolicyPropagation",
    "source": "reg-112",
    "type": "io.open-cluster-management.operator.multiclusterglobalhubs.policy.event",
    "datacontenttype": "application/json",
    "time": "2024-01-25T07:34:54.942302988Z",
    "data": {
        "events": {
            "6b9b8545-1a84-4b55-8423-a9b28a1a4967": [
                {
                    "clusterId": "23d77b87-7637-411f-af89-9e1c1a18f694",
                    "policyId": "9b86154b-cc49-407e-b9f5-1112fb6a3e56",
                    "eventName": "mem-limit.175d21cdaba3ef7e",
                    "message": Policy default/mem-limit was propagated to cluster hub2/hub2",
                    "reason": "PolicyPropagation",
                    "count": 1,
                    "source": "policy-propagator",
                    "createdAt": "2024-01-25T04:07:07Z"
                }
            ]
        }
    }
}
```
#### Update
```
{
    "specversion": "1.0",
    "id": "reg-112.LocalPolicyHistoryEvents",
    "source": "reg-112",
    "type": "io.open-cluster-management.operator.multiclusterglobalhubs.policy.event",
    "datacontenttype": "application/json",
    "time": "2024-01-25T07:34:54.942302988Z",
    "data": {
        "policyStatusEvents": {
            "6b9b8545-1a84-4b55-8423-a9b28a1a4967": [
                {
                    "clusterId": "23d77b87-7637-411f-af89-9e1c1a18f694",
                    "policyId": "9b86154b-cc49-407e-b9f5-1112fb6a3e56",
                    "compliance": "NonCompliant",
                    "eventName": "kube-system.test-role-policy-1705906106.17ad7b80d4e6f6a4",
                    "message": "NonCompliant; violation - roles [deployments-role-1705906106] not found in namespace default",
                    "reason": "PolicyStatusSync",
                    "count": 1,
                    "source": null,
                    "createdAt": "2024-01-25T04:07:07Z"
                }
            ],
            "b71adfc1-87f9-40d9-9b15-2221095a2bbd": [
                {
                    "clusterId": "23d77b87-7637-411f-af89-9e1c1a18f694",
                    "policyId": "9cf86cb6-b8ca-4752-bad1-a481ae35aa07",
                    "compliance": "Compliant",
                    "eventName": "default.test-enforce-pod-1705906106.17ad7b83028892b6",
                    "message": "Compliant; notification - pods [pod-test-1705906106] found as specified in namespace default",
                    "reason": "PolicyStatusSync",
                    "count": 1,
                    "source": null,
                    "createdAt": "2024-01-25T04:07:16Z"
                },
                {
                    "clusterId": "23d77b87-7637-411f-af89-9e1c1a18f694",
                    "policyId": "9cf86cb6-b8ca-4752-bad1-a481ae35aa07",
                    "compliance": "Compliant",
                    "eventName": "default.test-enforce-pod-1705906106.17ad7b80e1f0088a",
                    "message": "Compliant; notification - pods [pod-test-1705906106] was created successfully in namespace default",
                    "reason": "PolicyStatusSync",
                    "count": 1,
                    "source": null,
                    "createdAt": "2024-01-25T04:07:07Z"
                }
            ]
        },
        "bundleVersion": {
            "Generation": 0,
            "Value": 2
        }
    },
    "offset": "0",
    "kafkaoffset": "220",
    "kafkapartition": "0",
    "size": "1728",
    "kafkatopic": "status.reg-112",
    "kafkamessagekey": "reg-112.LocalPolicyHistoryEvents"
}
```
#### Delete
```
{
    "specversion": "1.0",
    "id": "reg-112.PolicyPropagation",
    "source": "reg-112",
    "type": "io.open-cluster-management.operator.multiclusterglobalhubs.policy.event",
    "datacontenttype": "application/json",
    "time": "2024-01-25T07:34:54.942302988Z",
    "data": {
        "events": {
            "6b9b8545-1a84-4b55-8423-a9b28a1a4967": [
                {
                    "clusterId": "23d77b87-7637-411f-af89-9e1c1a18f694",
                    "policyId": "9b86154b-cc49-407e-b9f5-1112fb6a3e56",
                    "eventName": "mem-limit.175d21cdaba3ef7e",
                    "message": Policy default/mem-limit was deleted from cluster hub2/hub2",
                    "reason": "PolicyPropagation",
                    "count": 1,
                    "source": "policy-propagator",
                    "createdAt": "2024-01-25T04:07:07Z"
                }
            ]
        }
    }
}
```
### ManagedCluster
#### Provision
```
{
    "specversion": "1.0",
    "id": "reg-112.ManagedClusterProvision",
    "source": "reg-112",
    "type": "io.open-cluster-management.operator.multiclusterglobalhubs.managedcluster.event",
    "datacontenttype": "application/json",
    "time": "2024-01-25T04:06:16.542703828Z",
    "data": {
        "events": {
            "6b9b8545-1a84-4b55-8423-a9b28a1a4967": [
                {
                    "eventName": "kube-system.provision.17ad7b80d4e6f6a4",
                    "message": "The cluster (cluster1) is being provisioned now",
                    "reason": "ManagedClusterProvisioning",
                    "count": 1,
                    "source": "hive",
                    "createdAt": "2024-01-25T04:07:07Z"
                },
                {
                    "eventName": "kube-system.provision.17ad7b80d4e6f6a4",
                    "message": "The cluster (cluster1) is provisioned successfully",
                    "reason": "ManagedClusterProvisioned",
                    "count": 1,
                    "source": "hive",
                    "createdAt": "2024-01-25T05:07:07Z"
                }  
            ]
        }
    }
}
```
#### Import
```
{
    "specversion": "1.0",
    "id": "reg-112.ManagedClusterImport",
    "source": "reg-112",
    "type": "io.open-cluster-management.operator.multiclusterglobalhubs.managedcluster.event",
    "datacontenttype": "application/json",
    "time": "2024-01-25T04:06:16.542703828Z",
    "data": {
        "events": {
            "6b9b8545-1a84-4b55-8423-a9b28a1a4967": [
                {
                    "eventName": "kube-system.import.17ad7b80d4e6f6a4",
                    "message": "The cluster (cluster1) is being imported now",
                    "reason": "ManagedClusterImporting",
                    "count": 1,
                    "source": "import-controller",
                    "createdAt": "2024-01-25T05:08:07Z"
                },
                {
                    "eventName": "kube-system.import.17ad7b80d4e6f6a4",
                    "message": "The cluster (cluster1) is imported successfully",
                    "reason": "ManagedClusterImported",
                    "count": 1,
                    "source": "import-controller",
                    "createdAt": "2024-01-25T05:09:07Z"
                }  
            ]
        }
    }
}
```
#### Update
```
{
    "specversion": "1.0",
    "id": "reg-112.ManagedClusterStatus",
    "source": "reg-112",
    "type": "io.open-cluster-management.operator.multiclusterglobalhubs.managedcluster.event",
    "datacontenttype": "application/json",
    "time": "2024-01-25T04:06:16.542703828Z",
    "data": {
        "events": {
            "6b9b8545-1a84-4b55-8423-a9b28a1a4967": [
                {
                    "eventName": "kube-system.status.17ad7b80d4e6f6a4",
                    "message": "The cluster (cluster1) is available now",
                    "reason": "ManagedClusterAvailable",
                    "count": 1,
                    "source": "addon-framework",
                    "createdAt": "2024-01-25T05:08:07Z"
                },
                {
                    "eventName": "kube-system.status.17ad7b80d4e6f6a4",
                    "message": "Search addon is unavailable now",
                    "reason": "AddonUnavailable",
                    "count": 1,
                    "source": "addon-framework",
                    "createdAt": "2024-01-25T05:09:07Z"
                }  
            ]
        }
    }
}
```
#### Update
```
{
    "specversion": "1.0",
    "id": "reg-112.ManagedClusterStatus",
    "source": "reg-112",
    "type": "io.open-cluster-management.operator.multiclusterglobalhubs.managedcluster.event",
    "datacontenttype": "application/json",
    "time": "2024-01-25T04:06:16.542703828Z",
    "data": {
        "events": {
            "6b9b8545-1a84-4b55-8423-a9b28a1a4967": [
                {
                    "eventName": "kube-system.status.17ad7b80d4e6f6a4",
                    "message": "The cluster (cluster1) is available now",
                    "reason": "ManagedClusterAvailable",
                    "count": 1,
                    "source": "addon-framework",
                    "createdAt": "2024-01-25T05:08:07Z"
                },
                {
                    "eventName": "kube-system.status.17ad7b80d4e6f6a4",
                    "message": "Search addon is unavailable now",
                    "reason": "AddonUnavailable",
                    "count": 1,
                    "source": "addon-framework",
                    "createdAt": "2024-01-25T05:09:07Z"
                }  
            ]
        }
    }
}
```
### Delete
```
{
    "specversion": "1.0",
    "id": "reg-112.ManagedClusterStatus",
    "source": "reg-112",
    "type": "io.open-cluster-management.operator.multiclusterglobalhubs.managedcluster.event",
    "datacontenttype": "application/json",
    "time": "2024-01-25T04:06:16.542703828Z",
    "data": {
        "events": {
            "6b9b8545-1a84-4b55-8423-a9b28a1a4967": [
                {
                    "eventName": "kube-system.status.17ad7b80d4e6f6a4",
                    "message": "The cluster (cluster1) is being detached now",
                    "reason": "ManagedClusterDetaching",
                    "count": 1,
                    "source": "klusterlet",
                    "createdAt": "2024-01-25T05:08:07Z"
                },
                {
                    "eventName": "kube-system.status.17ad7b80d4e6f6a4",
                    "message": "The cluster (cluster1) is detached now",
                    "reason": "ManagedClusterDetached",
                    "count": 1,
                    "source": "klusterlet",
                    "createdAt": "2024-01-25T05:09:07Z"
                }  
            ]
        }
    }
}
```