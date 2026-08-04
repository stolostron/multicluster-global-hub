package migration

import (
	"context"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

type deployingProducerMock struct{}

func (deployingProducerMock) SendEvent(_ context.Context, _ cloudevents.Event) error { return nil }

func (deployingProducerMock) Reconnect(_ *transport.TransportInternalConfig, _ string) error {
	return nil
}

func TestDeployingWaitsForACLPropagation(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	migrationUID := types.UID("deploying-acl-uid")
	mcm := &v1alpha1.ManagedClusterMigration{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-migration",
			Namespace:         "default",
			UID:               migrationUID,
			CreationTimestamp: metav1.Time{Time: time.Now()},
		},
		Spec: v1alpha1.ManagedClusterMigrationSpec{
			From: "source-hub",
			To:   "dest-hub",
		},
		Status: v1alpha1.ManagedClusterMigrationStatus{
			Phase: v1alpha1.PhaseDeploying,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mcm).
		WithStatusSubresource(mcm).
		Build()

	ctrl := &ClusterMigrationController{
		Client:   fakeClient,
		Producer: deployingProducerMock{},
	}

	migrationID := string(migrationUID)
	AddMigrationStatus(migrationID)
	AddSourceClusters(migrationID, map[string][]string{"source-hub": {"cluster1"}})
	t.Cleanup(func() { RemoveMigrationStatus(migrationID) })

	requeue, err := ctrl.deploying(context.Background(), mcm.DeepCopy())
	assert.NoError(t, err)
	assert.True(t, requeue)

	_, scheduled := GetDeployingACLReadyTime(migrationID)
	assert.True(t, scheduled)

	requeue, err = ctrl.deploying(context.Background(), mcm.DeepCopy())
	assert.NoError(t, err)
	assert.True(t, requeue)
}

func TestDeployingProceedsAfterACLReadyTime(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	migrationUID := types.UID("deploying-ready-uid")
	mcm := &v1alpha1.ManagedClusterMigration{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-migration",
			Namespace:         "default",
			UID:               migrationUID,
			CreationTimestamp: metav1.Time{Time: time.Now()},
		},
		Spec: v1alpha1.ManagedClusterMigrationSpec{
			From: "source-hub",
			To:   "dest-hub",
		},
		Status: v1alpha1.ManagedClusterMigrationStatus{
			Phase: v1alpha1.PhaseDeploying,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mcm).
		WithStatusSubresource(mcm).
		Build()

	ctrl := &ClusterMigrationController{
		Client:   fakeClient,
		Producer: deployingProducerMock{},
	}

	migrationID := string(migrationUID)
	AddMigrationStatus(migrationID)
	AddSourceClusters(migrationID, map[string][]string{"source-hub": {"cluster1"}})
	SetDeployingACLReadyTime(migrationID, time.Now().Add(-time.Minute))
	t.Cleanup(func() { RemoveMigrationStatus(migrationID) })

	requeue, err := ctrl.deploying(context.Background(), mcm.DeepCopy())
	assert.NoError(t, err)
	assert.True(t, requeue)
	assert.True(t, GetStarted(migrationID, "source-hub", v1alpha1.PhaseDeploying))
}
