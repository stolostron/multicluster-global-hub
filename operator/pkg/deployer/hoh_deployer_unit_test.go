package deployer_test

import (
	"encoding/json"
	"fmt"
	"testing"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func Test_Comparasion(t *testing.T) {

	existingJson := `
	{
    "apiVersion": "apps/v1",
    "kind": "Deployment",
    "metadata": {
        "annotations": {
            "deployment.kubernetes.io/revision": "7"
        },
        "creationTimestamp": "2024-10-29T10:39:49Z",
        "generation": 636467,
        "labels": {
            "app": "inventory-api",
            "global-hub.open-cluster-management.io/managed-by": "global-hub-operator"
        },
        "name": "inventory-api",
        "namespace": "multicluster-global-hub",
        "ownerReferences": [
            {
                "apiVersion": "operator.open-cluster-management.io/v1alpha4",
                "blockOwnerDeletion": true,
                "controller": true,
                "kind": "MulticlusterGlobalHub",
                "name": "multiclusterglobalhub",
                "uid": "19497c00-9190-4e8a-853b-c9b168253931"
            }
        ],
        "resourceVersion": "57903183",
        "uid": "a7496ddd-a301-4859-b688-3a40a904d410"
    },
    "spec": {
        "progressDeadlineSeconds": 600,
        "replicas": 2,
        "revisionHistoryLimit": 10,
        "selector": {
            "matchLabels": {
                "app": "inventory-api"
            }
        },
        "strategy": {
            "rollingUpdate": {
                "maxSurge": "25%",
                "maxUnavailable": "25%"
            },
            "type": "RollingUpdate"
        },
        "template": {
            "metadata": {
                "creationTimestamp": null,
                "labels": {
                    "app": "inventory-api"
                }
            },
            "spec": {
                "containers": [
                    {
                        "command": [
                            "/usr/local/bin/inventory-api",
                            "serve"
                        ],
                        "env": [
                            {
                                "name": "POD_NAMESPACE",
                                "valueFrom": {
                                    "fieldRef": {
                                        "apiVersion": "v1",
                                        "fieldPath": "metadata.namespace"
                                    }
                                }
                            },
                            {
                                "name": "INVENTORY_API_CONFIG",
                                "value": "/inventory/inventory-api-config.yaml"
                            }
                        ],
                        "image": "quay.io/stolostron/inventory-api:latest",
                        "imagePullPolicy": "Always",
                        "livenessProbe": {
                            "failureThreshold": 3,
                            "httpGet": {
                                "path": "/api/inventory/v1/livez",
                                "port": 8081,
                                "scheme": "HTTPS"
                            },
                            "initialDelaySeconds": 15,
                            "periodSeconds": 20,
                            "successThreshold": 1,
                            "timeoutSeconds": 1
                        },
                        "name": "inventory-api",
                        "ports": [
                            {
                                "containerPort": 8081,
                                "name": "http-server",
                                "protocol": "TCP"
                            },
                            {
                                "containerPort": 9081,
                                "name": "grpc-server",
                                "protocol": "TCP"
                            }
                        ],
                        "readinessProbe": {
                            "failureThreshold": 3,
                            "httpGet": {
                                "path": "/api/inventory/v1/readyz",
                                "port": 8081,
                                "scheme": "HTTPS"
                            },
                            "initialDelaySeconds": 5,
                            "periodSeconds": 10,
                            "successThreshold": 1,
                            "timeoutSeconds": 1
                        },
                        "resources": {},
                        "terminationMessagePath": "/dev/termination-log",
                        "terminationMessagePolicy": "File",
                        "volumeMounts": [
                            {
                                "mountPath": "/inventory",
                                "name": "config-volume",
                                "readOnly": true
                            },
                            {
                                "mountPath": "/inventory/certs",
                                "name": "server-certs",
                                "readOnly": true
                            },
                            {
                                "mountPath": "/inventory/client-certs",
                                "name": "client-ca",
                                "readOnly": true
                            },
                            {
                                "mountPath": "/inventory/kafka-certs",
                                "name": "kafka-certs",
                                "readOnly": true
                            }
                        ]
                    }
                ],
                "dnsPolicy": "ClusterFirst",
                "initContainers": [
                    {
                        "command": [
                            "/usr/local/bin/inventory-api",
                            "migrate"
                        ],
                        "env": [
                            {
                                "name": "INVENTORY_API_CONFIG",
                                "value": "/inventory/inventory-api-config.yaml"
                            }
                        ],
                        "image": "quay.io/stolostron/inventory-api:latest",
                        "imagePullPolicy": "Always",
                        "name": "migration",
                        "resources": {
                            "requests": {
                                "cpu": "1m",
                                "memory": "20Mi"
                            }
                        },
                        "terminationMessagePath": "/dev/termination-log",
                        "terminationMessagePolicy": "File",
                        "volumeMounts": [
                            {
                                "mountPath": "/inventory",
                                "name": "config-volume",
                                "readOnly": true
                            }
                        ]
                    }
                ],
                "restartPolicy": "Always",
                "schedulerName": "default-scheduler",
                "securityContext": {},
                "serviceAccount": "inventory-api",
                "serviceAccountName": "inventory-api",
                "terminationGracePeriodSeconds": 30,
                "volumes": [
                    {
                        "name": "config-volume",
                        "secret": {
                            "defaultMode": 420,
                            "secretName": "inventory-api-config"
                        }
                    },
                    {
                        "name": "server-certs",
                        "secret": {
                            "defaultMode": 420,
                            "secretName": "inventory-api-server-certs"
                        }
                    },
                    {
                        "name": "client-ca",
                        "secret": {
                            "defaultMode": 420,
                            "secretName": "inventory-api-client-ca-certs"
                        }
                    },
                    {
                        "name": "kafka-certs",
                        "secret": {
                            "defaultMode": 420,
                            "secretName": "inventory-api-kafka-certs"
                        }
                    }
                ]
            }
        }
    },
    "status": {
        "availableReplicas": 2,
        "conditions": [
            {
                "lastTransitionTime": "2024-10-29T10:40:09Z",
                "lastUpdateTime": "2024-10-29T10:40:09Z",
                "message": "Deployment has minimum availability.",
                "reason": "MinimumReplicasAvailable",
                "status": "True",
                "type": "Available"
            },
            {
                "lastTransitionTime": "2024-10-29T10:39:49Z",
                "lastUpdateTime": "2024-10-29T10:44:49Z",
                "message": "ReplicaSet \"inventory-api-7b7cd944d6\" has successfully progressed.",
                "reason": "NewReplicaSetAvailable",
                "status": "True",
                "type": "Progressing"
            }
        ],
        "observedGeneration": 636467,
        "readyReplicas": 2,
        "replicas": 2,
        "updatedReplicas": 2
    }
}`

	desiredJson := `
	{
    "apiVersion": "apps/v1",
    "kind": "Deployment",
    "metadata": {
        "labels": {
            "app": "inventory-api",
            "global-hub.open-cluster-management.io/managed-by": "global-hub-operator"
        },
        "name": "inventory-api",
        "namespace": "multicluster-global-hub",
        "ownerReferences": [
            {
                "apiVersion": "operator.open-cluster-management.io/v1alpha4",
                "blockOwnerDeletion": true,
                "controller": true,
                "kind": "MulticlusterGlobalHub",
                "name": "multiclusterglobalhub",
                "uid": "19497c00-9190-4e8a-853b-c9b168253931"
            }
        ]
    },
    "spec": {
        "replicas": 2,
        "selector": {
            "matchLabels": {
                "app": "inventory-api"
            }
        },
        "template": {
            "metadata": {
                "labels": {
                    "app": "inventory-api"
                }
            },
            "spec": {
                "containers": [
                    {
                        "command": [
                            "/usr/local/bin/inventory-api",
                            "serve"
                        ],
                        "env": [
                            {
                                "name": "POD_NAMESPACE",
                                "valueFrom": {
                                    "fieldRef": {
                                        "apiVersion": "v1",
                                        "fieldPath": "metadata.namespace"
                                    }
                                }
                            },
                            {
                                "name": "INVENTORY_API_CONFIG",
                                "value": "/inventory/inventory-api-config.yaml"
                            }
                        ],
                        "image": "quay.io/stolostron/inventory-api:latest",
                        "imagePullPolicy": "Always",
                        "livenessProbe": {
                            "httpGet": {
                                "path": "/api/inventory/v1/livez",
                                "port": 8081,
                                "scheme": "HTTPS"
                            },
                            "initialDelaySeconds": 15,
                            "periodSeconds": 20
                        },
                        "name": "inventory-api",
                        "ports": [
                            {
                                "containerPort": 8081,
                                "name": "http-server",
                                "protocol": "TCP"
                            },
                            {
                                "containerPort": 9081,
                                "name": "grpc-server",
                                "protocol": "TCP"
                            }
                        ],
                        "readinessProbe": {
                            "httpGet": {
                                "path": "/api/inventory/v1/readyz",
                                "port": 8081,
                                "scheme": "HTTPS"
                            },
                            "initialDelaySeconds": 5,
                            "periodSeconds": 10
                        },
                        "volumeMounts": [
                            {
                                "mountPath": "/inventory",
                                "name": "config-volume",
                                "readOnly": true
                            },
                            {
                                "mountPath": "/inventory/certs",
                                "name": "server-certs",
                                "readOnly": true
                            },
                            {
                                "mountPath": "/inventory/client-certs",
                                "name": "client-ca",
                                "readOnly": true
                            },
                            {
                                "mountPath": "/inventory/kafka-certs",
                                "name": "kafka-certs",
                                "readOnly": true
                            }
                        ]
                    }
                ],
                "initContainers": [
                    {
                        "command": [
                            "/usr/local/bin/inventory-api",
                            "migrate"
                        ],
                        "env": [
                            {
                                "name": "INVENTORY_API_CONFIG",
                                "value": "/inventory/inventory-api-config.yaml"
                            }
                        ],
                        "image": "quay.io/stolostron/inventory-api:latest",
                        "imagePullPolicy": "Always",
                        "name": "migration",
                        "resources": {
                            "requests": {
                                "cpu": "1m",
                                "memory": "20Mi"
                            }
                        },
                        "volumeMounts": [
                            {
                                "mountPath": "/inventory",
                                "name": "config-volume",
                                "readOnly": true
                            }
                        ]
                    }
                ],
                "serviceAccountName": "inventory-api",
                "volumes": [
                    {
                        "name": "config-volume",
                        "secret": {
                            "secretName": "inventory-api-config"
                        }
                    },
                    {
                        "name": "server-certs",
                        "secret": {
                            "secretName": "inventory-api-server-certs"
                        }
                    },
                    {
                        "name": "client-ca",
                        "secret": {
                            "secretName": "inventory-api-client-ca-certs"
                        }
                    },
                    {
                        "name": "kafka-certs",
                        "secret": {
                            "secretName": "inventory-api-kafka-certs"
                        }
                    }
                ]
            }
        }
    }
}`
	existingStructure := &unstructured.Unstructured{}
	json.Unmarshal([]byte(existingJson), existingStructure)
	desiredStructure := &unstructured.Unstructured{}
	json.Unmarshal([]byte(desiredJson), desiredStructure)
	fmt.Println("equal is", apiequality.Semantic.DeepDerivative(desiredStructure, existingStructure))

	t.Fatal("error")
}
