package protocol

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
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
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
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

	t.Cleanup(func() { config.SetTransporter(nil) })
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
	if trans.subName != StrimziOperatorName {
		t.Errorf("subName = %q, want %q", trans.subName, StrimziOperatorName)
	}
	if config.GetTransporter() != trans {
		t.Fatal("GetTransporter() should return the active Strimzi transporter")
	}
}

func TestNewStrimziTransporterCommunityDefaults(t *testing.T) {
	t.Cleanup(func() { config.SetTransporter(nil) })

	mgh := &v1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-mgh",
			Namespace: utils.GetDefaultNamespace(),
		},
	}
	trans := NewStrimziTransporter(nil, mgh, WithCommunity(true))
	if trans.subName != StrimziOperatorName {
		t.Errorf("subName = %q, want %q", trans.subName, StrimziOperatorName)
	}
	if trans.subPackageName != StrimziOperatorName {
		t.Errorf("subPackageName = %q, want %q", trans.subPackageName, StrimziOperatorName)
	}
}

func TestTransporterSingletonRefreshOnConstruction(t *testing.T) {
	ctx := context.Background()
	testScheme := runtime.NewScheme()
	_ = v1alpha4.AddToScheme(testScheme)
	_ = corev1.AddToScheme(testScheme)

	ns := utils.GetDefaultNamespace()
	mgh := &v1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{Name: "test-mgh", Namespace: ns},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme).Build()

	t.Cleanup(func() { config.SetTransporter(nil) })

	strimzi := NewStrimziTransporter(nil, mgh)
	if config.GetTransporter() != strimzi {
		t.Fatal("expected Strimzi transporter after NewStrimziTransporter")
	}

	byo := NewBYOTransporter(ctx, types.NamespacedName{Name: "transport", Namespace: ns}, fakeClient)
	if config.GetTransporter() != byo {
		t.Fatal("expected BYO transporter after NewBYOTransporter")
	}

	strimziAgain := NewStrimziTransporter(nil, mgh)
	if config.GetTransporter() != strimziAgain {
		t.Fatal("expected Strimzi transporter after switching back from BYO")
	}
	if config.GetTransporter() == byo {
		t.Fatal("GetTransporter() still returns BYO after Strimzi re-construction")
	}
}

func TestTransporterConcurrentConstruction(t *testing.T) {
	ctx := context.Background()
	testScheme := runtime.NewScheme()
	if err := v1alpha4.AddToScheme(testScheme); err != nil {
		t.Fatalf("add v1alpha4 to test scheme: %v", err)
	}
	if err := corev1.AddToScheme(testScheme); err != nil {
		t.Fatalf("add corev1 to test scheme: %v", err)
	}
	if err := kafkav1beta2.AddToScheme(testScheme); err != nil {
		t.Fatalf("add kafkav1beta2 to test scheme: %v", err)
	}

	ns := utils.GetDefaultNamespace()
	mgh := &v1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{Name: "test-mgh", Namespace: ns},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme).Build()
	fakeMgr := &fakeManager{c: fakeClient}

	t.Cleanup(func() { config.SetTransporter(nil) })

	done := make(chan struct{})
	var readerWg sync.WaitGroup
	var writerWg sync.WaitGroup

	for i := 0; i < 8; i++ {
		readerWg.Add(1)
		go func() {
			defer readerWg.Done()
			for {
				select {
				case <-done:
					return
				default:
					if tr := config.GetTransporter(); tr != nil {
						// Exercise concurrent reads only; avoid I/O that requires Kafka/Secret fixtures.
						_ = tr
					}
				}
			}
		}()
	}

	for i := 0; i < 16; i++ {
		writerWg.Add(1)
		go func(i int) {
			defer writerWg.Done()
			if i%2 == 0 {
				NewStrimziTransporter(fakeMgr, mgh)
			} else {
				NewBYOTransporter(ctx, types.NamespacedName{Name: "transport", Namespace: ns}, fakeClient)
			}
		}(i)
	}

	writerWg.Wait()
	close(done)
	readerWg.Wait()
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
	t.Cleanup(func() { config.SetTransporter(nil) })
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

	trans = NewStrimziTransporter(nil, mgh, WithOLMVersion(config.OLMVersionV0))
	if trans.olmVersion != config.OLMVersionV0 {
		t.Errorf("expected olmVersion %q, got %q", config.OLMVersionV0, trans.olmVersion)
	}

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
			t.Cleanup(func() { config.SetTransporter(nil) })
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

func TestManagedHubUserACLs(t *testing.T) {
	t.Parallel()

	clusterTopic := &transport.ClusterTopic{
		SpecTopic:      "gh-spec",
		MigrationTopic: "gh-migration",
		StatusTopic:    "gh-status.hub1",
	}
	acls := managedHubUserACLs(clusterTopic, "hub1")

	if len(acls) != 4 {
		t.Fatalf("expected 4 ACLs, got %d", len(acls))
	}

	groupACL, ok := findGroupACL(acls, "hub1")
	if !ok {
		t.Fatal("expected consumer-group ACL for hub1")
	}
	assertACLContract(
		t, groupACL,
		kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourceTypeGroup,
		"hub1",
		kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourcePatternTypeLiteral,
		[]kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem{
			kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemRead,
		},
	)

	specACL, ok := findTopicACL(acls, clusterTopic.SpecTopic)
	if !ok {
		t.Fatal("expected gh-spec topic ACL")
	}
	assertACLContract(
		t, specACL,
		kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourceTypeTopic,
		clusterTopic.SpecTopic,
		kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourcePatternTypeLiteral,
		[]kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem{
			kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemDescribe,
			kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemRead,
		},
	)

	migrationACL, ok := findTopicACL(acls, clusterTopic.MigrationTopic)
	if !ok {
		t.Fatal("expected gh-migration topic ACL")
	}
	assertACLContract(
		t, migrationACL,
		kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourceTypeTopic,
		clusterTopic.MigrationTopic,
		kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourcePatternTypeLiteral,
		[]kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem{
			kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemDescribe,
			kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemRead,
		},
	)

	statusACL, ok := findTopicACL(acls, clusterTopic.StatusTopic)
	if !ok {
		t.Fatal("expected status topic ACL")
	}
	assertACLContract(
		t, statusACL,
		kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourceTypeTopic,
		clusterTopic.StatusTopic,
		kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourcePatternTypeLiteral,
		[]kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem{
			kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemWrite,
		},
	)
}

func TestCombineManagedHubACLsUpgrade(t *testing.T) {
	t.Parallel()

	clusterTopic := &transport.ClusterTopic{
		SpecTopic:      "gh-spec",
		MigrationTopic: "gh-migration",
		StatusTopic:    "gh-status.hub1",
	}
	legacyACLs := []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem{
		utils.ConsumeGroupReadACL("*"),
		utils.GetTopicACL(clusterTopic.SpecTopic, []kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem{
			kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemDescribe,
			kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemRead,
			kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemWrite,
		}),
		utils.WriteTopicACL(clusterTopic.MigrationTopic),
	}

	merged := combineACLs(filterObsoleteManagedHubACLs(legacyACLs, clusterTopic.SpecTopic),
		managedHubUserACLs(clusterTopic, "hub1"))

	hasWildcardGroup, specOps, migrationOps := collectManagedHubACLTopicOps(merged, clusterTopic)

	if hasWildcardGroup {
		t.Fatal("wildcard consumer group ACL should be removed on upgrade")
	}

	groupACL, ok := findGroupACL(merged, "hub1")
	if !ok {
		t.Fatal("expected literal consumer-group ACL for hub1 after upgrade")
	}
	assertACLContract(
		t, groupACL,
		kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourceTypeGroup,
		"hub1",
		kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourcePatternTypeLiteral,
		[]kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem{
			kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemRead,
		},
	)

	specACL, ok := findTopicACL(merged, clusterTopic.SpecTopic)
	if !ok {
		t.Fatal("expected gh-spec topic ACL after upgrade")
	}
	assertACLContract(
		t, specACL,
		kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourceTypeTopic,
		clusterTopic.SpecTopic,
		kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourcePatternTypeLiteral,
		[]kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem{
			kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemDescribe,
			kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemRead,
		},
	)
	for _, op := range specOps {
		if op == kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemWrite {
			t.Fatal("legacy gh-spec Write ACL must be removed on upgrade")
		}
	}

	migrationReadACL, ok := findTopicACLWithOperations(
		merged, clusterTopic.MigrationTopic,
		[]kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem{
			kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemDescribe,
			kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemRead,
		},
	)
	if !ok {
		t.Fatal("expected gh-migration Describe+Read ACL after upgrade")
	}
	assertACLContract(
		t, migrationReadACL,
		kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourceTypeTopic,
		clusterTopic.MigrationTopic,
		kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourcePatternTypeLiteral,
		[]kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem{
			kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemDescribe,
			kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemRead,
		},
	)

	migrationWriteACL, ok := findTopicACLWithOperations(
		merged, clusterTopic.MigrationTopic,
		[]kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem{
			kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemWrite,
		},
	)
	if !ok {
		t.Fatal("migration topic Write ACL from migration watcher must be preserved on upgrade")
	}
	assertACLContract(
		t, migrationWriteACL,
		kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourceTypeTopic,
		clusterTopic.MigrationTopic,
		kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourcePatternTypeLiteral,
		[]kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem{
			kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemWrite,
		},
	)
	if !containsOperation(migrationOps, kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemRead) {
		t.Fatal("managed hub must retain Read on gh-migration after upgrade")
	}
}

func collectManagedHubACLTopicOps(
	merged []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem,
	clusterTopic *transport.ClusterTopic,
) (hasWildcardGroup bool, specOps, migrationOps []kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem) {
	for _, acl := range merged {
		if acl.Resource.Name == nil {
			continue
		}
		if acl.Resource.Type == kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourceTypeGroup {
			if *acl.Resource.Name == "*" {
				hasWildcardGroup = true
			}
			continue
		}
		if acl.Resource.Type != kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourceTypeTopic {
			continue
		}
		switch *acl.Resource.Name {
		case clusterTopic.SpecTopic:
			specOps = append(specOps, acl.Operations...)
		case clusterTopic.MigrationTopic:
			migrationOps = append(migrationOps, acl.Operations...)
		}
	}
	return hasWildcardGroup, specOps, migrationOps
}

func findTopicACL(acls []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem, topicName string) (
	kafkav1beta2.KafkaUserSpecAuthorizationAclsElem, bool,
) {
	for _, acl := range acls {
		if acl.Resource.Name == nil || *acl.Resource.Name != topicName {
			continue
		}
		if acl.Resource.Type != kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourceTypeTopic {
			continue
		}
		return acl, true
	}
	return kafkav1beta2.KafkaUserSpecAuthorizationAclsElem{}, false
}

func findGroupACL(acls []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem, groupName string) (
	kafkav1beta2.KafkaUserSpecAuthorizationAclsElem, bool,
) {
	for _, acl := range acls {
		if acl.Resource.Name == nil || *acl.Resource.Name != groupName {
			continue
		}
		if acl.Resource.Type != kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourceTypeGroup {
			continue
		}
		return acl, true
	}
	return kafkav1beta2.KafkaUserSpecAuthorizationAclsElem{}, false
}

func findTopicACLWithOperations(
	acls []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem,
	topicName string,
	wantOps []kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem,
) (kafkav1beta2.KafkaUserSpecAuthorizationAclsElem, bool) {
	for _, acl := range acls {
		if acl.Resource.Name == nil || *acl.Resource.Name != topicName {
			continue
		}
		if acl.Resource.Type != kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourceTypeTopic {
			continue
		}
		if aclOperationsEqual(acl.Operations, wantOps) {
			return acl, true
		}
	}
	return kafkav1beta2.KafkaUserSpecAuthorizationAclsElem{}, false
}

func assertACLContract(
	t *testing.T,
	acl kafkav1beta2.KafkaUserSpecAuthorizationAclsElem,
	wantType kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourceType,
	wantName string,
	wantPattern kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourcePatternType,
	wantOps []kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem,
) {
	t.Helper()
	if acl.Resource.Name == nil {
		t.Fatal("ACL resource name must be set")
	}
	if *acl.Resource.Name != wantName {
		t.Fatalf("ACL resource name = %q, want %q", *acl.Resource.Name, wantName)
	}
	if acl.Resource.Type != wantType {
		t.Fatalf("ACL resource type = %q, want %q", acl.Resource.Type, wantType)
	}
	if acl.Resource.PatternType == nil {
		t.Fatal("ACL pattern type must be set")
	}
	if *acl.Resource.PatternType != wantPattern {
		t.Fatalf("ACL pattern type = %q, want %q", *acl.Resource.PatternType, wantPattern)
	}
	if !aclOperationsEqual(acl.Operations, wantOps) {
		t.Fatalf("ACL operations = %#v, want %#v", acl.Operations, wantOps)
	}
}

func aclOperationsEqual(got, want []kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem) bool {
	if len(got) != len(want) {
		return false
	}
	gotCopy := append([]kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem(nil), got...)
	wantCopy := append([]kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem(nil), want...)
	sortOperations(gotCopy)
	sortOperations(wantCopy)
	for i := range gotCopy {
		if gotCopy[i] != wantCopy[i] {
			return false
		}
	}
	return true
}

func sortOperations(ops []kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem) {
	sort.Slice(ops, func(i, j int) bool {
		return ops[i] < ops[j]
	})
}

func containsOperation(ops []kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem,
	target kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem,
) bool {
	for _, op := range ops {
		if op == target {
			return true
		}
	}
	return false
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
