// Copyright (c) 2026 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package migration

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	migrationbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
)

func TestHandleStage_requiresMigrationId(t *testing.T) {
	syncer := &MigrationSourceSyncer{completedStages: make(map[string]string)}
	err := syncer.handleStage(context.Background(), &migrationbundle.MigrationSourceBundle{
		Stage: migrationv1alpha1.PhaseDeploying,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "migrationId is required")
}

func TestHandleStage_rejectsMismatchedMigrationId(t *testing.T) {
	syncer := &MigrationSourceSyncer{
		processingMigrationId: "migration-a",
		completedStages:       make(map[string]string),
	}
	err := syncer.handleStage(context.Background(), &migrationbundle.MigrationSourceBundle{
		MigrationId: "migration-b",
		Stage:       migrationv1alpha1.PhaseDeploying,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected migrationId migration-a")
}

func TestHandleStage_skipsDuplicateInProgressStage(t *testing.T) {
	syncer := &MigrationSourceSyncer{
		processingMigrationId: "migration-1",
		completedStages: map[string]string{
			migrationv1alpha1.PhaseRegistering: "in-progress",
		},
	}
	err := syncer.handleStage(context.Background(), &migrationbundle.MigrationSourceBundle{
		MigrationId: "migration-1",
		Stage:       migrationv1alpha1.PhaseRegistering,
	})
	require.NoError(t, err)
}

func TestHandleStage_skipsCompletedStage(t *testing.T) {
	syncer := &MigrationSourceSyncer{
		processingMigrationId: "migration-1",
		completedStages: map[string]string{
			migrationv1alpha1.PhaseCleaning: "completed",
		},
	}
	err := syncer.handleStage(context.Background(), &migrationbundle.MigrationSourceBundle{
		MigrationId: "migration-1",
		Stage:       migrationv1alpha1.PhaseCleaning,
	})
	require.NoError(t, err)
}

func TestHandleStage_unknownStageIsNoOp(t *testing.T) {
	syncer := &MigrationSourceSyncer{
		processingMigrationId: "migration-1",
		completedStages:       make(map[string]string),
	}
	err := syncer.handleStage(context.Background(), &migrationbundle.MigrationSourceBundle{
		MigrationId: "migration-1",
		Stage:       "UnknownStage",
	})
	require.NoError(t, err)
}

func TestExecuteStage_clearsInProgressOnFailure(t *testing.T) {
	syncer := &MigrationSourceSyncer{
		processingMigrationId: "migration-1",
		completedStages: map[string]string{
			migrationv1alpha1.PhaseDeploying: "in-progress",
		},
	}
	source := &migrationbundle.MigrationSourceBundle{
		MigrationId: "migration-1",
		Stage:       migrationv1alpha1.PhaseDeploying,
	}
	err := syncer.executeStage(context.Background(), source, func(context.Context, *migrationbundle.MigrationSourceBundle) error {
		return errors.New("stage failed")
	})
	require.Error(t, err)
	assert.Empty(t, syncer.completedStages[migrationv1alpha1.PhaseDeploying])
}

func TestExecuteStage_marksCompletedOnSuccess(t *testing.T) {
	syncer := &MigrationSourceSyncer{
		processingMigrationId: "migration-1",
		completedStages:       make(map[string]string),
	}
	source := &migrationbundle.MigrationSourceBundle{
		MigrationId: "migration-1",
		Stage:       migrationv1alpha1.PhaseCleaning,
	}
	err := syncer.executeStage(context.Background(), source, func(context.Context, *migrationbundle.MigrationSourceBundle) error {
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, "completed", syncer.completedStages[migrationv1alpha1.PhaseCleaning])
}

func TestExecuteStage_rollbackingDoesNotMarkCompleted(t *testing.T) {
	syncer := &MigrationSourceSyncer{
		processingMigrationId: "migration-1",
		completedStages:       make(map[string]string),
	}
	source := &migrationbundle.MigrationSourceBundle{
		MigrationId: "migration-1",
		Stage:       migrationv1alpha1.PhaseRollbacking,
	}
	err := syncer.executeStage(context.Background(), source, func(context.Context, *migrationbundle.MigrationSourceBundle) error {
		return nil
	})
	require.NoError(t, err)
	assert.Empty(t, syncer.completedStages[migrationv1alpha1.PhaseRollbacking])
}

func TestHandleStage_startsNewMigration(t *testing.T) {
	syncer := &MigrationSourceSyncer{
		bundleVersion: eventversion.NewVersion(),
		completedStages: map[string]string{
			migrationv1alpha1.PhaseCleaning: "completed",
		},
	}
	err := syncer.handleStage(context.Background(), &migrationbundle.MigrationSourceBundle{
		MigrationId: "migration-new",
		Stage:       migrationv1alpha1.PhaseDeploying,
	})
	require.NoError(t, err)
	assert.Equal(t, "migration-new", syncer.processingMigrationId)
	assert.Equal(t, "in-progress", syncer.completedStages[migrationv1alpha1.PhaseDeploying])
}

func TestHandleStage_resetsOnValidatingMigrationSwitch(t *testing.T) {
	syncer := &MigrationSourceSyncer{
		bundleVersion:         eventversion.NewVersion(),
		processingMigrationId: "migration-old",
		completedStages: map[string]string{
			migrationv1alpha1.PhaseDeploying: "completed",
		},
	}
	err := syncer.handleStage(context.Background(), &migrationbundle.MigrationSourceBundle{
		MigrationId: "migration-new",
		Stage:       migrationv1alpha1.PhaseValidating,
	})
	require.NoError(t, err)
	assert.Equal(t, "migration-new", syncer.processingMigrationId)
	assert.Equal(t, "in-progress", syncer.completedStages[migrationv1alpha1.PhaseValidating])
}
