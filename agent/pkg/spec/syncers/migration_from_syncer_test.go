// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package syncers

import (
	"context"
	"encoding/json"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	klusterletv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	addonv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
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
)

// go test -run ^TestMigrationSourceHubSyncer$ github.com/stolostron/multicluster-global-hub/agent/pkg/spec/syncers -v
func TestMigrationSourceHubSyncer(t *testing.T) {
	sleepForApplying = 1

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
			},
			receivedMigrationEventBundle: migration.ManagedClusterMigrationFromEvent{
				ToHub:           "hub2",
				Stage:           migrationv1alpha1.ConditionTypeInitialized,
				ManagedClusters: []string{"cluster1"},
			},
			expectedProduceEvent: func() *cloudevents.Event {
				configs.SetAgentConfig(&configs.AgentConfig{LeafHubName: "hub1"})

				evt := cloudevents.NewEvent()
				evt.SetType(string(enum.ManagedClusterMigrationType))
				evt.SetSource("hub1")
				evt.SetExtension(constants.CloudEventExtensionKeyClusterName, "hub2")
				evt.SetExtension(eventversion.ExtVersion, "0.1")
				return &evt
			}(),
		},
		{
			name: "Migrating: migrate cluster1 from hub1 to hub2",
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
			},
			receivedMigrationEventBundle: migration.ManagedClusterMigrationFromEvent{
				ToHub: "hub2",
				Stage: migrationv1alpha1.ConditionTypeRegistered,
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
			expectedProduceEvent: nil,
			expectedObjects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      bootstrapSecretNamePrefix + "hub2",
						Namespace: "multicluster-engine",
					},
				},
				&klusterletv1alpha1.KlusterletConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name: klusterletConfigNamePrefix + "hub2",
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster1",
						// Namespace: "cluster1",
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
			},
			receivedMigrationEventBundle: migration.ManagedClusterMigrationFromEvent{
				ToHub: "hub2",
				Stage: migrationv1alpha1.ConditionTypeCleaned,
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
				evt.SetData(*cloudevents.StringOfApplicationCloudEventsJSON(), &migration.ManagedClusterMigrationBundle{
					Stage:           migrationv1alpha1.ConditionTypeCleaned,
					ManagedClusters: []string{"cluster1"},
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

			managedClusterMigrationSyncer := NewManagedClusterMigrationFromSyncer(fakeClient, transportClient, nil)
			managedClusterMigrationSyncer.sendResources = true

			payload, err := json.Marshal(c.receivedMigrationEventBundle)
			assert.Nil(t, err)
			if err != nil {
				t.Errorf("Failed to marshal payload of managed cluster migration: %v", err)
			}

			// sync managed cluster migration
			err = managedClusterMigrationSyncer.Sync(ctx, payload)
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
					err = fakeClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)
					assert.Nil(t, err)
					// utils.PrettyPrint(obj)
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
