package protocol

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	ocv1 "github.com/operator-framework/operator-controller/api/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
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

	t.Cleanup(func() { transporter = nil })
	trans := NewStrimziTransporter(
		nil,
		mgh,
		WithCommunity(true),
		WithOLMVersion(config.OLMVersionV0),
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

func TestWithOLMVersion(t *testing.T) {
	t.Cleanup(func() { transporter = nil })
	mgh := &v1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-mgh",
			Namespace: utils.GetDefaultNamespace(),
		},
		Spec: v1alpha4.MulticlusterGlobalHubSpec{
			DataLayerSpec: v1alpha4.DataLayerSpec{
				Postgres: v1alpha4.PostgresSpec{Retention: "2y"},
			},
		},
	}
	trans := NewStrimziTransporter(nil, mgh, WithOLMVersion(config.OLMVersionV1))
	if trans.olmVersion != config.OLMVersionV1 {
		t.Errorf("expected olmVersion %q, got %q", config.OLMVersionV1, trans.olmVersion)
	}

	transporter = nil
	trans = NewStrimziTransporter(nil, mgh, WithOLMVersion(config.OLMVersionV0))
	if trans.olmVersion != config.OLMVersionV0 {
		t.Errorf("expected olmVersion %q, got %q", config.OLMVersionV0, trans.olmVersion)
	}

	transporter = nil
	trans = NewStrimziTransporter(nil, mgh)
	if trans.olmVersion != "" {
		t.Errorf("expected empty olmVersion, got %q", trans.olmVersion)
	}
}

func TestNewClusterExtension(t *testing.T) {
	mgh := &v1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-mgh",
			Namespace: "test-ns",
		},
		Spec: v1alpha4.MulticlusterGlobalHubSpec{
			DataLayerSpec: v1alpha4.DataLayerSpec{
				Postgres: v1alpha4.PostgresSpec{Retention: "2y"},
			},
		},
	}

	tests := []struct {
		name            string
		community       bool
		expectedPkg     string
		expectedChannel string
	}{
		{
			name:            "production AMQ",
			community:       false,
			expectedPkg:     DefaultAMQPackageName,
			expectedChannel: DefaultAMQChannel,
		},
		{
			name:            "community Strimzi",
			community:       true,
			expectedPkg:     CommunityPackageName,
			expectedChannel: CommunityChannel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transporter = nil
			t.Cleanup(func() { transporter = nil })
			trans := NewStrimziTransporter(
				nil, mgh,
				WithCommunity(tt.community),
				WithOLMVersion(config.OLMVersionV1),
			)
			ce := trans.newClusterExtension(mgh)

			if ce.Name != StrimziClusterExtensionName {
				t.Errorf("expected name %q, got %q", StrimziClusterExtensionName, ce.Name)
			}
			if ce.Spec.Namespace != "test-ns" {
				t.Errorf("expected namespace 'test-ns', got %q", ce.Spec.Namespace)
			}
			if ce.Spec.ServiceAccount.Name != StrimziInstallerSAName {
				t.Errorf("expected SA %q, got %q", StrimziInstallerSAName, ce.Spec.ServiceAccount.Name)
			}
			if ce.Spec.Source.SourceType != "Catalog" {
				t.Errorf("expected sourceType 'Catalog', got %q", ce.Spec.Source.SourceType)
			}
			if ce.Spec.Source.Catalog == nil {
				t.Fatal("expected catalog to be non-nil")
			}
			if ce.Spec.Source.Catalog.PackageName != tt.expectedPkg {
				t.Errorf("expected package %q, got %q", tt.expectedPkg, ce.Spec.Source.Catalog.PackageName)
			}
			if len(ce.Spec.Source.Catalog.Channels) != 1 || ce.Spec.Source.Catalog.Channels[0] != tt.expectedChannel {
				t.Errorf("expected channel [%q], got %v", tt.expectedChannel, ce.Spec.Source.Catalog.Channels)
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

func newStrimziTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}
	if err := rbacv1.AddToScheme(s); err != nil {
		t.Fatalf("failed to add rbacv1 to scheme: %v", err)
	}
	if err := ocv1.AddToScheme(s); err != nil {
		t.Fatalf("failed to add ocv1 to scheme: %v", err)
	}
	return s
}

type fakeManager struct {
	ctrl.Manager
	c client.Client
}

func (f *fakeManager) GetClient() client.Client { return f.c }

func TestEnsureClusterExtension_CreateFromScratch(t *testing.T) {
	ctx := context.Background()
	s := newStrimziTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(s).Build()

	mgh := &v1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{Name: "test-mgh", Namespace: "test-ns"},
	}

	trans := &strimziTransporter{
		ctx:            ctx,
		manager:        &fakeManager{c: c},
		mgh:            mgh,
		olmVersion:     config.OLMVersionV1,
		subChannel:     DefaultAMQChannel,
		subPackageName: DefaultAMQPackageName,
	}

	if err := trans.ensureClusterExtension(mgh); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sa := &corev1.ServiceAccount{}
	if err := c.Get(ctx, types.NamespacedName{Name: StrimziInstallerSAName, Namespace: "test-ns"}, sa); err != nil {
		t.Fatalf("expected SA to be created: %v", err)
	}

	crb := &rbacv1.ClusterRoleBinding{}
	if err := c.Get(ctx, types.NamespacedName{Name: StrimziInstallerCRBName}, crb); err != nil {
		t.Fatalf("expected CRB to be created: %v", err)
	}

	ce := &ocv1.ClusterExtension{}
	if err := c.Get(ctx, types.NamespacedName{Name: StrimziClusterExtensionName}, ce); err != nil {
		t.Fatalf("expected CE to be created: %v", err)
	}
	if ce.Spec.ServiceAccount.Name != StrimziInstallerSAName {
		t.Errorf("expected SA %q, got %q", StrimziInstallerSAName, ce.Spec.ServiceAccount.Name)
	}
	if ce.Spec.Namespace != "test-ns" {
		t.Errorf("expected namespace 'test-ns', got %q", ce.Spec.Namespace)
	}
}

func TestEnsureClusterExtension_Idempotent(t *testing.T) {
	ctx := context.Background()
	s := newStrimziTestScheme(t)

	existingSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: StrimziInstallerSAName, Namespace: "test-ns"},
	}
	existingCRB := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: StrimziInstallerCRBName},
		Subjects:   []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: StrimziInstallerSAName, Namespace: "test-ns"}},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: "cluster-admin"},
	}
	existingCE := &ocv1.ClusterExtension{
		ObjectMeta: metav1.ObjectMeta{
			Name:   StrimziClusterExtensionName,
			Labels: map[string]string{constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal},
		},
		Spec: ocv1.ClusterExtensionSpec{
			Namespace:      "test-ns",
			ServiceAccount: ocv1.ServiceAccountReference{Name: StrimziInstallerSAName},
			Source: ocv1.SourceConfig{
				SourceType: ocv1.SourceTypeCatalog,
				Catalog:    &ocv1.CatalogFilter{PackageName: DefaultAMQPackageName, Channels: []string{DefaultAMQChannel}},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(existingSA, existingCRB, existingCE).Build()
	mgh := &v1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{Name: "test-mgh", Namespace: "test-ns"},
	}

	trans := &strimziTransporter{
		ctx:            ctx,
		manager:        &fakeManager{c: c},
		mgh:            mgh,
		olmVersion:     config.OLMVersionV1,
		subChannel:     DefaultAMQChannel,
		subPackageName: DefaultAMQPackageName,
	}

	if err := trans.ensureClusterExtension(mgh); err != nil {
		t.Fatalf("unexpected error on idempotent call: %v", err)
	}
}

func TestEnsureClusterExtension_ChannelUpdate(t *testing.T) {
	ctx := context.Background()
	s := newStrimziTestScheme(t)

	existingSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: StrimziInstallerSAName, Namespace: "test-ns"},
	}
	existingCRB := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: StrimziInstallerCRBName},
		Subjects:   []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: StrimziInstallerSAName, Namespace: "test-ns"}},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: "cluster-admin"},
	}
	existingCE := &ocv1.ClusterExtension{
		ObjectMeta: metav1.ObjectMeta{Name: StrimziClusterExtensionName},
		Spec: ocv1.ClusterExtensionSpec{
			Namespace:      "test-ns",
			ServiceAccount: ocv1.ServiceAccountReference{Name: StrimziInstallerSAName},
			Source: ocv1.SourceConfig{
				SourceType: ocv1.SourceTypeCatalog,
				Catalog:    &ocv1.CatalogFilter{PackageName: DefaultAMQPackageName, Channels: []string{"old-channel"}},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(existingSA, existingCRB, existingCE).Build()
	mgh := &v1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{Name: "test-mgh", Namespace: "test-ns"},
	}

	trans := &strimziTransporter{
		ctx:            ctx,
		manager:        &fakeManager{c: c},
		mgh:            mgh,
		olmVersion:     config.OLMVersionV1,
		subChannel:     DefaultAMQChannel,
		subPackageName: DefaultAMQPackageName,
	}

	if err := trans.ensureClusterExtension(mgh); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated := &ocv1.ClusterExtension{}
	if err := c.Get(ctx, types.NamespacedName{Name: StrimziClusterExtensionName}, updated); err != nil {
		t.Fatalf("failed to get updated CE: %v", err)
	}
	if len(updated.Spec.Source.Catalog.Channels) != 1 || updated.Spec.Source.Catalog.Channels[0] != DefaultAMQChannel {
		t.Errorf("expected channel %q, got %v", DefaultAMQChannel, updated.Spec.Source.Catalog.Channels)
	}
}

func TestEnsureClusterExtension_ImmutableFieldDrift(t *testing.T) {
	ctx := context.Background()
	s := newStrimziTestScheme(t)

	existingSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: StrimziInstallerSAName, Namespace: "test-ns"},
	}
	existingCRB := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: StrimziInstallerCRBName},
		Subjects:   []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: StrimziInstallerSAName, Namespace: "test-ns"}},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: "cluster-admin"},
	}
	existingCE := &ocv1.ClusterExtension{
		ObjectMeta: metav1.ObjectMeta{Name: StrimziClusterExtensionName},
		Spec: ocv1.ClusterExtensionSpec{
			Namespace:      "different-ns",
			ServiceAccount: ocv1.ServiceAccountReference{Name: StrimziInstallerSAName},
			Source: ocv1.SourceConfig{
				SourceType: ocv1.SourceTypeCatalog,
				Catalog:    &ocv1.CatalogFilter{PackageName: DefaultAMQPackageName, Channels: []string{DefaultAMQChannel}},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(existingSA, existingCRB, existingCE).Build()
	mgh := &v1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{Name: "test-mgh", Namespace: "test-ns"},
	}

	trans := &strimziTransporter{
		ctx:            ctx,
		manager:        &fakeManager{c: c},
		mgh:            mgh,
		olmVersion:     config.OLMVersionV1,
		subChannel:     DefaultAMQChannel,
		subPackageName: DefaultAMQPackageName,
	}

	err := trans.ensureClusterExtension(mgh)
	if err == nil {
		t.Fatal("expected error for immutable field drift")
	}

	// Verify CE was deleted (will be recreated on next reconcile)
	ce := &ocv1.ClusterExtension{}
	if err := c.Get(ctx, types.NamespacedName{Name: StrimziClusterExtensionName}, ce); !errors.IsNotFound(err) {
		t.Errorf("expected NotFound for CE after immutable field drift, got: %v", err)
	}
}

func TestEnsureClusterExtension_CRBDrift(t *testing.T) {
	ctx := context.Background()
	s := newStrimziTestScheme(t)

	existingSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: StrimziInstallerSAName, Namespace: "test-ns"},
	}
	existingCRB := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: StrimziInstallerCRBName},
		Subjects:   []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: "wrong-sa", Namespace: "test-ns"}},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: "cluster-admin"},
	}

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(existingSA, existingCRB).Build()
	mgh := &v1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{Name: "test-mgh", Namespace: "test-ns"},
	}

	trans := &strimziTransporter{
		ctx:            ctx,
		manager:        &fakeManager{c: c},
		mgh:            mgh,
		olmVersion:     config.OLMVersionV1,
		subChannel:     DefaultAMQChannel,
		subPackageName: DefaultAMQPackageName,
	}

	err := trans.ensureClusterExtension(mgh)
	if err == nil {
		t.Fatal("expected error for CRB drift")
	}

	// Verify CRB was deleted (will be recreated on next reconcile)
	crb := &rbacv1.ClusterRoleBinding{}
	if err := c.Get(ctx, types.NamespacedName{Name: StrimziInstallerCRBName}, crb); !errors.IsNotFound(err) {
		t.Errorf("expected NotFound for CRB after drift, got: %v", err)
	}
}

func TestIsClusterExtensionInstalled(t *testing.T) {
	ctx := context.Background()
	s := newStrimziTestScheme(t)

	tests := []struct {
		name      string
		objects   []client.Object
		installed bool
	}{
		{
			name:      "CE not found",
			objects:   nil,
			installed: false,
		},
		{
			name: "CE exists but not installed",
			objects: []client.Object{
				&ocv1.ClusterExtension{
					ObjectMeta: metav1.ObjectMeta{Name: StrimziClusterExtensionName, Generation: 1},
					Status: ocv1.ClusterExtensionStatus{
						Conditions: []metav1.Condition{
							{Type: ocv1.TypeInstalled, Status: metav1.ConditionFalse, ObservedGeneration: 1},
						},
					},
				},
			},
			installed: false,
		},
		{
			name: "CE installed",
			objects: []client.Object{
				&ocv1.ClusterExtension{
					ObjectMeta: metav1.ObjectMeta{Name: StrimziClusterExtensionName, Generation: 1},
					Status: ocv1.ClusterExtensionStatus{
						Conditions: []metav1.Condition{
							{Type: ocv1.TypeInstalled, Status: metav1.ConditionTrue, ObservedGeneration: 1},
						},
					},
				},
			},
			installed: true,
		},
		{
			name: "CE installed but stale generation",
			objects: []client.Object{
				&ocv1.ClusterExtension{
					ObjectMeta: metav1.ObjectMeta{Name: StrimziClusterExtensionName, Generation: 2},
					Status: ocv1.ClusterExtensionStatus{
						Conditions: []metav1.Condition{
							{Type: ocv1.TypeInstalled, Status: metav1.ConditionTrue, ObservedGeneration: 1},
						},
					},
				},
			},
			installed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(s)
			if len(tt.objects) > 0 {
				builder = builder.WithObjects(tt.objects...).WithStatusSubresource(tt.objects...)
			}
			c := builder.Build()

			trans := &strimziTransporter{
				ctx:     ctx,
				manager: &fakeManager{c: c},
			}

			installed, err := trans.isClusterExtensionInstalled()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if installed != tt.installed {
				t.Errorf("expected installed=%v, got %v", tt.installed, installed)
			}
		})
	}
}
