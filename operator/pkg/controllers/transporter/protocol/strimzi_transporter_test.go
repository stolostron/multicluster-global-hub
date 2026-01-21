package protocol

import (
	"encoding/json"
	"strings"
	"testing"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func TestNewStrimziTransporter(t *testing.T) {
	mgh := &v1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-mgh",
			Namespace: utils.GetDefaultNamespace(),
			Annotations: map[string]string{
				operatorconstants.CatalogSourceNameKey:      "test",
				operatorconstants.CatalogSourceNamespaceKey: "default",
				operatorconstants.SubscriptionPackageName:   "test-package",
				operatorconstants.SubscriptionChannel:       "test-channel",
			},
		},
		Spec: v1alpha4.MulticlusterGlobalHubSpec{
			DataLayerSpec: v1alpha4.DataLayerSpec{
				Postgres: v1alpha4.PostgresSpec{
					Retention: "2y",
				},
			},
		},
	}

	trans := NewStrimziTransporter(
		nil,
		mgh,
		WithCommunity(true),
		WithNamespacedName(types.NamespacedName{
			Name:      KafkaClusterName,
			Namespace: mgh.Namespace,
		}),
	)

	if trans.subCatalogSourceName != "test" {
		t.Errorf("catalogSource name should be test, but %v", trans.subCatalogSourceName)
	}

	if trans.subCatalogSourceNamespace != "default" {
		t.Errorf("catalogSource name should be default, but %v", trans.subCatalogSourceNamespace)
	}
	if trans.subPackageName != "test-package" {
		t.Errorf("subPackageName name should be test-package, but %v", trans.subCatalogSourceNamespace)
	}
	if trans.subChannel != "test-channel" {
		t.Errorf("subChannel name should be test-channel, but %v", trans.subCatalogSourceNamespace)
	}
}

func TestNewKafkaCluster(t *testing.T) {
	tests := []struct {
		name                 string
		mgh                  *v1alpha4.MulticlusterGlobalHub
		replica              int32
		expectedKafkaCluster string
	}{
		{
			name:    "availabilityConfig is Basic",
			replica: 1,
			mgh: &v1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-mgh",
					Namespace: utils.GetDefaultNamespace(),
					Annotations: map[string]string{
						operatorconstants.CatalogSourceNameKey:      "test",
						operatorconstants.CatalogSourceNamespaceKey: "default",
					},
				},
				Spec: v1alpha4.MulticlusterGlobalHubSpec{
					AvailabilityConfig: v1alpha4.HABasic,
					DataLayerSpec: v1alpha4.DataLayerSpec{
						Postgres: v1alpha4.PostgresSpec{
							Retention: "2y",
						},
					},
				},
			},
			expectedKafkaCluster: `{
    "metadata": {
        "name": "kafka",
        "namespace": "multicluster-global-hub",
        "labels": {
            "global-hub.open-cluster-management.io/managed-by": "global-hub"
        },
        "annotations": {
            "strimzi.io/kraft": "enabled",
            "strimzi.io/node-pools": "enabled"
        }
    },
    "spec": {
        "entityOperator": {
            "topicOperator": {},
            "userOperator": {}
        },
        "kafka": {
            "authorization": {
                "type": "simple"
            },
            "config": {
                "default.replication.factor": 1,
                "min.insync.replicas": 1,
                "offsets.topic.replication.factor": 1,
                "transaction.state.log.min.isr": 1,
                "transaction.state.log.replication.factor": 1,
                "log.segment.bytes": "268435456",
                "log.segment.ms": "3600000",
                "log.retention.bytes": "1073741824",
                "log.retention.ms": "86400000",
                "log.retention.check.interval.ms": "300000",
                "compression.type": "snappy"
            },
            "jvmOptions": {
                "-XX": {
                    "G1HeapRegionSize": "16M",
                    "InitiatingHeapOccupancyPercent": "35",
                    "MaxGCPauseMillis": "20",
                    "MaxMetaspaceFreeRatio": "80",
                    "MinMetaspaceFreeRatio": "50",
                    "UseG1GC": "true"
                },
                "-Xms": "1024M",
                "-Xmx": "1024M"
            },
            "listeners": [
                {
                    "authentication": {
                        "type": "tls"
                    },
                    "name": "tls",
                    "port": 9093,
                    "tls": true,
                    "type": "route"
                }
            ],
            "resources": {
                "limits": {
                    "cpu": "1",
                    "memory": "4Gi"
                },
                "requests": {
                    "cpu": "500m",
                    "memory": "2Gi"
                }
            },
            "version": "4.1.0"
        }
    }
}`,
		},
		{
			name:    "availabilityConfig is High",
			replica: 3,
			mgh: &v1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-mgh",
					Namespace: utils.GetDefaultNamespace(),
					Annotations: map[string]string{
						operatorconstants.CatalogSourceNameKey:      "test",
						operatorconstants.CatalogSourceNamespaceKey: "default",
					},
				},
				Spec: v1alpha4.MulticlusterGlobalHubSpec{
					DataLayerSpec: v1alpha4.DataLayerSpec{
						Postgres: v1alpha4.PostgresSpec{
							Retention: "2y",
						},
					},
				},
			},
			expectedKafkaCluster: `{
    "metadata": {
        "name": "kafka",
        "namespace": "multicluster-global-hub",
        "labels": {
            "global-hub.open-cluster-management.io/managed-by": "global-hub"
        },
        "annotations": {
            "strimzi.io/kraft": "enabled",
            "strimzi.io/node-pools": "enabled"
        }
    },
    "spec": {
        "entityOperator": {
            "topicOperator": {},
            "userOperator": {}
        },
        "kafka": {
            "authorization": {
                "type": "simple"
            },
            "config": {
                "default.replication.factor": 3,
                "min.insync.replicas": 2,
                "offsets.topic.replication.factor": 3,
                "transaction.state.log.min.isr": 2,
                "transaction.state.log.replication.factor": 3,
                "log.segment.bytes": "268435456",
                "log.segment.ms": "3600000",
                "log.retention.bytes": "1073741824",
                "log.retention.ms": "86400000",
                "log.retention.check.interval.ms": "300000",
                "compression.type": "snappy"
            },
            "jvmOptions": {
                "-XX": {
                    "G1HeapRegionSize": "16M",
                    "InitiatingHeapOccupancyPercent": "35",
                    "MaxGCPauseMillis": "20",
                    "MaxMetaspaceFreeRatio": "80",
                    "MinMetaspaceFreeRatio": "50",
                    "UseG1GC": "true"
                },
                "-Xms": "1024M",
                "-Xmx": "1024M"
            },
            "listeners": [
                {
                    "authentication": {
                        "type": "tls"
                    },
                    "name": "tls",
                    "port": 9093,
                    "tls": true,
                    "type": "route"
                }
            ],
            "resources": {
                "limits": {
                    "cpu": "1",
                    "memory": "4Gi"
                },
                "requests": {
                    "cpu": "500m",
                    "memory": "2Gi"
                }
            },
            "version": "4.1.0"
        }
    }
}`,
		},

		{
			name:    "availabilityConfig is High and expose via nodeport",
			replica: 3,
			mgh: &v1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-mgh",
					Namespace: utils.GetDefaultNamespace(),
					Annotations: map[string]string{
						operatorconstants.CatalogSourceNameKey:      "test",
						operatorconstants.CatalogSourceNamespaceKey: "default",
						operatorconstants.KafkaUseNodeport:          "",
						operatorconstants.KinDClusterIPKey:          "10.0.0.1",
					},
				},
				Spec: v1alpha4.MulticlusterGlobalHubSpec{
					DataLayerSpec: v1alpha4.DataLayerSpec{
						Postgres: v1alpha4.PostgresSpec{
							Retention: "2y",
						},
					},
				},
			},
			expectedKafkaCluster: `{
    "metadata": {
        "name": "kafka",
        "namespace": "multicluster-global-hub",
        "labels": {
            "global-hub.open-cluster-management.io/managed-by": "global-hub"
        },
        "annotations": {
            "strimzi.io/kraft": "enabled",
            "strimzi.io/node-pools": "enabled"
        }
    },
    "spec": {
        "entityOperator": {
            "topicOperator": {},
            "userOperator": {}
        },
        "kafka": {
            "authorization": {
                "type": "simple"
            },
            "config": {
                "default.replication.factor": 3,
                "min.insync.replicas": 2,
                "offsets.topic.replication.factor": 3,
                "transaction.state.log.min.isr": 2,
                "transaction.state.log.replication.factor": 3,
                "log.segment.bytes": "268435456",
                "log.segment.ms": "3600000",
                "log.retention.bytes": "1073741824",
                "log.retention.ms": "86400000",
                "log.retention.check.interval.ms": "300000",
                "compression.type": "snappy"
            },
            "jvmOptions": {
                "-XX": {
                    "G1HeapRegionSize": "16M",
                    "InitiatingHeapOccupancyPercent": "35",
                    "MaxGCPauseMillis": "20",
                    "MaxMetaspaceFreeRatio": "80",
                    "MinMetaspaceFreeRatio": "50",
                    "UseG1GC": "true"
                },
                "-Xms": "1024M",
                "-Xmx": "1024M"
            },
            "listeners": [
                {
                    "authentication": {
                        "type": "tls"
                    },
                    "configuration": {
                        "bootstrap": {
                            "nodePort": 30093
                        },
                        "brokers": [
                            {
                                "advertisedHost": "10.0.0.1",
                                "broker": 0
                            }
                        ]
                    },
                    "name": "tls",
                    "port": 9093,
                    "tls": true,
                    "type": "nodeport"
                }
            ],
            "resources": {
                "limits": {
                    "cpu": "1",
                    "memory": "4Gi"
                },
                "requests": {
                    "cpu": "500m",
                    "memory": "2Gi"
                }
            },
            "version": "4.1.0"
        }
    }
}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transporter := NewStrimziTransporter(nil, tt.mgh)
			transporter.topicPartitionReplicas = tt.replica
			cluster := transporter.newKafkaCluster(tt.mgh)
			clusterBytes, _ := json.Marshal(cluster)
			// Replace spaces
			s := strings.ReplaceAll(tt.expectedKafkaCluster, " ", "")
			// Replace newlines
			s = strings.ReplaceAll(s, "\n", "")
			if string(clusterBytes) != s {
				t.Errorf("want %v, but got %v", s, string(clusterBytes))
			}
		})
	}
}

func TestCombineACLs(t *testing.T) {
	host1 := "host1"
	host2 := "host2"
	resourceName1 := "name1"
	resourceName2 := "name2"
	type testCase struct {
		name           string
		kafkaUserAcls  []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem
		desiredAcls    []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem
		expectedResult []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem
	}

	// Test cases
	testCases := []testCase{
		{
			name: "Single Acl",
			kafkaUserAcls: []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem{
				{
					Host: &host1,
					Resource: kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResource{
						Type: kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourceTypeTopic,
						Name: &resourceName1,
					},
					Operations: []kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem{
						kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemRead,
					},
				},
			},
			desiredAcls: []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem{
				{
					Host: &host1,
					Resource: kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResource{
						Type: kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourceTypeTopic,
						Name: &resourceName1,
					},
					Operations: []kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem{
						kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemRead,
					},
				},
			},
			expectedResult: []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem{
				{
					Host: &host1,
					Resource: kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResource{
						Type: kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourceTypeTopic,
						Name: &resourceName1,
					},
					Operations: []kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem{
						kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemRead,
					},
				},
			},
		},
		{
			name: "Different Acls",
			kafkaUserAcls: []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem{
				{
					Host: &host1,
					Resource: kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResource{
						Name: &resourceName1,
						Type: kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourceTypeTopic,
					},
					Operations: []kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem{
						kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemRead,
					},
				},
			},
			desiredAcls: []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem{
				{
					Host: &host2,
					Resource: kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResource{
						Name: &resourceName2,
						Type: kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourceTypeTopic,
					},
					Operations: []kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem{
						kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemWrite,
					},
				},
			},
			expectedResult: []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem{
				{
					Host: &host1,
					Resource: kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResource{
						Name: &resourceName1,
						Type: kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourceTypeTopic,
					},
					Operations: []kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem{
						kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemRead,
					},
				},
				{
					Host: &host2,
					Resource: kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResource{
						Name: &resourceName2,
						Type: kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourceTypeTopic,
					},
					Operations: []kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem{
						kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemWrite,
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := combineACLs(tc.kafkaUserAcls, tc.desiredAcls)

			if diff := cmp.Diff(tc.expectedResult, result, cmpopts.SortSlices(func(x, y kafkav1beta2.KafkaUserSpecAuthorizationAclsElem) bool {
				return *x.Host < *y.Host &&
					*x.Resource.Name < *y.Resource.Name &&
					x.Operations[0] < y.Operations[0]
			})); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
