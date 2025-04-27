package migration

import (
	"testing"

	"github.com/stretchr/testify/assert"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
)

func TestMigrationEventProgress(t *testing.T) {
	mep := &MigrationEventProgress{
		sourceStatuses: map[string]*MigrationPhase{
			"source-cluster": {
				Validating: MigrationPhaseStatus{
					started:  false,
					finished: false,
					error:    "",
				},
			},
		},
		targetStatuses: map[string]*MigrationPhase{
			"target-cluster": {
				Validating: MigrationPhaseStatus{
					started:  false,
					finished: false,
					error:    "",
				},
			},
		},
	}

	assert.False(t, IsSourceStarted(mep, migrationv1alpha1.PhaseInitializing, "source-cluster"))
	SetSourceStarted(mep, migrationv1alpha1.PhaseInitializing, "source-cluster")
	assert.True(t, IsSourceStarted(mep, migrationv1alpha1.PhaseInitializing, "source-cluster"))

	// Test IsSourceStarted
	assert.False(t, IsSourceStarted(mep, migrationv1alpha1.PhaseValidating, "source-cluster"))
	SetSourceStarted(mep, migrationv1alpha1.PhaseValidating, "source-cluster")
	assert.True(t, IsSourceStarted(mep, migrationv1alpha1.PhaseValidating, "source-cluster"))

	// Test IsSourceFinished
	assert.False(t, IsSourceFinished(mep, migrationv1alpha1.PhaseValidating, "source-cluster"))
	SetSourceFinished(mep, migrationv1alpha1.PhaseValidating, "source-cluster")
	assert.True(t, IsSourceFinished(mep, migrationv1alpha1.PhaseValidating, "source-cluster"))

	// Test IsTargetStarted
	assert.False(t, IsTargetStarted(mep, migrationv1alpha1.PhaseValidating, "target-cluster"))
	SetTargetStarted(mep, migrationv1alpha1.PhaseValidating, "target-cluster")
	assert.True(t, IsTargetStarted(mep, migrationv1alpha1.PhaseValidating, "target-cluster"))

	// Test IsTargetFinished
	assert.False(t, IsTargetFinished(mep, migrationv1alpha1.PhaseValidating, "target-cluster"))
	SetTargetFinished(mep, migrationv1alpha1.PhaseValidating, "target-cluster")
	assert.True(t, IsTargetFinished(mep, migrationv1alpha1.PhaseValidating, "target-cluster"))
}
