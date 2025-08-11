package migration

import (
	"testing"

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

func TestMigrationStatusHelpers(t *testing.T) {
	tests := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "Should add and remove migration status correctly",
			testFunc: func(t *testing.T) {
				migrationID := "test-migration-123"
				hubName := "test-hub"
				phase := migrationv1alpha1.PhaseInitializing

				// Test AddMigrationStatus
				AddMigrationStatus(migrationID)
				status := getMigrationStatus(migrationID)
				assert.NotNil(t, status)

				// Test SetStarted and GetStarted
				assert.False(t, GetStarted(migrationID, hubName, phase))
				SetStarted(migrationID, hubName, phase)
				assert.True(t, GetStarted(migrationID, hubName, phase))

				// Test SetFinished and GetFinished
				assert.False(t, GetFinished(migrationID, hubName, phase))
				SetFinished(migrationID, hubName, phase)
				assert.True(t, GetFinished(migrationID, hubName, phase))

				// Test SetErrorMessage and GetErrorMessage
				errorMsg := "test error message"
				assert.Empty(t, GetErrorMessage(migrationID, hubName, phase))
				SetErrorMessage(migrationID, hubName, phase, errorMsg)
				assert.Equal(t, errorMsg, GetErrorMessage(migrationID, hubName, phase))

				// Test RemoveMigrationStatus
				RemoveMigrationStatus(migrationID)
				status = getMigrationStatus(migrationID)
				assert.Nil(t, status)
			},
		},
		{
			name: "Should handle source clusters correctly",
			testFunc: func(t *testing.T) {
				migrationID := "test-migration-456"
				sourceHub := "source-hub"
				// clusters variable removed as it's no longer used

				// Add migration status first
				AddMigrationStatus(migrationID)

				// Test basic migration status functionality
				// These functions no longer exist in the current implementation
				// Testing the core status tracking functionality instead
				SetStarted(migrationID, sourceHub, "testPhase")
				assert.True(t, GetStarted(migrationID, sourceHub, "testPhase"))

				SetFinished(migrationID, sourceHub, "testPhase")
				assert.True(t, GetFinished(migrationID, sourceHub, "testPhase"))

				// Clean up
				RemoveMigrationStatus(migrationID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFunc)
	}
}

func TestEventStatusEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "Should handle nil migration status gracefully",
			testFunc: func(t *testing.T) {
				migrationID := "non-existent-migration"
				hubName := "test-hub"
				phase := migrationv1alpha1.PhaseInitializing

				// All getters should return default values for non-existent migration
				assert.False(t, GetStarted(migrationID, hubName, phase), "GetStarted should return false for non-existent migration")
				assert.False(t, GetFinished(migrationID, hubName, phase), "GetFinished should return false for non-existent migration")
				assert.Empty(t, GetErrorMessage(migrationID, hubName, phase), "GetErrorMessage should return empty for non-existent migration")
				// GetSourceClusters no longer exists in current implementation

				// Setters should handle gracefully (not crash)
				assert.NotPanics(t, func() { SetStarted(migrationID, hubName, phase) }, "SetStarted should not panic for non-existent migration")
				assert.NotPanics(t, func() { SetFinished(migrationID, hubName, phase) }, "SetFinished should not panic for non-existent migration")
				assert.NotPanics(t, func() { SetErrorMessage(migrationID, hubName, phase, "error") }, "SetErrorMessage should not panic for non-existent migration")
				// SetSourceClusters no longer exists in current implementation
			},
		},
		{
			name: "Should handle empty strings and special characters",
			testFunc: func(t *testing.T) {
				migrationID := "test-special-chars"
				specialHubName := "hub-with-dashes_and_underscores.dots"
				phase := migrationv1alpha1.PhaseValidating

				AddMigrationStatus(migrationID)

				// Test with special characters in hub name
				SetStarted(migrationID, specialHubName, phase)
				assert.True(t, GetStarted(migrationID, specialHubName, phase), "Should handle special characters in hub name")

				// Test with empty error message
				SetErrorMessage(migrationID, specialHubName, phase, "")
				assert.Empty(t, GetErrorMessage(migrationID, specialHubName, phase), "Should handle empty error message")

				// Test with special characters in error message
				specialErrorMsg := "Error with special chars: !@#$%^&*()_+-={}[]|\\:;\"'<>?,./ 中文"
				SetErrorMessage(migrationID, specialHubName, phase, specialErrorMsg)
				assert.Equal(t, specialErrorMsg, GetErrorMessage(migrationID, specialHubName, phase), "Should handle special characters in error message")

				RemoveMigrationStatus(migrationID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFunc)
	}
}
