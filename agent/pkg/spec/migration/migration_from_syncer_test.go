// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package migration

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	klusterletv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	addonv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
					Status: clusterv1.ManagedClusterStatus{
						Conditions: []metav1.Condition{
							{
								Type:   clusterv1.ManagedClusterConditionAvailable,
								Status: metav1.ConditionTrue,
							},
						},
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
			evt.SetTime(time.Now()) // Set event time to avoid time-based skipping in shouldSkipMigrationEvent
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
	sentEvent     *cloudevents.Event
	SendEventFunc func(ctx context.Context, evt cloudevents.Event) error
	ReconnectFunc func(config *transport.TransportInternalConfig, topic string) error
}

func (m *ProducerMock) SendEvent(ctx context.Context, evt cloudevents.Event) error {
	if m.SendEventFunc != nil {
		return m.SendEventFunc(ctx, evt)
	}
	m.sentEvent = &evt
	return nil
}

func (m *ProducerMock) Reconnect(config *transport.TransportInternalConfig, topic string) error {
	if m.ReconnectFunc != nil {
		return m.ReconnectFunc(config, topic)
	}
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

// TestPrepareUnstructuredResourceForMigration tests the prepareUnstructuredResourceForMigration function
func TestPrepareUnstructuredResourceForMigration(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add corev1 to scheme: %v", err)
	}
	if err := clusterv1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add clusterv1 to scheme: %v", err)
	}

	cases := []struct {
		name             string
		clusterName      string
		migrateResource  MigrationResource
		initObjects      []client.Object
		expectedCount    int
		expectedError    bool
		expectedErrorMsg string
	}{
		{
			name:        "Should get specific resource by name",
			clusterName: "cluster1",
			migrateResource: MigrationResource{
				name: "<CLUSTER_NAME>-admin-password",
				gvk: schema.GroupVersionKind{
					Group:   "",
					Version: "v1",
					Kind:    "Secret",
				},
				needStatus: false,
			},
			initObjects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "cluster1-admin-password",
						Namespace:       "cluster1",
						ResourceVersion: "12345",
						UID:             "test-uid",
					},
					Data: map[string][]byte{
						"password": []byte("secret"),
					},
				},
			},
			expectedCount: 1,
			expectedError: false,
		},
		{
			name:        "Should return empty when specific resource not found",
			clusterName: "cluster1",
			migrateResource: MigrationResource{
				name: "<CLUSTER_NAME>-admin-password",
				gvk: schema.GroupVersionKind{
					Group:   "",
					Version: "v1",
					Kind:    "Secret",
				},
				needStatus: false,
			},
			initObjects:   []client.Object{},
			expectedCount: 0,
			expectedError: false,
		},
		{
			name:        "Should list all resources in namespace when name not specified",
			clusterName: "cluster1",
			migrateResource: MigrationResource{
				name: "",
				gvk: schema.GroupVersionKind{
					Group:   "",
					Version: "v1",
					Kind:    "ConfigMap",
				},
				needStatus: false,
			},
			initObjects: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config1",
						Namespace: "cluster1",
					},
					Data: map[string]string{"key": "value1"},
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config2",
						Namespace: "cluster1",
					},
					Data: map[string]string{"key": "value2"},
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config3",
						Namespace: "other-namespace",
					},
					Data: map[string]string{"key": "value3"},
				},
			},
			expectedCount: 2,
			expectedError: false,
		},
		{
			name:        "Should filter resources by annotation key",
			clusterName: "cluster1",
			migrateResource: MigrationResource{
				name:          "",
				annotationKey: "siteconfig.open-cluster-management.io/preserve",
				gvk: schema.GroupVersionKind{
					Group:   "",
					Version: "v1",
					Kind:    "Secret",
				},
				needStatus: false,
			},
			initObjects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret1",
						Namespace: "cluster1",
						Annotations: map[string]string{
							"siteconfig.open-cluster-management.io/preserve": "true",
						},
					},
					Data: map[string][]byte{"key": []byte("value1")},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret2",
						Namespace: "cluster1",
					},
					Data: map[string][]byte{"key": []byte("value2")},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret3",
						Namespace: "cluster1",
						Annotations: map[string]string{
							"siteconfig.open-cluster-management.io/preserve": "false",
						},
					},
					Data: map[string][]byte{"key": []byte("value3")},
				},
			},
			expectedCount: 2, // secret1 and secret3 have the annotation key
			expectedError: false,
		},
		{
			name:        "Should return empty when no resources match annotation key",
			clusterName: "cluster1",
			migrateResource: MigrationResource{
				name:          "",
				annotationKey: "non-existent-annotation",
				gvk: schema.GroupVersionKind{
					Group:   "",
					Version: "v1",
					Kind:    "ConfigMap",
				},
				needStatus: false,
			},
			initObjects: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config1",
						Namespace: "cluster1",
					},
					Data: map[string]string{"key": "value1"},
				},
			},
			expectedCount: 0,
			expectedError: false,
		},
		{
			name:        "Should clean metadata fields for migration",
			clusterName: "cluster1",
			migrateResource: MigrationResource{
				name: "<CLUSTER_NAME>-admin-password",
				gvk: schema.GroupVersionKind{
					Group:   "",
					Version: "v1",
					Kind:    "Secret",
				},
				needStatus: false,
			},
			initObjects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "cluster1-admin-password",
						Namespace:       "cluster1",
						ResourceVersion: "12345",
						Generation:      5,
						UID:             "test-uid",
						Finalizers:      []string{"finalizer1"},
						Annotations: map[string]string{
							kubectlConfigAnnotation: "kubectl-config",
							"other-annotation":      "value",
						},
					},
					Data: map[string][]byte{
						"password": []byte("secret"),
					},
				},
			},
			expectedCount: 1,
			expectedError: false,
		},
		{
			name:        "Should preserve status when needStatus is true",
			clusterName: "cluster1",
			migrateResource: MigrationResource{
				name: "<CLUSTER_NAME>",
				gvk: schema.GroupVersionKind{
					Group:   "hive.openshift.io",
					Version: "v1",
					Kind:    "ClusterDeployment",
				},
				needStatus: true,
			},
			initObjects: []client.Object{
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "hive.openshift.io/v1",
						"kind":       "ClusterDeployment",
						"metadata": map[string]interface{}{
							"name":      "cluster1",
							"namespace": "cluster1",
							"annotations": map[string]interface{}{
								"hive.openshift.io/reconcile-pause": "true",
							},
						},
						"spec": map[string]interface{}{
							"clusterName": "cluster1",
						},
						"status": map[string]interface{}{
							"conditions": []interface{}{
								map[string]interface{}{
									"type":   "Ready",
									"status": "True",
								},
							},
						},
					},
				},
			},
			expectedCount: 1,
			expectedError: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(c.initObjects...).Build()

			syncer := &MigrationSourceSyncer{
				client: fakeClient,
			}

			resources, err := prepareUnstructuredResourceForMigration(
				ctx, fakeClient, c.clusterName, c.migrateResource)

			// Clean resources after collection
			for i := range resources {
				cleanResourceForMigration(&resources[i], c.migrateResource,
					syncer.cleanObjectMetadata, syncer.processResourceByType)
			}

			if c.expectedError {
				assert.NotNil(t, err)
				if c.expectedErrorMsg != "" {
					assert.Contains(t, err.Error(), c.expectedErrorMsg)
				}
			} else {
				assert.Nil(t, err)
				assert.Equal(t, c.expectedCount, len(resources))

				// Verify resources are properly cleaned
				for _, res := range resources {
					// Metadata should be cleaned
					assert.Empty(t, res.GetResourceVersion(), "ResourceVersion should be cleared")
					assert.Empty(t, res.GetFinalizers(), "Finalizers should be cleared")
					assert.Equal(t, int64(0), res.GetGeneration(), "Generation should be 0")

					// kubectl last-applied-configuration annotation should be removed
					annotations := res.GetAnnotations()
					if annotations != nil {
						_, exists := annotations[kubectlConfigAnnotation]
						assert.False(t, exists, "kubectl last-applied-configuration should be removed")
					}

					// Check status based on needStatus
					_, hasStatus, _ := unstructured.NestedFieldCopy(res.Object, "status")
					if c.migrateResource.needStatus {
						if len(c.initObjects) > 0 {
							// If init object has status, it should be preserved
							initObj := c.initObjects[0]
							if unstructuredObj, ok := initObj.(*unstructured.Unstructured); ok {
								_, initHasStatus, _ := unstructured.NestedFieldCopy(unstructuredObj.Object, "status")
								if initHasStatus {
									assert.True(t, hasStatus, "Status should be preserved when needStatus is true")
								}
							}
						}
					} else {
						assert.False(t, hasStatus, "Status should be removed when needStatus is false")
					}

					// Note: For ClusterDeployment resources, the hive.openshift.io/reconcile-pause
					// annotation is preserved during migration and removed only during rollback
					// or cleanup on the target hub
				}
			}
		})
	}
}

func TestCollectReferencedResources(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add corev1 to scheme: %v", err)
	}

	cases := []struct {
		name                    string
		clusterName             string
		resourcesList           []unstructured.Unstructured
		initObjects             []client.Object
		expectedSecretsCount    int
		expectedConfigMapsCount int
		expectedSecretNames     []string
		expectedConfigMapNames  []string
	}{
		{
			name:        "Should collect BMC credentials secret from BareMetalHost",
			clusterName: "cluster1",
			resourcesList: []unstructured.Unstructured{
				createBareMetalHost("bmh1", "cluster1", "bmc-secret-1"),
			},
			initObjects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "bmc-secret-1",
						Namespace: "cluster1",
					},
					Data: map[string][]byte{
						"username": []byte("admin"),
						"password": []byte("password123"),
					},
				},
			},
			expectedSecretsCount:    1,
			expectedConfigMapsCount: 0,
			expectedSecretNames:     []string{"bmc-secret-1"},
		},
		{
			name:        "Should collect pull secret from ClusterDeployment",
			clusterName: "cluster1",
			resourcesList: []unstructured.Unstructured{
				createClusterDeployment("cluster1", "cluster1", "pull-secret-1"),
			},
			initObjects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pull-secret-1",
						Namespace: "cluster1",
					},
					Type: corev1.SecretTypeDockerConfigJson,
					Data: map[string][]byte{
						".dockerconfigjson": []byte(`{"auths":{}}`),
					},
				},
			},
			expectedSecretsCount:    1,
			expectedConfigMapsCount: 0,
			expectedSecretNames:     []string{"pull-secret-1"},
		},
		{
			name:        "Should collect configmaps from ImageClusterInstall",
			clusterName: "cluster1",
			resourcesList: []unstructured.Unstructured{
				createImageClusterInstall("ici1", "cluster1", []string{"extra-manifest-1", "extra-manifest-2"}),
			},
			initObjects: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "extra-manifest-1",
						Namespace: "cluster1",
					},
					Data: map[string]string{
						"manifest.yaml": "apiVersion: v1\nkind: ConfigMap",
					},
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "extra-manifest-2",
						Namespace: "cluster1",
					},
					Data: map[string]string{
						"manifest.yaml": "apiVersion: v1\nkind: Service",
					},
				},
			},
			expectedSecretsCount:    0,
			expectedConfigMapsCount: 2,
			expectedConfigMapNames:  []string{"extra-manifest-1", "extra-manifest-2"},
		},
		{
			name:        "Should collect multiple resource types together",
			clusterName: "cluster1",
			resourcesList: []unstructured.Unstructured{
				createBareMetalHost("bmh1", "cluster1", "bmc-secret"),
				createClusterDeployment("cluster1", "cluster1", "pull-secret"),
				createImageClusterInstall("ici1", "cluster1", []string{"extra-manifest"}),
			},
			initObjects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "bmc-secret",
						Namespace: "cluster1",
					},
					Data: map[string][]byte{"username": []byte("admin")},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pull-secret",
						Namespace: "cluster1",
					},
					Data: map[string][]byte{".dockerconfigjson": []byte("{}")},
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "extra-manifest",
						Namespace: "cluster1",
					},
					Data: map[string]string{"manifest.yaml": "test"},
				},
			},
			expectedSecretsCount:    2,
			expectedConfigMapsCount: 1,
			expectedSecretNames:     []string{"bmc-secret", "pull-secret"},
			expectedConfigMapNames:  []string{"extra-manifest"},
		},
		{
			name:        "Should deduplicate secrets with same name",
			clusterName: "cluster1",
			resourcesList: []unstructured.Unstructured{
				createBareMetalHost("bmh1", "cluster1", "shared-secret"),
				createBareMetalHost("bmh2", "cluster1", "shared-secret"),
			},
			initObjects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "shared-secret",
						Namespace: "cluster1",
					},
					Data: map[string][]byte{"username": []byte("admin")},
				},
			},
			expectedSecretsCount:    1,
			expectedConfigMapsCount: 0,
			expectedSecretNames:     []string{"shared-secret"},
		},
		{
			name:        "Should handle missing secrets gracefully",
			clusterName: "cluster1",
			resourcesList: []unstructured.Unstructured{
				createBareMetalHost("bmh1", "cluster1", "non-existent-secret"),
			},
			initObjects:             []client.Object{},
			expectedSecretsCount:    0,
			expectedConfigMapsCount: 0,
		},
		{
			name:        "Should handle empty extraManifestsRefs",
			clusterName: "cluster1",
			resourcesList: []unstructured.Unstructured{
				createImageClusterInstall("ici1", "cluster1", []string{}),
			},
			initObjects:             []client.Object{},
			expectedSecretsCount:    0,
			expectedConfigMapsCount: 0,
		},
		{
			name:        "Should handle resources without references",
			clusterName: "cluster1",
			resourcesList: []unstructured.Unstructured{
				createBareMetalHostWithoutCredentials("bmh1", "cluster1"),
				createClusterDeploymentWithoutPullSecret("cluster1", "cluster1"),
			},
			initObjects:             []client.Object{},
			expectedSecretsCount:    0,
			expectedConfigMapsCount: 0,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(c.initObjects...).Build()

			syncer := &MigrationSourceSyncer{
				client: fakeClient,
			}

			referencedResources, err := syncer.collectReferencedResources(ctx, c.clusterName, c.resourcesList)

			assert.NoError(t, err)

			secretCount := 0
			configMapCount := 0
			secretNames := []string{}
			configMapNames := []string{}

			for _, res := range referencedResources {
				switch res.GetKind() {
				case "Secret":
					secretCount++
					secretNames = append(secretNames, res.GetName())
				case "ConfigMap":
					configMapCount++
					configMapNames = append(configMapNames, res.GetName())
				}
			}

			assert.Equal(t, c.expectedSecretsCount, secretCount, "Secret count mismatch")
			assert.Equal(t, c.expectedConfigMapsCount, configMapCount, "ConfigMap count mismatch")

			if c.expectedSecretNames != nil {
				assert.ElementsMatch(t, c.expectedSecretNames, secretNames, "Secret names mismatch")
			}
			if c.expectedConfigMapNames != nil {
				assert.ElementsMatch(t, c.expectedConfigMapNames, configMapNames, "ConfigMap names mismatch")
			}
		})
	}
}

// TestCollectReferencedResourcesOrder tests that referenced resources appear before main resources
func TestCollectReferencedResourcesOrder(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add corev1 to scheme: %v", err)
	}

	clusterName := "cluster1"

	// Create test resources in the order they would be collected
	mainResources := []unstructured.Unstructured{
		createBareMetalHost("bmh1", clusterName, "bmc-secret"),
		createClusterDeployment(clusterName, clusterName, "pull-secret"),
	}

	initObjects := []client.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "bmc-secret",
				Namespace: clusterName,
			},
			Data: map[string][]byte{"username": []byte("admin")},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pull-secret",
				Namespace: clusterName,
			},
			Data: map[string][]byte{".dockerconfigjson": []byte("{}")},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

	syncer := &MigrationSourceSyncer{
		client: fakeClient,
	}

	// Collect referenced resources
	referencedResources, err := syncer.collectReferencedResources(ctx, clusterName, mainResources)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(referencedResources), "Should collect 2 secrets")

	// Simulate what deploying() does: append referenced resources before main resources
	finalResourcesList := append(referencedResources, mainResources...)

	// Verify order: secrets first, then main resources
	assert.Equal(t, 4, len(finalResourcesList), "Should have 2 secrets + 2 main resources")

	// First two should be secrets
	assert.Equal(t, "Secret", finalResourcesList[0].GetKind(), "First resource should be Secret")
	assert.Equal(t, "Secret", finalResourcesList[1].GetKind(), "Second resource should be Secret")

	// Last two should be main resources
	assert.Equal(t, "BareMetalHost", finalResourcesList[2].GetKind(), "Third resource should be BareMetalHost")
	assert.Equal(t, "ClusterDeployment", finalResourcesList[3].GetKind(), "Fourth resource should be ClusterDeployment")

	// Verify secret names are in the list (order may vary)
	secretNames := []string{}
	for i := 0; i < 2; i++ {
		secretNames = append(secretNames, finalResourcesList[i].GetName())
	}
	assert.ElementsMatch(t, []string{"bmc-secret", "pull-secret"}, secretNames, "Should have both secrets")
}

// TestGetSecret tests the getSecret function
func TestGetSecret(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add corev1 to scheme: %v", err)
	}

	cases := []struct {
		name         string
		namespace    string
		secretName   string
		initObjects  []client.Object
		expectNil    bool
		expectError  bool
		validateFunc func(*testing.T, *unstructured.Unstructured)
	}{
		{
			name:       "Should retrieve and convert secret successfully",
			namespace:  "cluster1",
			secretName: "test-secret",
			initObjects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "test-secret",
						Namespace:       "cluster1",
						ResourceVersion: "12345",
						UID:             "test-uid",
						Labels: map[string]string{
							"app": "test",
						},
						Annotations: map[string]string{
							"description": "test secret",
						},
					},
					Type: corev1.SecretTypeOpaque,
					Data: map[string][]byte{
						"username": []byte("admin"),
						"password": []byte("secret123"),
					},
				},
			},
			expectNil:   false,
			expectError: false,
			validateFunc: func(t *testing.T, secret *unstructured.Unstructured) {
				assert.Equal(t, "Secret", secret.GetKind())
				assert.Equal(t, "v1", secret.GetAPIVersion())
				assert.Equal(t, "test-secret", secret.GetName())
				assert.Equal(t, "cluster1", secret.GetNamespace())
				assert.Equal(t, "test", secret.GetLabels()["app"])
				assert.Equal(t, "test secret", secret.GetAnnotations()["description"])

				// Verify metadata is cleaned (no ResourceVersion, UID, etc.)
				assert.Empty(t, secret.GetResourceVersion())
				assert.Empty(t, secret.GetUID())
				assert.Empty(t, secret.GetFinalizers())
				assert.Empty(t, secret.GetOwnerReferences())

				// Verify data exists
				data := secret.Object["data"]
				assert.NotNil(t, data, "data field should exist")

				// Verify type exists
				secretType := secret.Object["type"]
				assert.NotNil(t, secretType, "type field should exist")
			},
		},
		{
			name:        "Should return nil for non-existent secret",
			namespace:   "cluster1",
			secretName:  "non-existent",
			initObjects: []client.Object{},
			expectNil:   true,
			expectError: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(c.initObjects...).Build()

			syncer := &MigrationSourceSyncer{
				client: fakeClient,
			}

			secret, err := syncer.getSecret(ctx, c.namespace, c.secretName)

			if c.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if c.expectNil {
				assert.Nil(t, secret)
			} else {
				assert.NotNil(t, secret)
				if c.validateFunc != nil {
					c.validateFunc(t, secret)
				}
			}
		})
	}
}

// TestGetConfigMap tests the getConfigMap function
func TestGetConfigMap(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add corev1 to scheme: %v", err)
	}

	cases := []struct {
		name          string
		namespace     string
		configMapName string
		initObjects   []client.Object
		expectNil     bool
		expectError   bool
		validateFunc  func(*testing.T, *unstructured.Unstructured)
	}{
		{
			name:          "Should retrieve and convert configmap with data successfully",
			namespace:     "cluster1",
			configMapName: "test-configmap",
			initObjects: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "test-configmap",
						Namespace:       "cluster1",
						ResourceVersion: "12345",
						UID:             "test-uid",
						Labels: map[string]string{
							"app": "test",
						},
						Annotations: map[string]string{
							"description": "test configmap",
						},
					},
					Data: map[string]string{
						"config.yaml": "key: value",
						"script.sh":   "#!/bin/bash\necho hello",
					},
				},
			},
			expectNil:   false,
			expectError: false,
			validateFunc: func(t *testing.T, cm *unstructured.Unstructured) {
				assert.Equal(t, "ConfigMap", cm.GetKind())
				assert.Equal(t, "v1", cm.GetAPIVersion())
				assert.Equal(t, "test-configmap", cm.GetName())
				assert.Equal(t, "cluster1", cm.GetNamespace())
				assert.Equal(t, "test", cm.GetLabels()["app"])
				assert.Equal(t, "test configmap", cm.GetAnnotations()["description"])

				// Verify metadata is cleaned
				assert.Empty(t, cm.GetResourceVersion())
				assert.Empty(t, cm.GetUID())
				assert.Empty(t, cm.GetFinalizers())
				assert.Empty(t, cm.GetOwnerReferences())

				// Verify data exists
				data := cm.Object["data"]
				assert.NotNil(t, data, "data field should exist")
			},
		},
		{
			name:          "Should retrieve configmap with binaryData successfully",
			namespace:     "cluster1",
			configMapName: "binary-configmap",
			initObjects: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "binary-configmap",
						Namespace: "cluster1",
					},
					BinaryData: map[string][]byte{
						"binary.dat": {0x00, 0x01, 0x02, 0x03},
					},
				},
			},
			expectNil:   false,
			expectError: false,
			validateFunc: func(t *testing.T, cm *unstructured.Unstructured) {
				assert.Equal(t, "ConfigMap", cm.GetKind())

				// Verify binaryData exists
				binaryData := cm.Object["binaryData"]
				assert.NotNil(t, binaryData, "binaryData field should exist")
			},
		},
		{
			name:          "Should retrieve configmap with both data and binaryData",
			namespace:     "cluster1",
			configMapName: "mixed-configmap",
			initObjects: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "mixed-configmap",
						Namespace: "cluster1",
					},
					Data: map[string]string{
						"text.txt": "hello",
					},
					BinaryData: map[string][]byte{
						"binary.dat": {0xFF, 0xFE},
					},
				},
			},
			expectNil:   false,
			expectError: false,
			validateFunc: func(t *testing.T, cm *unstructured.Unstructured) {
				assert.Equal(t, "ConfigMap", cm.GetKind())

				// Verify data exists
				data := cm.Object["data"]
				assert.NotNil(t, data, "data field should exist")

				// Verify binaryData exists
				binaryData := cm.Object["binaryData"]
				assert.NotNil(t, binaryData, "binaryData field should exist")
			},
		},
		{
			name:          "Should return nil for non-existent configmap",
			namespace:     "cluster1",
			configMapName: "non-existent",
			initObjects:   []client.Object{},
			expectNil:     true,
			expectError:   false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(c.initObjects...).Build()

			syncer := &MigrationSourceSyncer{
				client: fakeClient,
			}

			configMap, err := syncer.getConfigMap(ctx, c.namespace, c.configMapName)

			if c.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if c.expectNil {
				assert.Nil(t, configMap)
			} else {
				assert.NotNil(t, configMap)
				if c.validateFunc != nil {
					c.validateFunc(t, configMap)
				}
			}
		})
	}
}

// Helper functions to create test resources

func createBareMetalHost(name, namespace, credentialsName string) unstructured.Unstructured {
	bmh := &unstructured.Unstructured{}
	bmh.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "metal3.io",
		Version: "v1alpha1",
		Kind:    "BareMetalHost",
	})
	bmh.SetName(name)
	bmh.SetNamespace(namespace)
	_ = unstructured.SetNestedField(bmh.Object, credentialsName, "spec", "bmc", "credentialsName")
	return *bmh
}

func createBareMetalHostWithoutCredentials(name, namespace string) unstructured.Unstructured {
	bmh := &unstructured.Unstructured{}
	bmh.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "metal3.io",
		Version: "v1alpha1",
		Kind:    "BareMetalHost",
	})
	bmh.SetName(name)
	bmh.SetNamespace(namespace)
	return *bmh
}

func createClusterDeployment(name, namespace, pullSecretName string) unstructured.Unstructured {
	cd := &unstructured.Unstructured{}
	cd.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "hive.openshift.io",
		Version: "v1",
		Kind:    "ClusterDeployment",
	})
	cd.SetName(name)
	cd.SetNamespace(namespace)
	_ = unstructured.SetNestedField(cd.Object, pullSecretName, "spec", "provisioning", "pullSecretRef", "name")
	return *cd
}

func createClusterDeploymentWithoutPullSecret(name, namespace string) unstructured.Unstructured {
	cd := &unstructured.Unstructured{}
	cd.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "hive.openshift.io",
		Version: "v1",
		Kind:    "ClusterDeployment",
	})
	cd.SetName(name)
	cd.SetNamespace(namespace)
	return *cd
}

func createImageClusterInstall(name, namespace string, configMapNames []string) unstructured.Unstructured {
	ici := &unstructured.Unstructured{}
	ici.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "extensions.hive.openshift.io",
		Version: "v1alpha1",
		Kind:    "ImageClusterInstall",
	})
	ici.SetName(name)
	ici.SetNamespace(namespace)

	if len(configMapNames) > 0 {
		refs := make([]interface{}, len(configMapNames))
		for i, cmName := range configMapNames {
			refs[i] = map[string]interface{}{
				"name": cmName,
			}
		}
		_ = unstructured.SetNestedSlice(ici.Object, refs, "spec", "extraManifestsRefs")
	}

	return *ici
}
