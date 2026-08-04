package migration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
)

func TestMigrationEventProgress(t *testing.T) {
	migrateId := "migration"
	AddMigrationStatus(migrateId)

	assert.False(t, GetStarted(migrateId, "source-cluster", migrationv1alpha1.PhaseInitializing))
	SetStarted(migrateId, "source-cluster", migrationv1alpha1.PhaseInitializing)
	assert.True(t, GetStarted(migrateId, "source-cluster", migrationv1alpha1.PhaseInitializing))

	// Test GetStarted
	assert.False(t, GetStarted(migrateId, "source-cluster", migrationv1alpha1.PhaseValidating))
	SetStarted(migrateId, "source-cluster", migrationv1alpha1.PhaseValidating)
	assert.True(t, GetStarted(migrateId, "source-cluster", migrationv1alpha1.PhaseValidating))

	// Test GetFinished
	assert.False(t, GetFinished(migrateId, "source-cluster", migrationv1alpha1.PhaseValidating))
	SetFinished(migrateId, "source-cluster", migrationv1alpha1.PhaseValidating)
	assert.True(t, GetFinished(migrateId, "source-cluster", migrationv1alpha1.PhaseValidating))

	// Test GetStarted
	assert.False(t, GetStarted(migrateId, "target-cluster", migrationv1alpha1.PhaseValidating))
	SetStarted(migrateId, "target-cluster", migrationv1alpha1.PhaseValidating)
	assert.True(t, GetStarted(migrateId, "target-cluster", migrationv1alpha1.PhaseValidating))

	// Test GetFinished
	assert.False(t, GetFinished(migrateId, "target-cluster", migrationv1alpha1.PhaseValidating))
	SetFinished(migrateId, "target-cluster", migrationv1alpha1.PhaseValidating)
	assert.True(t, GetFinished(migrateId, "target-cluster", migrationv1alpha1.PhaseValidating))

	// Test Get/Set Error
	assert.Empty(t, GetErrorMessage(migrateId, "target-cluster", migrationv1alpha1.PhaseValidating))
	SetErrorMessage(migrateId, "target-cluster", migrationv1alpha1.PhaseValidating, "failed to validate")
	assert.NotEmpty(t, GetErrorMessage(migrateId, "target-cluster", migrationv1alpha1.PhaseValidating))
}

func TestDeployingACLReadyTime(t *testing.T) {
	migrateId := "acl-ready-migration"
	t.Cleanup(func() { RemoveMigrationStatus(migrateId) })

	_, ok := GetDeployingACLReadyTime(migrateId)
	assert.False(t, ok)

	AddMigrationStatus(migrateId)
	_, ok = GetDeployingACLReadyTime(migrateId)
	assert.False(t, ok)

	readyTime := time.Now().Add(45 * time.Second)
	SetDeployingACLReadyTime(migrateId, readyTime)

	got, ok := GetDeployingACLReadyTime(migrateId)
	assert.True(t, ok)
	assert.Equal(t, readyTime, got)
}

func TestAddSourceClustersAndRemoveMigrationStatus(t *testing.T) {
	migrateId := "source-clusters-migration"
	t.Cleanup(func() { RemoveMigrationStatus(migrateId) })

	AddMigrationStatus(migrateId)
	clusters := map[string][]string{"hub1": {"cluster1", "cluster2"}}
	AddSourceClusters(migrateId, clusters)
	assert.Equal(t, clusters, GetSourceClusters(migrateId))

	RemoveMigrationStatus(migrateId)
	assert.Nil(t, GetSourceClusters(migrateId))
	_, ok := GetDeployingACLReadyTime(migrateId)
	assert.False(t, ok)
}
