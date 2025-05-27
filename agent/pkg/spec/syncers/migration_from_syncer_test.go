// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package syncers

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

	cases := []struct {
		name                         string
		receivedMigrationEventBundle migration.ManagedClusterMigrationFromEvent
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
			receivedMigrationEventBundle: migration.ManagedClusterMigrationFromEvent{
				MigrationId: "020340324302432049234023040320",
				ToHub:       "hub2",
				Stage:       migrationv1alpha1.PhaseInitializing,
				BootstrapSecret: &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: bootstrapSecretNamePrefix + "hub2", Namespace: "multicluster-engine"},
				},
				ManagedClusters: []string{"cluster1"},
				Resources:       []string{"configmap/cluster1/test-configmap", "secret/cluster1/test-secret"},
			},
			expectedProduceEvent: func() *cloudevents.Event {
				configs.SetAgentConfig(&configs.AgentConfig{LeafHubName: "hub1"})

				evt := cloudevents.NewEvent()
				evt.SetType(constants.MigrationTargetMsgKey) // spec message: initialize -> deploy
				evt.SetSource("hub1")
				evt.SetExtension(constants.CloudEventExtensionKeyClusterName, "hub2")
				evt.SetExtension(eventversion.ExtVersion, "0.1")
				return &evt
			}(),
		},
		{
			name: "Registering: register cluster1 to hub2",
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
			receivedMigrationEventBundle: migration.ManagedClusterMigrationFromEvent{
				MigrationId:     "020340324302432049234023040320",
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
			receivedMigrationEventBundle: migration.ManagedClusterMigrationFromEvent{
				MigrationId: "020340324302432049234023040320",
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
				_ = evt.SetData(*cloudevents.StringOfApplicationCloudEventsJSON(), &migration.ManagedClusterMigrationBundle{
					Stage: migrationv1alpha1.ConditionTypeCleaned,
				})
				return &evt
			}(),
			expectedObjects: nil,
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

			managedClusterMigrationSyncer := NewMigrationSourceSyncer(fakeClient, nil, transportClient,
				transportConfig)

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
				// fmt.Println(sentEvent)
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
