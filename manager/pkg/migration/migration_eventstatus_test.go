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

				// Setters should handle gracefully (not crash)
				assert.NotPanics(t, func() { SetStarted(migrationID, hubName, phase) }, "SetStarted should not panic for non-existent migration")
				assert.NotPanics(t, func() { SetFinished(migrationID, hubName, phase) }, "SetFinished should not panic for non-existent migration")
				assert.NotPanics(t, func() { SetErrorMessage(migrationID, hubName, phase, "error") }, "SetErrorMessage should not panic for non-existent migration")
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

// TestResetMigrationStatus tests the ResetMigrationStatus function - regression test for hub name parsing bug
func TestResetMigrationStatus(t *testing.T) {
	tests := []struct {
		name             string
		setupFunc        func() (migrationID string, hubName string, phase string)
		resetHubName     string
		expectedReset    bool
		expectedNotReset string // hub that should NOT be reset
	}{
		{
			name: "Should reset migration status for matching hub name",
			setupFunc: func() (string, string, string) {
				migrationID := "test-reset-migration-1"
				hubName := "source-hub"
				phase := migrationv1alpha1.PhaseValidating

				AddMigrationStatus(migrationID)
				SetStarted(migrationID, hubName, phase)
				SetFinished(migrationID, hubName, phase)
				SetErrorMessage(migrationID, hubName, phase, "test error")

				return migrationID, hubName, phase
			},
			resetHubName:  "source-hub",
			expectedReset: true,
		},
		{
			name: "Should not reset migration status for non-matching hub name",
			setupFunc: func() (string, string, string) {
				migrationID := "test-reset-migration-2"
				hubName := "target-hub"
				phase := migrationv1alpha1.PhaseRegistering

				AddMigrationStatus(migrationID)
				SetStarted(migrationID, hubName, phase)
				SetFinished(migrationID, hubName, phase)
				SetErrorMessage(migrationID, hubName, phase, "test error")

				return migrationID, hubName, phase
			},
			resetHubName:  "different-hub",
			expectedReset: false,
		},
		{
			name: "Should reset only matching hub and preserve other hubs",
			setupFunc: func() (string, string, string) {
				migrationID := "test-reset-migration-3"
				hubName := "target-hub"
				phase := migrationv1alpha1.PhaseDeploying

				AddMigrationStatus(migrationID)
				// Setup target hub state
				SetStarted(migrationID, hubName, phase)
				SetFinished(migrationID, hubName, phase)
				SetErrorMessage(migrationID, hubName, phase, "target error")

				// Setup different hub state that should NOT be reset
				otherHub := "other-hub"
				SetStarted(migrationID, otherHub, phase)
				SetFinished(migrationID, otherHub, phase)
				SetErrorMessage(migrationID, otherHub, phase, "other error")

				return migrationID, hubName, phase
			},
			resetHubName:     "target-hub",
			expectedReset:    true,
			expectedNotReset: "other-hub",
		},
		{
			name: "Should handle hub names with dashes correctly - regression test",
			setupFunc: func() (string, string, string) {
				migrationID := "test-reset-migration-4"
				hubName := "hub-with-dashes"
				phase := migrationv1alpha1.PhaseInitializing

				AddMigrationStatus(migrationID)
				SetStarted(migrationID, hubName, phase)
				SetFinished(migrationID, hubName, phase)
				SetErrorMessage(migrationID, hubName, phase, "test error")

				return migrationID, hubName, phase
			},
			resetHubName:  "hub-with-dashes",
			expectedReset: true,
		},
		{
			name: "Should handle multiple phases for same hub",
			setupFunc: func() (string, string, string) {
				migrationID := "test-reset-migration-5"
				hubName := "multi-phase-hub"
				phase1 := migrationv1alpha1.PhaseValidating
				phase2 := migrationv1alpha1.PhaseDeploying

				AddMigrationStatus(migrationID)
				// Setup phase 1
				SetStarted(migrationID, hubName, phase1)
				SetFinished(migrationID, hubName, phase1)
				SetErrorMessage(migrationID, hubName, phase1, "phase1 error")

				// Setup phase 2
				SetStarted(migrationID, hubName, phase2)
				SetFinished(migrationID, hubName, phase2)
				SetErrorMessage(migrationID, hubName, phase2, "phase2 error")

				return migrationID, hubName, phase1 // Return first phase for primary verification
			},
			resetHubName:  "multi-phase-hub",
			expectedReset: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test data
			migrationID, hubName, phase := tt.setupFunc()

			// Verify initial state - should be set
			assert.True(t, GetStarted(migrationID, hubName, phase), "Initial started state should be true")
			assert.True(t, GetFinished(migrationID, hubName, phase), "Initial finished state should be true")
			assert.NotEmpty(t, GetErrorMessage(migrationID, hubName, phase), "Initial error message should not be empty")

			// Call ResetMigrationStatus
			t.Logf("Calling ResetMigrationStatus with hub name: %s", tt.resetHubName)
			ResetMigrationStatus(tt.resetHubName)

			// Verify the result
			if tt.expectedReset {
				assert.False(t, GetStarted(migrationID, hubName, phase), "Started state should be reset to false")
				assert.False(t, GetFinished(migrationID, hubName, phase), "Finished state should be reset to false")
				assert.Empty(t, GetErrorMessage(migrationID, hubName, phase), "Error message should be reset to empty")

				// If testing multiple phases, verify both phases are reset
				if tt.name == "Should handle multiple phases for same hub" {
					phase2 := migrationv1alpha1.PhaseDeploying
					assert.False(t, GetStarted(migrationID, hubName, phase2), "Started state for phase2 should be reset to false")
					assert.False(t, GetFinished(migrationID, hubName, phase2), "Finished state for phase2 should be reset to false")
					assert.Empty(t, GetErrorMessage(migrationID, hubName, phase2), "Error message for phase2 should be reset to empty")
				}
			} else {
				assert.True(t, GetStarted(migrationID, hubName, phase), "Started state should remain true for non-matching hub")
				assert.True(t, GetFinished(migrationID, hubName, phase), "Finished state should remain true for non-matching hub")
				assert.NotEmpty(t, GetErrorMessage(migrationID, hubName, phase), "Error message should remain for non-matching hub")
			}

			// If there's a hub that should NOT be reset, verify it's preserved
			if tt.expectedNotReset != "" {
				assert.True(t, GetStarted(migrationID, tt.expectedNotReset, phase), "Non-matching hub should not be reset")
				assert.True(t, GetFinished(migrationID, tt.expectedNotReset, phase), "Non-matching hub should not be reset")
				assert.NotEmpty(t, GetErrorMessage(migrationID, tt.expectedNotReset, phase), "Non-matching hub error should not be reset")
			}

			// Cleanup
			RemoveMigrationStatus(migrationID)
		})
	}
}

// TestGetClusterList tests the GetClusterList function
func TestGetClusterList(t *testing.T) {
	tests := []struct {
		name           string
		migrationID    string
		hub            string
		phase          string
		setupClusters  []string
		expectedResult []string
		shouldSetup    bool
	}{
		{
			name:           "valid migration with cluster list",
			migrationID:    "test-migration-with-clusters",
			hub:            "source-hub",
			phase:          migrationv1alpha1.PhaseValidating,
			setupClusters:  []string{"cluster1", "cluster2", "cluster3"},
			expectedResult: []string{"cluster1", "cluster2", "cluster3"},
			shouldSetup:    true,
		},
		{
			name:           "valid migration with empty cluster list",
			migrationID:    "test-migration-empty-clusters",
			hub:            "source-hub",
			phase:          migrationv1alpha1.PhaseInitializing,
			setupClusters:  []string{},
			expectedResult: []string{},
			shouldSetup:    true,
		},
		{
			name:           "valid migration with single cluster",
			migrationID:    "test-migration-single-cluster",
			hub:            "target-hub",
			phase:          migrationv1alpha1.PhaseDeploying,
			setupClusters:  []string{"single-cluster"},
			expectedResult: []string{"single-cluster"},
			shouldSetup:    true,
		},
		{
			name:           "non-existent migration",
			migrationID:    "non-existent-migration",
			hub:            "any-hub",
			phase:          migrationv1alpha1.PhaseValidating,
			setupClusters:  nil,
			expectedResult: nil,
			shouldSetup:    false,
		},
		{
			name:           "valid migration but non-existent hub-phase combination",
			migrationID:    "test-migration-wrong-phase",
			hub:            "wrong-hub",
			phase:          "wrong-phase",
			setupClusters:  nil,
			expectedResult: nil,
			shouldSetup:    true,
		},
		{
			name:           "valid migration with nil cluster list initially",
			migrationID:    "test-migration-nil-clusters",
			hub:            "test-hub",
			phase:          migrationv1alpha1.PhaseRegistering,
			setupClusters:  nil,
			expectedResult: nil,
			shouldSetup:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup migration status if needed
			if tt.shouldSetup {
				AddMigrationStatus(tt.migrationID)
				if tt.setupClusters != nil {
					SetClusterList(tt.migrationID, tt.setupClusters)
				}
			}

			// Test GetClusterList
			result := GetClusterList(tt.migrationID)

			// Verify result
			if tt.expectedResult == nil {
				assert.Nil(t, result, "Expected nil result for migration %s, hub %s, phase %s", tt.migrationID, tt.hub, tt.phase)
			} else {
				assert.Equal(t, tt.expectedResult, result, "Cluster list mismatch for migration %s, hub %s, phase %s", tt.migrationID, tt.hub, tt.phase)
			}

			// Cleanup if migration was created
			if tt.shouldSetup {
				RemoveMigrationStatus(tt.migrationID)
			}
		})
	}
}
