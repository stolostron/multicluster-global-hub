// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package migration

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	klusterletv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	addonv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/controller"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// go test -run ^TestMigrationSourceHubSyncer$ github.com/stolostron/multicluster-global-hub/agent/pkg/spec/syncers -v
func TestMigrationSourceHubSyncer(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add corev1 to scheme: %v", err)
	}
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add clientgoscheme to scheme: %v", err)
	}
	if err := clusterv1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add clusterv1 to scheme: %v", err)
	}
	if err := clusterv1beta1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add clusterv1beta1 to scheme: %v", err)
	}
	if err := operatorv1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add operatorv1 to scheme: %v", err)
	}
	if err := klusterletv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add klusterletv1alpha1 to scheme: %v", err)
	}
	if err := addonv1.SchemeBuilder.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add addonv1 to scheme: %v", err)
	}
	if err := mchv1.SchemeBuilder.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add mchv1 to scheme: %v", err)
	}

	currentSyncerMigrationId := "020340324302432049234023040320"

	cases := []struct {
		name                         string
		receivedMigrationEventBundle migration.MigrationSourceBundle
		initObjects                  []client.Object
		expectedProduceEvent         *cloudevents.Event
		expectedObjects              []client.Object
	}{
		{
			name: "Initializing: migrate cluster1 from hub1 to hub2",
			initObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster1",
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient:     true,
						LeaseDurationSeconds: 60,
					},
				},
				&addonv1.KlusterletAddonConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cluster1",
						Namespace: "cluster1",
					},
				},
				&mchv1.MultiClusterHub{
					ObjectMeta: metav1.ObjectMeta{
						Name: "multiclusterhub",
					},
					Spec: mchv1.MultiClusterHubSpec{},
					Status: mchv1.MultiClusterHubStatus{
						CurrentVersion: "2.13.0",
					},
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-configmap",
						Namespace: "cluster1",
						UID:       "020340324302432049234023040320",
					},
					Data: map[string]string{"hello": "world"},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-secret",
						Namespace: "cluster1",
						UID:       "020340324302432049234023040321",
					},
					StringData: map[string]string{"test": "secret"},
				},
			},
			receivedMigrationEventBundle: migration.MigrationSourceBundle{
				MigrationId: currentSyncerMigrationId,
				ToHub:       "hub2",
				Stage:       migrationv1alpha1.PhaseInitializing,
				BootstrapSecret: &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: bootstrapSecretNamePrefix + "hub2", Namespace: "multicluster-engine"},
				},
				ManagedClusters: []string{"cluster1"},
			},
			expectedProduceEvent: func() *cloudevents.Event {
				// report to global hub deploying status
				configs.SetAgentConfig(&configs.AgentConfig{LeafHubName: "hub1"})

				evt := cloudevents.NewEvent()
				evt.SetType(string(enum.ManagedClusterMigrationType)) // spec message: initialize -> deploy
				evt.SetSource("hub1")
				evt.SetExtension(constants.CloudEventExtensionKeyClusterName, "global-hub")
				evt.SetExtension(eventversion.ExtVersion, "0.1")
				return &evt
			}(),
		},
		{
			name: "Registering: migrate cluster1 from hub1 to hub2",
			initObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster1",
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient:     true,
						LeaseDurationSeconds: 60,
					},
				},
				&mchv1.MultiClusterHub{
					ObjectMeta: metav1.ObjectMeta{
						Name: "multiclusterhub",
					},
					Spec: mchv1.MultiClusterHubSpec{},
					Status: mchv1.MultiClusterHubStatus{
						CurrentVersion: "2.14.0",
					},
				},
			},
			receivedMigrationEventBundle: migration.MigrationSourceBundle{
				MigrationId:     currentSyncerMigrationId,
				ToHub:           "hub2",
				Stage:           migrationv1alpha1.PhaseRegistering,
				ManagedClusters: []string{"cluster1"},
			},
			expectedProduceEvent: nil,
			expectedObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster1",
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient:     false,
						LeaseDurationSeconds: 60,
					},
				},
			},
		},
		{
			name: "Cleaned up: migrate cluster1 from hub1 to hub2",
			initObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster1",
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient:     true,
						LeaseDurationSeconds: 60,
					},
				},
				&mchv1.MultiClusterHub{
					ObjectMeta: metav1.ObjectMeta{
						Name: "multiclusterhub",
					},
					Spec: mchv1.MultiClusterHubSpec{},
					Status: mchv1.MultiClusterHubStatus{
						CurrentVersion: "2.14.0",
					},
				},
			},
			receivedMigrationEventBundle: migration.MigrationSourceBundle{
				MigrationId: currentSyncerMigrationId,
				ToHub:       "hub2",
				Stage:       migrationv1alpha1.PhaseCleaning,
				BootstrapSecret: &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      bootstrapSecretNamePrefix + "hub2",
						Namespace: "multicluster-engine",
					},
					Data: map[string][]byte{
						"test1": []byte(`payload`),
					},
				},
				ManagedClusters: []string{"cluster1"},
			},
			expectedProduceEvent: func() *cloudevents.Event {
				configs.SetAgentConfig(&configs.AgentConfig{LeafHubName: "hub1"})

				evt := cloudevents.NewEvent()
				evt.SetType(string(enum.ManagedClusterMigrationType))
				evt.SetSource("hub1")
				evt.SetExtension(constants.CloudEventExtensionKeyClusterName, "hub2")
				evt.SetExtension(eventversion.ExtVersion, "0.1")
				_ = evt.SetData(*cloudevents.StringOfApplicationCloudEventsJSON(), &migration.MigrationStatusBundle{
					Stage: migrationv1alpha1.ConditionTypeCleaned,
				})
				return &evt
			}(),
			expectedObjects: nil,
		},
		{
			name: "Rollback initializing: rollback cluster1 migration annotations",
			initObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster1",
						Annotations: map[string]string{
							constants.ManagedClusterMigrating: "global-hub.open-cluster-management.io/migrating",
							KlusterletConfigAnnotation:        "migration-hub2",
						},
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient:     true,
						LeaseDurationSeconds: 60,
					},
				},
				&mchv1.MultiClusterHub{
					ObjectMeta: metav1.ObjectMeta{
						Name: "multiclusterhub",
					},
					Spec: mchv1.MultiClusterHubSpec{},
					Status: mchv1.MultiClusterHubStatus{
						CurrentVersion: "2.14.0",
					},
				},
			},
			receivedMigrationEventBundle: migration.MigrationSourceBundle{
				MigrationId:     currentSyncerMigrationId,
				ToHub:           "hub2",
				Stage:           migrationv1alpha1.PhaseRollbacking,
				RollbackStage:   migrationv1alpha1.PhaseInitializing,
				ManagedClusters: []string{"cluster1"},
				BootstrapSecret: &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: bootstrapSecretNamePrefix + "hub2", Namespace: "multicluster-engine"},
					Type:       corev1.SecretTypeOpaque,
					StringData: map[string]string{"test": "secret"},
				},
			},
			expectedProduceEvent: func() *cloudevents.Event {
				configs.SetAgentConfig(&configs.AgentConfig{LeafHubName: "hub1"})

				evt := cloudevents.NewEvent()
				evt.SetType(string(enum.ManagedClusterMigrationType))
				evt.SetSource("hub1")
				evt.SetExtension(constants.CloudEventExtensionKeyClusterName, "global-hub")
				evt.SetExtension(eventversion.ExtVersion, "0.1")
				return &evt
			}(),
			expectedObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "cluster1",
						Annotations: map[string]string{},
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient:     true,
						LeaseDurationSeconds: 60,
					},
				},
			},
		},
		{
			name: "Registering: cluster already has HubAcceptsClient false - should skip",
			initObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster1",
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient:     false, // Already false
						LeaseDurationSeconds: 60,
					},
				},
				&mchv1.MultiClusterHub{
					ObjectMeta: metav1.ObjectMeta{
						Name: "multiclusterhub",
					},
					Spec: mchv1.MultiClusterHubSpec{},
					Status: mchv1.MultiClusterHubStatus{
						CurrentVersion: "2.14.0",
					},
				},
			},
			receivedMigrationEventBundle: migration.MigrationSourceBundle{
				MigrationId:     currentSyncerMigrationId,
				ToHub:           "hub2",
				Stage:           migrationv1alpha1.PhaseRegistering,
				ManagedClusters: []string{"cluster1"},
			},
			expectedProduceEvent: nil,
			expectedObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster1",
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient:     false, // Should remain false
						LeaseDurationSeconds: 60,
					},
				},
			},
		},
		{
			name: "Rollback registering: restore HubAcceptsClient to true and clean up",
			initObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster1",
						Annotations: map[string]string{
							constants.ManagedClusterMigrating: "global-hub.open-cluster-management.io/migrating",
							KlusterletConfigAnnotation:        "migration-hub2",
						},
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient:     false, // Set to false during registering
						LeaseDurationSeconds: 60,
					},
				},
				&mchv1.MultiClusterHub{
					ObjectMeta: metav1.ObjectMeta{
						Name: "multiclusterhub",
					},
					Spec: mchv1.MultiClusterHubSpec{},
					Status: mchv1.MultiClusterHubStatus{
						CurrentVersion: "2.14.0",
					},
				},
			},
			receivedMigrationEventBundle: migration.MigrationSourceBundle{
				MigrationId:     currentSyncerMigrationId,
				ToHub:           "hub2",
				Stage:           migrationv1alpha1.PhaseRollbacking,
				RollbackStage:   migrationv1alpha1.PhaseRegistering,
				ManagedClusters: []string{"cluster1"},
				BootstrapSecret: &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: bootstrapSecretNamePrefix + "hub2", Namespace: "multicluster-engine"},
					Type:       corev1.SecretTypeOpaque,
					StringData: map[string]string{"test": "secret"},
				},
			},
			expectedProduceEvent: func() *cloudevents.Event {
				configs.SetAgentConfig(&configs.AgentConfig{LeafHubName: "hub1"})

				evt := cloudevents.NewEvent()
				evt.SetType(string(enum.ManagedClusterMigrationType))
				evt.SetSource("hub1")
				evt.SetExtension(constants.CloudEventExtensionKeyClusterName, "global-hub")
				evt.SetExtension(eventversion.ExtVersion, "0.1")
				return &evt
			}(),
			expectedObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "cluster1",
						Annotations: map[string]string{}, // Annotations should be cleaned up
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient:     true, // Should be set to true during rollback
						LeaseDurationSeconds: 60,
					},
				},
			},
		},
		{
			name: "Rollback deploying: rollback cluster1 migration after deploy failure",
			initObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster1",
						Annotations: map[string]string{
							constants.ManagedClusterMigrating: "global-hub.open-cluster-management.io/migrating",
							KlusterletConfigAnnotation:        "migration-hub2",
						},
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient:     true,
						LeaseDurationSeconds: 60,
					},
				},
				&mchv1.MultiClusterHub{
					ObjectMeta: metav1.ObjectMeta{
						Name: "multiclusterhub",
					},
					Spec: mchv1.MultiClusterHubSpec{},
					Status: mchv1.MultiClusterHubStatus{
						CurrentVersion: "2.14.0",
					},
				},
			},
			receivedMigrationEventBundle: migration.MigrationSourceBundle{
				MigrationId:     currentSyncerMigrationId,
				ToHub:           "hub2",
				Stage:           migrationv1alpha1.PhaseRollbacking,
				RollbackStage:   migrationv1alpha1.PhaseDeploying,
				ManagedClusters: []string{"cluster1"},
				BootstrapSecret: &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: bootstrapSecretNamePrefix + "hub2", Namespace: "multicluster-engine"},
					Type:       corev1.SecretTypeOpaque,
					StringData: map[string]string{"test": "secret"},
				},
			},
			expectedProduceEvent: func() *cloudevents.Event {
				configs.SetAgentConfig(&configs.AgentConfig{LeafHubName: "hub1"})

				evt := cloudevents.NewEvent()
				evt.SetType(string(enum.ManagedClusterMigrationType))
				evt.SetSource("hub1")
				evt.SetExtension(constants.CloudEventExtensionKeyClusterName, "global-hub")
				evt.SetExtension(eventversion.ExtVersion, "0.1")
				return &evt
			}(),
			expectedObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "cluster1",
						Annotations: map[string]string{},
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient:     true,
						LeaseDurationSeconds: 60,
					},
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(c.initObjects...).WithObjects(
				c.initObjects...).Build()

			producer := ProducerMock{}
			transportClient := &controller.TransportClient{}
			transportClient.SetProducer(&producer)

			transportConfig := &transport.TransportInternalConfig{
				TransportType: string(transport.Chan),
				KafkaCredential: &transport.KafkaConfig{
					SpecTopic: "spec",
				},
			}

			agentConfig := &configs.AgentConfig{
				TransportConfig: transportConfig,
				LeafHubName:     "hub1",
			}
			managedClusterMigrationSyncer := NewMigrationSourceSyncer(fakeClient, nil, transportClient, agentConfig)
			managedClusterMigrationSyncer.processingMigrationId = currentSyncerMigrationId
			payload, err := json.Marshal(c.receivedMigrationEventBundle)
			assert.Nil(t, err)
			if err != nil {
				t.Errorf("Failed to marshal payload of managed cluster migration: %v", err)
			}

			// sync managed cluster migration
			evt := utils.ToCloudEvent(constants.MigrationTargetMsgKey, constants.CloudEventGlobalHubClusterName,
				"hub2", payload)
			err = managedClusterMigrationSyncer.Sync(ctx, &evt)
			if err != nil {
				t.Errorf("Failed to sync managed cluster migration: %v", err)
			}

			sentEvent := producer.sentEvent
			expectEvent := c.expectedProduceEvent
			if expectEvent != nil {
				assert.Equal(t, sentEvent.Type(), expectEvent.Type())
				assert.Equal(t, sentEvent.Type(), expectEvent.Type())
				assert.Equal(t, sentEvent.Source(), expectEvent.Source())
				assert.Equal(t, sentEvent.Extensions(), sentEvent.Extensions())
			}

			if c.expectedObjects != nil {
				for _, obj := range c.expectedObjects {
					runtimeObj := obj.DeepCopyObject().(client.Object)
					err = fakeClient.Get(ctx, client.ObjectKeyFromObject(obj), runtimeObj)
					assert.Nil(t, err)
					assert.True(t, apiequality.Semantic.DeepDerivative(obj, runtimeObj))
				}
			}
		})
	}
}

func TestGenerateKlusterletConfig(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add clientgoscheme to scheme: %v", err)
	}
	_ = mchv1.AddToScheme(scheme)
	cases := []struct {
		name                string
		mch                 *mchv1.MultiClusterHub
		targetHub           string
		initObjects         []client.Object
		bootstrapSecretName string
		expectedError       bool
		expected213         bool
	}{
		{
			name: "MCH version 2.13",
			mch: &mchv1.MultiClusterHub{
				Status: mchv1.MultiClusterHubStatus{
					CurrentVersion: "2.13.0",
				},
			},
			targetHub:           "hub2",
			bootstrapSecretName: "bootstrap-hub2",
			expectedError:       false,
			expected213:         true,
			initObjects: []client.Object{
				&mchv1.MultiClusterHub{
					ObjectMeta: metav1.ObjectMeta{
						Name: "multiclusterhub",
					},
					Spec: mchv1.MultiClusterHubSpec{},
					Status: mchv1.MultiClusterHubStatus{
						CurrentVersion: "2.13.0",
					},
				},
			},
		},
		{
			name: "MCH version 2.14",
			mch: &mchv1.MultiClusterHub{
				Status: mchv1.MultiClusterHubStatus{
					CurrentVersion: "2.14.1",
				},
			},
			targetHub:           "hub2",
			bootstrapSecretName: "bootstrap-hub2",
			expectedError:       false,
			expected213:         false,
			initObjects: []client.Object{
				&mchv1.MultiClusterHub{
					ObjectMeta: metav1.ObjectMeta{
						Name: "multiclusterhub",
					},
					Spec: mchv1.MultiClusterHubSpec{},
					Status: mchv1.MultiClusterHubStatus{
						CurrentVersion: "2.14.2",
					},
				},
			},
		},
		{
			name:                "No MCH found",
			mch:                 nil,
			targetHub:           "hub2",
			bootstrapSecretName: "bootstrap-hub2",
			expectedError:       true,
		},
		{
			name:                "No MCH status found",
			mch:                 nil,
			targetHub:           "hub2",
			bootstrapSecretName: "bootstrap-hub2",
			expectedError:       false,
			expected213:         false,
			initObjects: []client.Object{
				&mchv1.MultiClusterHub{
					ObjectMeta: metav1.ObjectMeta{
						Name: "multiclusterhub",
					},
					Spec: mchv1.MultiClusterHubSpec{},
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(c.initObjects...).WithObjects(
				c.initObjects...).Build()
			obj, err := generateKlusterletConfig(fakeClient, c.targetHub, c.bootstrapSecretName)
			if c.expectedError {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
				byte, _ := yaml.Marshal(obj)
				if c.expected213 {
					assert.True(t, !strings.Contains(string(byte), "multipleHubsConfig"))
				} else {
					assert.True(t, strings.Contains(string(byte), "multipleHubsConfig"))
				}
			}
		})
	}
}

type ProducerMock struct {
	sentEvent *cloudevents.Event
}

func (m *ProducerMock) SendEvent(ctx context.Context, evt cloudevents.Event) error {
	m.sentEvent = &evt
	return nil
}

func (m *ProducerMock) Reconnect(config *transport.TransportInternalConfig, topic string) error {
	return nil
}

// TestValidating tests the validating phase of migration
func TestValidating(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := clusterv1beta1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add clusterv1beta1 to scheme: %v", err)
	}
	if err := clusterv1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add clusterv1 to scheme: %v", err)
	}

	cases := []struct {
		name                 string
		migrationBundle      *migration.MigrationSourceBundle
		initObjects          []client.Object
		expectedClusters     []string
		expectedError        bool
		expectedErrorMessage string
	}{
		{
			name: "Should get clusters from placement decisions successfully",
			migrationBundle: &migration.MigrationSourceBundle{
				MigrationId:   "test-migration-123",
				PlacementName: "test-placement",
			},
			initObjects: []client.Object{
				&clusterv1beta1.PlacementDecision{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-placement-decision-1",
						Namespace: "default",
						Labels: map[string]string{
							clusterv1beta1.PlacementLabel: "test-placement",
						},
					},
					Status: clusterv1beta1.PlacementDecisionStatus{
						Decisions: []clusterv1beta1.ClusterDecision{
							{ClusterName: "cluster1"},
							{ClusterName: "cluster2"},
						},
					},
				},
				&clusterv1beta1.PlacementDecision{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-placement-decision-2",
						Namespace: "default",
						Labels: map[string]string{
							clusterv1beta1.PlacementLabel: "test-placement",
						},
					},
					Status: clusterv1beta1.PlacementDecisionStatus{
						Decisions: []clusterv1beta1.ClusterDecision{
							{ClusterName: "cluster3"},
						},
					},
				},
				// Add the ManagedCluster objects that the validation logic expects
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster1"},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient: true,
					},
					Status: clusterv1.ManagedClusterStatus{
						Conditions: []metav1.Condition{
							{
								Type:   "ManagedClusterConditionAvailable",
								Status: metav1.ConditionTrue,
							},
						},
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster2"},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient: true,
					},
					Status: clusterv1.ManagedClusterStatus{
						Conditions: []metav1.Condition{
							{
								Type:   "ManagedClusterConditionAvailable",
								Status: metav1.ConditionTrue,
							},
						},
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster3"},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient: true,
					},
					Status: clusterv1.ManagedClusterStatus{
						Conditions: []metav1.Condition{
							{
								Type:   "ManagedClusterConditionAvailable",
								Status: metav1.ConditionTrue,
							},
						},
					},
				},
			},
			expectedClusters: []string{"cluster1", "cluster2", "cluster3"},
			expectedError:    false,
		},
		{
			name: "Should handle empty placement decisions",
			migrationBundle: &migration.MigrationSourceBundle{
				MigrationId:   "test-migration-456",
				PlacementName: "empty-placement",
			},
			initObjects:      []client.Object{},
			expectedClusters: nil, // Returns nil when no placement decisions found
			expectedError:    true,
		},
		{
			name: "Should handle no placement name provided",
			migrationBundle: &migration.MigrationSourceBundle{
				MigrationId:   "test-migration-789",
				PlacementName: "",
			},
			initObjects:      []client.Object{},
			expectedClusters: nil,
			expectedError:    false,
		},
		{
			name: "Should filter out empty cluster names",
			migrationBundle: &migration.MigrationSourceBundle{
				MigrationId:   "test-migration-filter",
				PlacementName: "filter-placement",
			},
			initObjects: []client.Object{
				&clusterv1beta1.PlacementDecision{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "filter-placement-decision",
						Namespace: "default",
						Labels: map[string]string{
							clusterv1beta1.PlacementLabel: "filter-placement",
						},
					},
					Status: clusterv1beta1.PlacementDecisionStatus{
						Decisions: []clusterv1beta1.ClusterDecision{
							{ClusterName: "cluster1"},
							{ClusterName: ""}, // Empty cluster name should be filtered out
							{ClusterName: "cluster2"},
						},
					},
				},
				// Add the ManagedCluster objects that the validation logic expects
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster1"},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient: true,
					},
					Status: clusterv1.ManagedClusterStatus{
						Conditions: []metav1.Condition{
							{
								Type:   "ManagedClusterConditionAvailable",
								Status: metav1.ConditionTrue,
							},
						},
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster2"},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient: true,
					},
					Status: clusterv1.ManagedClusterStatus{
						Conditions: []metav1.Condition{
							{
								Type:   "ManagedClusterConditionAvailable",
								Status: metav1.ConditionTrue,
							},
						},
					},
				},
			},
			expectedClusters: []string{"cluster1", "cluster2"},
			expectedError:    false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(c.initObjects...).Build()

			syncer := &MigrationSourceSyncer{
				client:        fakeClient,
				clusterErrors: make(map[string]string),
			}

			// Create a copy of the migration bundle to avoid modifying the original
			bundle := &migration.MigrationSourceBundle{
				MigrationId:     c.migrationBundle.MigrationId,
				PlacementName:   c.migrationBundle.PlacementName,
				ManagedClusters: c.migrationBundle.ManagedClusters,
			}

			err := syncer.validating(ctx, bundle)

			if c.expectedError {
				assert.NotNil(t, err)
				if c.expectedErrorMessage != "" {
					assert.Contains(t, err.Error(), c.expectedErrorMessage)
				}
			} else {
				assert.Nil(t, err)
				if c.migrationBundle.PlacementName != "" {
					// When placement name is provided, ManagedClusters should be set to whatever getClustersFromPlacementDecisions returns
					assert.Equal(t, c.expectedClusters, bundle.ManagedClusters)
				} else {
					// When placement name is empty, ManagedClusters should remain unchanged
					assert.Equal(t, c.migrationBundle.ManagedClusters, bundle.ManagedClusters)
				}
			}
		})
	}
}

// TestGetClustersFromPlacementDecisions tests the getClustersFromPlacementDecisions function
func TestGetClustersFromPlacementDecisions(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()

	if err := clusterv1beta1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add clusterv1beta1 to scheme: %v", err)
	}

	cases := []struct {
		name                 string
		placementName        string
		initObjects          []client.Object
		expectedClusters     []string
		expectedError        bool
		expectedErrorMessage string
	}{
		{
			name:          "Should return clusters from multiple placement decisions",
			placementName: "test-placement",
			initObjects: []client.Object{
				&clusterv1beta1.PlacementDecision{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-placement-decision-1",
						Namespace: "default",
						Labels: map[string]string{
							clusterv1beta1.PlacementLabel: "test-placement",
						},
					},
					Status: clusterv1beta1.PlacementDecisionStatus{
						Decisions: []clusterv1beta1.ClusterDecision{
							{ClusterName: "cluster1"},
							{ClusterName: "cluster2"},
						},
					},
				},
				&clusterv1beta1.PlacementDecision{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-placement-decision-2",
						Namespace: "default",
						Labels: map[string]string{
							clusterv1beta1.PlacementLabel: "test-placement",
						},
					},
					Status: clusterv1beta1.PlacementDecisionStatus{
						Decisions: []clusterv1beta1.ClusterDecision{
							{ClusterName: "cluster3"},
						},
					},
				},
			},
			expectedClusters: []string{"cluster1", "cluster2", "cluster3"},
			expectedError:    false,
		},
		{
			name:             "Should return empty list when no placement decisions found",
			placementName:    "nonexistent-placement",
			initObjects:      []client.Object{},
			expectedClusters: nil, // Returns nil when no clusters found
			expectedError:    false,
		},
		{
			name:          "Should handle placement decisions with no clusters",
			placementName: "empty-placement",
			initObjects: []client.Object{
				&clusterv1beta1.PlacementDecision{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "empty-placement-decision",
						Namespace: "default",
						Labels: map[string]string{
							clusterv1beta1.PlacementLabel: "empty-placement",
						},
					},
					Status: clusterv1beta1.PlacementDecisionStatus{
						Decisions: []clusterv1beta1.ClusterDecision{},
					},
				},
			},
			expectedClusters: nil, // Returns nil when no clusters found
			expectedError:    false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(c.initObjects...).Build()

			syncer := &MigrationSourceSyncer{
				client:        fakeClient,
				clusterErrors: make(map[string]string),
			}

			clusters, err := syncer.getClustersFromPlacementDecisions(ctx, c.placementName)

			if c.expectedError {
				assert.NotNil(t, err)
				if c.expectedErrorMessage != "" {
					assert.Contains(t, err.Error(), c.expectedErrorMessage)
				}
			} else {
				assert.Nil(t, err)
				assert.Equal(t, c.expectedClusters, clusters)
			}
		})
	}
}

// TestValidateSingleCluster tests the validateSingleCluster function
func TestValidateSingleCluster(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := clusterv1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add clusterv1 to scheme: %v", err)
	}

	cases := []struct {
		name                 string
		clusterName          string
		managedCluster       *clusterv1.ManagedCluster
		expectedError        bool
		expectedErrorMessage string
		leafHubName          string
	}{
		{
			name:        "Should pass validation for valid cluster",
			clusterName: "valid-cluster",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "valid-cluster",
				},
				Spec: clusterv1.ManagedClusterSpec{
					HubAcceptsClient: true,
				},
				Status: clusterv1.ManagedClusterStatus{
					Conditions: []metav1.Condition{
						{
							Type:   clusterv1.ManagedClusterConditionAvailable,
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			expectedError: false,
			leafHubName:   "test-hub",
		},
		{
			name:                 "Should fail when cluster not found",
			clusterName:          "nonexistent-cluster",
			managedCluster:       nil,
			expectedError:        true,
			expectedErrorMessage: "managed cluster nonexistent-cluster is not found in managed hub test-hub",
			leafHubName:          "test-hub",
		},
		{
			name:        "Should fail for hosted cluster",
			clusterName: "hosted-cluster",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hosted-cluster",
					Annotations: map[string]string{
						constants.AnnotationClusterDeployMode: constants.ClusterDeployModeHosted,
					},
				},
				Spec: clusterv1.ManagedClusterSpec{
					HubAcceptsClient: true,
				},
				Status: clusterv1.ManagedClusterStatus{
					Conditions: []metav1.Condition{
						{
							Type:   clusterv1.ManagedClusterConditionAvailable,
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			expectedError:        true,
			expectedErrorMessage: "managed cluster hosted-cluster is imported as hosted mode in managed hub test-hub, it cannot be migrated",
			leafHubName:          "test-hub",
		},
		{
			name:        "Should fail for local cluster",
			clusterName: "local-cluster",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "local-cluster",
					Labels: map[string]string{
						constants.LocalClusterName: "true",
					},
				},
				Spec: clusterv1.ManagedClusterSpec{
					HubAcceptsClient: true,
				},
				Status: clusterv1.ManagedClusterStatus{
					Conditions: []metav1.Condition{
						{
							Type:   clusterv1.ManagedClusterConditionAvailable,
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			expectedError:        true,
			expectedErrorMessage: "managed cluster local-cluster is local cluster in managed hub test-hub, it cannot be migrated",
			leafHubName:          "test-hub",
		},
		{
			name:        "Should fail for unavailable cluster",
			clusterName: "unavailable-cluster",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "unavailable-cluster",
				},
				Spec: clusterv1.ManagedClusterSpec{
					HubAcceptsClient: true,
				},
				Status: clusterv1.ManagedClusterStatus{
					Conditions: []metav1.Condition{
						{
							Type:   clusterv1.ManagedClusterConditionAvailable,
							Status: metav1.ConditionFalse,
						},
					},
				},
			},
			expectedError:        true,
			expectedErrorMessage: "managed cluster unavailable-cluster is not available in managed hub test-hub, it cannot be migrated",
			leafHubName:          "test-hub",
		},
		{
			name:        "Should fail for managed hub cluster",
			clusterName: "managed-hub-cluster",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "managed-hub-cluster",
					Annotations: map[string]string{
						constants.AnnotationONMulticlusterHub: "true",
					},
				},
				Spec: clusterv1.ManagedClusterSpec{
					HubAcceptsClient: true,
				},
				Status: clusterv1.ManagedClusterStatus{
					Conditions: []metav1.Condition{
						{
							Type:   clusterv1.ManagedClusterConditionAvailable,
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			expectedError:        true,
			expectedErrorMessage: "managed cluster managed-hub-cluster is a managed hub cluster in managed hub test-hub, it cannot be migrated",
			leafHubName:          "test-hub",
		},
		{
			name:        "Should fail for cluster with no conditions",
			clusterName: "no-conditions-cluster",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "no-conditions-cluster",
				},
				Spec: clusterv1.ManagedClusterSpec{
					HubAcceptsClient: true,
				},
				Status: clusterv1.ManagedClusterStatus{
					Conditions: []metav1.Condition{},
				},
			},
			expectedError:        true,
			expectedErrorMessage: "managed cluster no-conditions-cluster is not available in managed hub test-hub, it cannot be migrated",
			leafHubName:          "test-hub",
		},
		{
			name:        "Should pass for cluster with no annotations",
			clusterName: "no-annotations-cluster",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "no-annotations-cluster",
				},
				Spec: clusterv1.ManagedClusterSpec{
					HubAcceptsClient: true,
				},
				Status: clusterv1.ManagedClusterStatus{
					Conditions: []metav1.Condition{
						{
							Type:   clusterv1.ManagedClusterConditionAvailable,
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			expectedError: false,
			leafHubName:   "test-hub",
		},
		{
			name:        "Should pass for cluster with no labels",
			clusterName: "no-labels-cluster",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "no-labels-cluster",
				},
				Spec: clusterv1.ManagedClusterSpec{
					HubAcceptsClient: true,
				},
				Status: clusterv1.ManagedClusterStatus{
					Conditions: []metav1.Condition{
						{
							Type:   clusterv1.ManagedClusterConditionAvailable,
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			expectedError: false,
			leafHubName:   "test-hub",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var initObjects []client.Object
			if c.managedCluster != nil {
				initObjects = append(initObjects, c.managedCluster)
			}

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

			syncer := &MigrationSourceSyncer{
				client:      fakeClient,
				leafHubName: c.leafHubName,
			}

			err := syncer.validateSingleCluster(ctx, c.clusterName)

			if c.expectedError {
				assert.NotNil(t, err)
				if c.expectedErrorMessage != "" {
					assert.Contains(t, err.Error(), c.expectedErrorMessage)
				}
			} else {
				assert.Nil(t, err)
			}
		})
	}
}
