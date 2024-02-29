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
  "id": "7923be59-0050-4112-90c0-1e8d1ec4d486",
  "source": "kind-hub1",
  "type": "io.open-cluster-management.operator.multiclusterglobalhubs.policy.localspec",
  "datacontenttype": "application/json",
  "time": "2024-02-29T03:01:12.717129594Z",
  "data": [
    {
      "kind": "Policy",
      "apiVersion": "policy.open-cluster-management.io/v1",
      "metadata": {
        "name": "policy-limitrange",
        "namespace": "local-policy-namespace",
        "uid": "cd0a3ae3-6579-4fbc-b9c1-eb7e9d15dd6d",
        "resourceVersion": "849216",
        "creationTimestamp": "2024-02-29T03:01:04Z",
        "annotations": {
          "kubectl.kubernetes.io/last-applied-configuration": "{\"apiVersion\":\"policy.open-cluster-management.io/v1\",\"kind\":\"Policy\",\"metadata\":{\"annotations\":{\"policy.open-cluster-management.io/categories\":\"PR.IP Information Protection Processes and Procedures\",\"policy.open-cluster-management.io/controls\":\"PR.IP-1 Baseline Configuration\",\"policy.open-cluster-management.io/standards\":\"NIST-CSF\"},\"name\":\"policy-limitrange\",\"namespace\":\"local-policy-namespace\"},\"spec\":{\"disabled\":false,\"policy-templates\":[{\"objectDefinition\":{\"apiVersion\":\"policy.open-cluster-management.io/v1\",\"kind\":\"ConfigurationPolicy\",\"metadata\":{\"name\":\"policy-limitrange-container-mem-limit-range\"},\"spec\":{\"namespaceSelector\":{\"exclude\":[\"kube-*\"],\"include\":[\"default\"]},\"object-templates\":[{\"complianceType\":\"musthave\",\"objectDefinition\":{\"apiVersion\":\"v1\",\"kind\":\"LimitRange\",\"metadata\":{\"name\":\"container-mem-limit-range\"},\"spec\":{\"limits\":[{\"default\":{\"memory\":\"512Mi\"},\"defaultRequest\":{\"memory\":\"256Mi\"},\"type\":\"Container\"}]}}}],\"remediationAction\":\"inform\",\"severity\":\"medium\"}}}],\"remediationAction\":\"inform\"}}\n",
          "policy.open-cluster-management.io/categories": "PR.IP Information Protection Processes and Procedures",
          "policy.open-cluster-management.io/controls": "PR.IP-1 Baseline Configuration",
          "policy.open-cluster-management.io/standards": "NIST-CSF"
        }
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
                "name": "policy-limitrange-container-mem-limit-range"
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
                      "apiVersion": "v1",
                      "kind": "LimitRange",
                      "metadata": {
                        "name": "container-mem-limit-range"
                      },
                      "spec": {
                        "limits": [
                          {
                            "default": {
                              "memory": "512Mi"
                            },
                            "defaultRequest": {
                              "memory": "256Mi"
                            },
                            "type": "Container"
                          }
                        ]
                      }
                    }
                  }
                ],
                "remediationAction": "inform",
                "severity": "medium"
              }
            }
          }
        ]
      },
      "status": {}
    }
  ],
  "kafkamessagekey": "kind-hub1",
  "kafkaoffset": "1214",
  "extversion": "1.3",
  "kafkapartition": "0",
  "kafkatopic": "status.kind-hub1"
}
```
#### LocalCompliance
```
{
  "specversion": "1.0",
  "id": "4b3e7f44-289c-4c67-b1d6-8e5d4b59ecbd",
  "source": "kind-hub1",
  "type": "io.open-cluster-management.operator.multiclusterglobalhubs.policy.localcompliance",
  "datacontenttype": "application/json",
  "time": "2024-02-29T03:01:07.625306267Z",
  "data": [
    {
      "policyId": "cd0a3ae3-6579-4fbc-b9c1-eb7e9d15dd6d",
      "compliantClusters": [],
      "nonCompliantClusters": [],
      "unknownComplianceClusters": [
        "kind-hub1-cluster1"
      ]
    }
  ],
  "kafkatopic": "status.kind-hub1",
  "kafkamessagekey": "kind-hub1",
  "extversion": "0.1",
  "kafkaoffset": "1205",
  "kafkapartition": "0"
}

```
#### LocalCompleteCompliance
```
{
  "specversion": "1.0",
  "id": "d6561516-c8d5-4c3f-98e3-df650ee8e809",
  "source": "kind-hub1",
  "type": "io.open-cluster-management.operator.multiclusterglobalhubs.policy.localcompletecompliance",
  "datacontenttype": "application/json",
  "time": "2024-02-29T03:01:12.624601184Z",
  "data": [
    {
      "policyId": "cd0a3ae3-6579-4fbc-b9c1-eb7e9d15dd6d",
      "nonCompliantClusters": [
        "kind-hub1-cluster1"
      ],
      "unknownComplianceClusters": []
    }
  ],
  "kafkaoffset": "1212",
  "kafkapartition": "0",
  "extversion": "1.2",
  "kafkatopic": "status.kind-hub1",
  "kafkamessagekey": "kind-hub1",
  "extdependencyversion": "1.1"
}
```
### ManagedCluster
```
{
  "specversion": "1.0",
  "id": "213d0259-8999-46bf-aa13-bf4a3684075f",
  "source": "kind-hub1",
  "type": "io.open-cluster-management.operator.multiclusterglobalhubs.managedcluster",
  "datacontenttype": "application/json",
  "time": "2024-02-29T02:53:32.621450791Z",
  "data": [
    {
      "kind": "ManagedCluster",
      "apiVersion": "cluster.open-cluster-management.io/v1",
      "metadata": {
        "name": "kind-hub1-cluster1",
        "uid": "f6871dc9-1052-4a73-ba82-0db30bc41e89",
        "resourceVersion": "843851",
        "creationTimestamp": "2024-02-27T01:48:31Z",
        "labels": {
          "cluster.open-cluster-management.io/clusterset": "default",
          "feature.open-cluster-management.io/addon-application-manager": "available"
        },
        "annotations": {
          "global-hub.open-cluster-management.io/managed-by": "kind-hub1"
        }
      },
      "spec": {
        "managedClusterClientConfigs": [
          {
            "url": "https://hub1-cluster1-control-plane:6443",
            "caBundle": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUMvakNDQWVhZ0F3SUJBZ0lCQURBTkJna3Foa2lHOXcwQkFRc0ZBREFWTVJNd0VRWURWUVFERXdwcmRXSmwKY201bGRHVnpNQjRYRFRJME1ESXlOekF4TkRZeU5Wb1hEVE0wTURJeU5EQXhORFl5TlZvd0ZURVRNQkVHQTFVRQpBeE1LYTNWaVpYSnVaWFJsY3pDQ0FTSXdEUVlKS29aSWh2Y05BUUVCQlFBRGdnRVBBRENDQVFvQ2dnRUJBS3RGClF1c1MwVldPd3A3Z2lZTTBEOTRsRTFNUVRkSC9jWFZFV3RINXFUNGEwUVhzQVkvbWd4SEVvVmhjaVdmR0tSQVcKaDJLenNUL1dJbHFNMmx5YUI1Vktac1p6WEZ1d1FOZUpxM0ZubW5pTzNHZnZEaXlWc2tWUndEclY5MkxpSkhoZgpQZG52ZzlhUk1vUThUQ0lwblNkNXFVUFZuZ3kzSWwzTGxKa2VDTnJDTjhBVWU0V1kzMURDSVhzS0l0RnFoNHRmCitPMHVDUk1rSS9wazAxeG5CQVErN21xS3hVWStDTnZqRlhtcmxwUjdzTlZ2OE5NbVhveElkaktibkIzMkZoVUYKM0VubkJsSHRPVEI0QkU1WHlHNzU5WFN4a3JrTUcycGFsbHJoYXVEMGZPVVB2TFNEZGs1emo5blZiaTlsWHlLZwpsemtFL1Z1a3hSYUc2NTZ5Ry84Q0F3RUFBYU5aTUZjd0RnWURWUjBQQVFIL0JBUURBZ0trTUE4R0ExVWRFd0VCCi93UUZNQU1CQWY4d0hRWURWUjBPQkJZRUZDbUtBRm1QdndRbUVjYzdhd2dpYndmTVVxbjRNQlVHQTFVZEVRUU8KTUF5Q0NtdDFZbVZ5Ym1WMFpYTXdEUVlKS29aSWh2Y05BUUVMQlFBRGdnRUJBR1pTdjNXdUUxa0hZODRaQTBuZQpsWUVrZHhhbkRIT0p3SmdKVHkwQ0ZML0sxZzd2NWhZK2xLeEFBR0dOejA3V09Ob2xHK3BRYVY2SXVJOTE1citTClM2TFM2WktFaXUwcStheDFKTnFSZVYzZlBmazViZEdETFBCQkkrQk5qZnQ2bHhRMDBHS2YrOGFaUnI0b2ZPblcKa29BOVNIdk1RUUljREZVZ2FxMmZhVzNoVXF1OHRGR2gwaW5wYXovNmwrQ2Ntb201bVpmbi8vcGtjeUVMWmkrKwoyUm5VclVMdDBGM3dxQ1NiajV4c1dtaXkzbXNnT1FBMUZ5NnZQTyt3N3pFYVhsbVlaeFYvSHJWenFteUxsTnVsCmlSa2Y1VHRKUXNMbzBMWHNwOU8vdSs5R09qOUJGVUdIRG53MDJqdGZndDdnRUpCeXZsajkzOFl4eCs1a01uUjgKYVZBPQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg=="
          }
        ],
        "hubAcceptsClient": true,
        "leaseDurationSeconds": 60
      },
      "status": {
        "conditions": [
          {
            "type": "HubAcceptedManagedCluster",
            "status": "True",
            "lastTransitionTime": "2024-02-27T01:48:32Z",
            "reason": "HubClusterAdminAccepted",
            "message": "Accepted by hub cluster admin"
          },
          {
            "type": "ManagedClusterJoined",
            "status": "True",
            "lastTransitionTime": "2024-02-27T01:48:32Z",
            "reason": "ManagedClusterJoined",
            "message": "Managed cluster joined"
          },
          {
            "type": "ManagedClusterConditionAvailable",
            "status": "True",
            "lastTransitionTime": "2024-02-27T01:48:32Z",
            "reason": "ManagedClusterAvailable",
            "message": "Managed cluster is available"
          }
        ],
        "capacity": {
          "cpu": "8",
          "ephemeral-storage": "83156972Ki",
          "hugepages-1Gi": "0",
          "hugepages-2Mi": "0",
          "memory": "32232364Ki",
          "pods": "110"
        },
        "allocatable": {
          "cpu": "8",
          "ephemeral-storage": "83156972Ki",
          "hugepages-1Gi": "0",
          "hugepages-2Mi": "0",
          "memory": "32232364Ki",
          "pods": "110"
        },
        "version": {
          "kubernetes": "v1.24.0"
        },
        "clusterClaims": [
          {
            "name": "id.k8s.io",
            "value": "893013f7-90cf-4d44-b598-32437e4fca3c"
          }
        ]
      }
    }
  ],
  "kafkamessagekey": "kind-hub1",
  "kafkapartition": "0",
  "extversion": "0.1",
  "kafkaoffset": "1183",
  "kafkatopic": "status.kind-hub1"
}
```
### ManagedHubCluster
#### Heartbeat
```
{
  "specversion": "1.0",
  "id": "1f062142-0ab5-43d3-b428-171e323f8a49",
  "source": "kind-hub1",
  "type": "io.open-cluster-management.operator.multiclusterglobalhubs.managedhub.heartbeat",
  "datacontenttype": "application/json",
  "time": "2024-02-29T02:57:27.627736066Z",
  "data": [],
  "kafkaoffset": "1193",
  "kafkamessagekey": "kind-hub1",
  "extversion": "3.0",
  "kafkapartition": "0",
  "kafkatopic": "status.kind-hub1"
}
```
### HubClusterInfo
```
{
  "specversion": "1.0",
  "id": "11661511-979c-4e0c-810e-a62b5ddb11c1",
  "source": "kind-hub1",
  "type": "io.open-cluster-management.operator.multiclusterglobalhubs.managedhub.info",
  "datacontenttype": "application/json",
  "time": "2024-02-29T02:54:27.627870336Z",
  "data": {
    "consoleURL": "https://console-openshift-console.apps.xxx.com",
    "grafanaURL": "",
    "clusterId": "8606cf88-c711-4dae-ba7e-2e4d54227887"
  },
  "kafkapartition": "0",
  "kafkatopic": "status.kind-hub1",
  "kafkaoffset": "1186",
  "extversion": "0.1",
  "kafkamessagekey": "kind-hub1"
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
  "id": "4e85317c-9208-4c9d-be22-4a2725867670",
  "source": "kind-hub2",
  "type": "io.open-cluster-management.operator.multiclusterglobalhubs.event.localpolicy.propagate",
  "datacontenttype": "application/json",
  "time": "2024-02-29T03:01:06.30007874Z",
  "data": [
    {
      "eventName": "policy-limitrange.17b8363660d39188",
      "eventNamespace": "local-policy-namespace",
      "message": "Policy local-policy-namespace/policy-limitrange was propagated to cluster kind-hub2-cluster1/kind-hub2-cluster1",
      "reason": "PolicyPropagation",
      "count": 1,
      "source": {
        "component": "policy-propagator"
      },
      "createdAt": "2024-02-29T03:01:05Z",
      "policyId": "1f2deb7a-0d29-4762-b0fc-daa3ba16c5b5",
      "compliance": "Unknown"
    }
  ],
  "extversion": "0.1",
  "kafkaoffset": "0",
  "kafkapartition": "0",
  "kafkatopic": "event",
  "kafkamessagekey": "kind-hub2"
}
```
#### Update
```
{
  "specversion": "1.0",
  "id": "5b5917b5-1fa2-4eb8-a7fa-c1d97dc96218",
  "source": "kind-hub2",
  "type": "io.open-cluster-management.operator.multiclusterglobalhubs.event.localreplicatedpolicy.update",
  "datacontenttype": "application/json",
  "time": "2024-02-29T03:01:16.387894285Z",
  "data": [
    {
      "eventName": "local-policy-namespace.policy-limitrange.17b83638614ff6b7",
      "eventNamespace": "kind-hub2-cluster1",
      "message": "NonCompliant; violation - limitranges [container-mem-limit-range] not found in namespace default",
      "reason": "PolicyStatusSync",
      "count": 1,
      "source": {
        "component": "policy-status-history-sync"
      },
      "createdAt": "2024-02-29T03:01:14Z",
      "policyId": "1f2deb7a-0d29-4762-b0fc-daa3ba16c5b5",
      "clusterId": "0884ef05-115d-46f5-bbda-f759adcbbe5b",
      "compliance": "NonCompliant"
    }
  ],
  "kafkamessagekey": "kind-hub2",
  "extversion": "0.1",
  "kafkaoffset": "3",
  "kafkapartition": "0",
  "kafkatopic": "event"
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