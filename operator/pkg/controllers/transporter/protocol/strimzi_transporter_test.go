package protocol

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
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
        "creationTimestamp": null,
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
                "transaction.state.log.replication.factor": 1
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
                "requests": {
                    "cpu": "25m",
                    "memory": "128Mi"
                }
            },
            "version": "4.0.0"
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
        "creationTimestamp": null,
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
                "transaction.state.log.replication.factor": 3
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
                "requests": {
                    "cpu": "25m",
                    "memory": "128Mi"
                }
            },
            "version": "4.0.0"
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
        "creationTimestamp": null,
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
                "transaction.state.log.replication.factor": 3
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
                "requests": {
                    "cpu": "25m",
                    "memory": "128Mi"
                }
            },
            "version": "4.0.0"
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

func TestManagedHubUserACLs(t *testing.T) {
	t.Parallel()

	clusterTopic := &transport.ClusterTopic{
		SpecTopic:      "gh-spec",
		MigrationTopic: "gh-migration",
		StatusTopic:    "gh-status.hub1",
	}
	acls := managedHubUserACLs(clusterTopic, "hub1")

	// 3 ACLs: consumer-group, gh-spec (read-only), gh-status (write).
	// Migration topic ACLs are granted dynamically by MigrationACLReconciler.
	if len(acls) != 3 {
		t.Fatalf("expected 3 ACLs, got %d", len(acls))
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

	// Migration topic ACL must NOT be statically granted.
	if _, ok := findTopicACL(acls, clusterTopic.MigrationTopic); ok {
		t.Fatal("migration topic ACL must not be statically granted; it is managed dynamically")
	}

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
		utils.ReadTopicACL(clusterTopic.MigrationTopic, false),
	}

	merged := combineACLs(filterObsoleteManagedHubACLs(legacyACLs, clusterTopic.SpecTopic, clusterTopic.MigrationTopic),
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

	// Migration Read ACL must NOT be statically granted after upgrade.
	// It is managed dynamically by MigrationACLReconciler.
	_, hasStaticMigrationRead := findTopicACLWithOperations(
		merged, clusterTopic.MigrationTopic,
		[]kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem{
			kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemDescribe,
			kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemRead,
		},
	)
	if hasStaticMigrationRead {
		t.Fatal("migration topic Read ACL must not be statically granted after upgrade")
	}

	// Migration Write ACL from the MigrationACLReconciler must be preserved.
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
	if containsOperation(migrationOps, kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemRead) {
		t.Fatal("migration topic Read ACL must not be statically retained after upgrade")
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
