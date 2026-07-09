// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package migration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	migrationbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/controller"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func deployingMigrationCR(from, to string) *migrationv1alpha1.ManagedClusterMigration {
	return &migrationv1alpha1.ManagedClusterMigration{
		ObjectMeta: metav1.ObjectMeta{Name: "test-migration"},
		Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
			From: from,
			To:   to,
		},
		Status: migrationv1alpha1.ManagedClusterMigrationStatus{
			Phase: migrationv1alpha1.PhaseDeploying,
		},
	}
}

func migrationTestScheme() *runtime.Scheme {
	scheme := configs.GetRuntimeScheme()
	_ = migrationv1alpha1.AddToScheme(scheme)
	return scheme
}

func TestMigrationTargetSyncer_Sync_RejectsUntrustedSource(t *testing.T) {
	ctx := context.Background()
	scheme := migrationTestScheme()
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&migrationv1alpha1.ManagedClusterMigration{}).
		WithObjects(deployingMigrationCR("hub1", "hub2")).
		Build()

	transportClient := &controller.TransportClient{}
	agentConfig := &configs.AgentConfig{
		TransportConfig: &transport.TransportInternalConfig{
			KafkaCredential: &transport.KafkaConfig{StatusTopic: "status"},
		},
		LeafHubName:  "hub2",
		PodNamespace: "open-cluster-management",
	}
	configs.SetAgentConfig(agentConfig)
	syncer := NewMigrationTargetSyncer(fakeClient, transportClient, agentConfig)

	evt := utils.ToCloudEvent(string(enum.ManagedClusterMigrationType), "spoofed-hub", "hub2", nil)
	evt.SetExtension(constants.CloudEventExtensionKeyMigrationId, "migration-1")
	evt.SetExtension(constants.CloudEventExtensionKeyMigrationStage, migrationv1alpha1.PhaseDeploying)

	err := syncer.Sync(ctx, &evt)
	assert.Error(t, err, "untrusted migration source should fail Sync")
	assert.Contains(t, err.Error(), "untrusted migration event source", "error should identify untrusted source")
}

func TestMigrationTargetSyncer_SyncResource_RejectsClusterRole(t *testing.T) {
	ctx := context.Background()
	scheme := migrationTestScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	syncer := NewMigrationTargetSyncer(fakeClient, nil, &configs.AgentConfig{LeafHubName: "hub2"})

	clusterRole := &unstructured.Unstructured{}
	clusterRole.SetGroupVersionKind(rbacv1.SchemeGroupVersion.WithKind("ClusterRole"))
	clusterRole.SetName("admin")

	err := syncer.syncResource(ctx, "cluster1", clusterRole)
	assert.Error(t, err, "cluster role should be rejected by deploy allow-list")
	assert.Contains(t, err.Error(), "not allowed", "error should name disallowed resource")
}

func TestMigrationTargetSyncer_Sync_RejectsSourceHubWrongStage(t *testing.T) {
	ctx := context.Background()
	scheme := migrationTestScheme()
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&migrationv1alpha1.ManagedClusterMigration{}).
		WithObjects(deployingMigrationCR("hub1", "hub2")).
		Build()

	syncer := NewMigrationTargetSyncer(fakeClient, nil, &configs.AgentConfig{LeafHubName: "hub2"})

	evt := utils.ToCloudEvent(string(enum.ManagedClusterMigrationType), "hub1", "hub2", nil)
	evt.SetExtension(constants.CloudEventExtensionKeyMigrationId, "migration-1")
	evt.SetExtension(constants.CloudEventExtensionKeyMigrationStage, migrationv1alpha1.PhaseInitializing)

	err := syncer.Sync(ctx, &evt)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "source-hub migration event must use stage")
}

func TestMigrationTargetSyncer_Sync_AllowsRegisteredSourceDeploying(t *testing.T) {
	ctx := context.Background()
	scheme := migrationTestScheme()
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&migrationv1alpha1.ManagedClusterMigration{}).
		WithObjects(deployingMigrationCR("hub1", "hub2")).
		Build()

	producer := ProducerMock{}
	transportClient := &controller.TransportClient{}
	transportClient.SetProducer(&producer)
	agentConfig := &configs.AgentConfig{
		TransportConfig: &transport.TransportInternalConfig{
			KafkaCredential: &transport.KafkaConfig{StatusTopic: "status"},
		},
		LeafHubName:  "hub2",
		PodNamespace: "open-cluster-management",
	}
	configs.SetAgentConfig(agentConfig)
	syncer := NewMigrationTargetSyncer(fakeClient, transportClient, agentConfig)

	payload := migrationbundle.MigrationResourceBundle{
		TotalClusters:             1,
		MigrationClusterResources: []migrationbundle.MigrationClusterResource{},
	}
	evt := utils.ToCloudEvent(string(enum.ManagedClusterMigrationType), "hub1", "hub2", payload)
	evt.SetExtension(constants.CloudEventExtensionKeyMigrationId, "migration-1")
	evt.SetExtension(constants.CloudEventExtensionKeyMigrationStage, migrationv1alpha1.PhaseDeploying)
	evt.SetTime(time.Now())

	err := syncer.Sync(ctx, &evt)
	assert.NoError(t, err, "registered source hub deploying event should pass source validation")
}

func TestMigrationTargetSyncer_syncResource_AllowsConfigMap(t *testing.T) {
	ctx := context.Background()
	scheme := migrationTestScheme()
	_ = corev1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	syncer := NewMigrationTargetSyncer(fakeClient, nil, &configs.AgentConfig{LeafHubName: "hub2"})

	configMap := &unstructured.Unstructured{}
	configMap.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
	configMap.SetNamespace("cluster1")
	configMap.SetName("extra-manifests")
	_ = unstructured.SetNestedField(configMap.Object, map[string]interface{}{
		"key": "value",
	}, "data")

	assert.NoError(t, syncer.syncResource(ctx, "cluster1", configMap))

	got := &corev1.ConfigMap{}
	assert.NoError(t, fakeClient.Get(ctx, types.NamespacedName{
		Name:      "extra-manifests",
		Namespace: "cluster1",
	}, got))
	assert.Equal(t, "value", got.Data["key"])
}
