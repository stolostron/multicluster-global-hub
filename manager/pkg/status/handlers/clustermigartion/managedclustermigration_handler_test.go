// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package clustermigration

import (
	"context"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/migration"
	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	migrationbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func TestHandleExpiredMigrationEvent(t *testing.T) {
	handler := &managedClusterMigrationHandler{}

	event := cloudevents.NewEvent()
	event.SetSource("hub1")
	event.SetType("com.example.migration")
	event.SetSubject(constants.CloudEventGlobalHubClusterName)
	event.SetExtension(constants.CloudEventExtensionKeyMigrationId, "expired-migration")
	event.SetExtension(constants.CloudEventExtensionKeyMigrationStage, migrationv1alpha1.PhaseInitializing)
	event.SetExtension(constants.CloudEventExtensionKeyExpireTime,
		time.Now().Add(-5*time.Minute).Format(time.RFC3339))
	require.NoError(t, event.SetData(cloudevents.ApplicationJSON, migrationbundle.MigrationStatusBundle{}))

	err := handler.handle(context.Background(), &event)
	assert.NoError(t, err, "expired event should be silently skipped")

	assert.False(t, migration.GetFinished("expired-migration", "hub1", migrationv1alpha1.PhaseInitializing),
		"expired event should not be processed")
}

func TestHandleNonExpiredMigrationEvent(t *testing.T) {
	migrationId := "non-expired-123"
	migration.AddMigrationStatus(migrationId)
	handler := &managedClusterMigrationHandler{}

	event := cloudevents.NewEvent()
	event.SetSource("hub1")
	event.SetType("com.example.migration")
	event.SetSubject(constants.CloudEventGlobalHubClusterName)
	event.SetExtension(constants.CloudEventExtensionKeyMigrationId, migrationId)
	event.SetExtension(constants.CloudEventExtensionKeyMigrationStage, migrationv1alpha1.PhaseInitializing)
	event.SetExtension(constants.CloudEventExtensionKeyExpireTime,
		time.Now().Add(10*time.Minute).Format(time.RFC3339))
	require.NoError(t, event.SetData(cloudevents.ApplicationJSON, migrationbundle.MigrationStatusBundle{}))

	err := handler.handle(context.Background(), &event)
	assert.NoError(t, err)

	assert.True(t, migration.GetFinished(migrationId, "hub1", migrationv1alpha1.PhaseInitializing),
		"non-expired event should be processed")
}

func TestHandleMigrationEvent(t *testing.T) {
	migrationId := "123"
	migration.AddMigrationStatus(migrationId)
	handler := &managedClusterMigrationHandler{}

	tests := []struct {
		name         string
		stage        string
		errorMessage string
	}{
		{
			name:  "Initialing stage",
			stage: migrationv1alpha1.PhaseInitializing,
		},
		{
			name:  "Deployed stage",
			stage: migrationv1alpha1.PhaseDeploying,
		},
		{
			name:  "Registered stage",
			stage: migrationv1alpha1.PhaseRegistering,
		},
		{
			name:  "Cleaned stage",
			stage: migrationv1alpha1.PhaseCleaning,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			event := cloudevents.NewEvent()
			event.SetSource("hub1")
			event.SetType("com.example.migration")
			event.SetSubject(constants.CloudEventGlobalHubClusterName)
			event.SetExtension(constants.CloudEventExtensionKeyMigrationId, migrationId)
			event.SetExtension(constants.CloudEventExtensionKeyMigrationStage, tc.stage)
			require.NoError(t, event.SetData(cloudevents.ApplicationJSON, migrationbundle.MigrationStatusBundle{
				ErrMessage: tc.errorMessage,
			}))

			err := handler.handle(context.Background(), &event)
			if tc.errorMessage != "" {
				assert.Equal(t, tc.errorMessage, err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestHandleMigrationEventUsesClusterNameExtension(t *testing.T) {
	migrationId := "clustername-ext-123"
	migration.AddMigrationStatus(migrationId)
	defer migration.RemoveMigrationStatus(migrationId)

	handler := &managedClusterMigrationHandler{}

	event := cloudevents.NewEvent()
	event.SetSource("hub1")
	event.SetType("com.example.migration")
	event.SetExtension(constants.CloudEventExtensionKeyClusterName, constants.CloudEventGlobalHubClusterName)
	event.SetExtension(constants.CloudEventExtensionKeyMigrationId, migrationId)
	event.SetExtension(constants.CloudEventExtensionKeyMigrationStage, migrationv1alpha1.PhaseInitializing)
	require.NoError(t, event.SetData(cloudevents.ApplicationJSON, migrationbundle.MigrationStatusBundle{}))

	err := handler.handle(context.Background(), &event)
	assert.NoError(t, err)
	assert.True(t, migration.GetFinished(migrationId, "hub1", migrationv1alpha1.PhaseInitializing))
}

func TestHandleMigrationEventFailedClustersReported(t *testing.T) {
	migrationId := "failed-clusters-123"
	migration.AddMigrationStatus(migrationId)
	defer migration.RemoveMigrationStatus(migrationId)

	handler := &managedClusterMigrationHandler{}

	event := cloudevents.NewEvent()
	event.SetSource("hub1")
	event.SetType("com.example.migration")
	event.SetSubject(constants.CloudEventGlobalHubClusterName)
	event.SetExtension(constants.CloudEventExtensionKeyMigrationId, migrationId)
	event.SetExtension(constants.CloudEventExtensionKeyMigrationStage, migrationv1alpha1.PhaseValidating)
	require.NoError(t, event.SetData(cloudevents.ApplicationJSON, migrationbundle.MigrationStatusBundle{
		FailedClustersReported: true,
		FailedClusters:         []string{"cluster1", "cluster2"},
	}))

	err := handler.handle(context.Background(), &event)
	assert.NoError(t, err)
	assert.Equal(t, []string{"cluster1", "cluster2"},
		migration.GetFailedClusters(migrationId, "hub1", migrationv1alpha1.PhaseValidating))
}

func TestHandleMigrationEventResync(t *testing.T) {
	migrationId := "resync-123"
	migration.AddMigrationStatus(migrationId)
	migration.SetFinished(migrationId, "hub1", migrationv1alpha1.PhaseInitializing)
	defer migration.RemoveMigrationStatus(migrationId)

	handler := &managedClusterMigrationHandler{}

	event := cloudevents.NewEvent()
	event.SetSource("hub1")
	event.SetType("com.example.migration")
	event.SetSubject(constants.CloudEventGlobalHubClusterName)
	event.SetExtension(constants.CloudEventExtensionKeyMigrationId, migrationId)
	event.SetExtension(constants.CloudEventExtensionKeyMigrationStage, migrationv1alpha1.PhaseInitializing)
	require.NoError(t, event.SetData(cloudevents.ApplicationJSON, migrationbundle.MigrationStatusBundle{
		Resync: true,
	}))

	err := handler.handle(context.Background(), &event)
	assert.NoError(t, err)
	assert.False(t, migration.GetFinished(migrationId, "hub1", migrationv1alpha1.PhaseInitializing))
}

func TestHandleMigrationEventBundleFieldFallbacks(t *testing.T) {
	migrationId := "bundle-fallback-123"
	migration.AddMigrationStatus(migrationId)
	defer migration.RemoveMigrationStatus(migrationId)

	handler := &managedClusterMigrationHandler{}

	event := cloudevents.NewEvent()
	event.SetSource("hub1")
	event.SetType("com.example.migration")
	event.SetSubject(constants.CloudEventGlobalHubClusterName)
	require.NoError(t, event.SetData(cloudevents.ApplicationJSON, migrationbundle.MigrationStatusBundle{
		MigrationId: migrationId,
		Stage:       migrationv1alpha1.PhaseInitializing,
	}))

	err := handler.handle(context.Background(), &event)
	assert.NoError(t, err)
	assert.True(t, migration.GetFinished(migrationId, "hub1", migrationv1alpha1.PhaseInitializing))
}

func TestHandleMigrationEventValidatingSetsClusterList(t *testing.T) {
	migrationId := "validating-clusters-123"
	migration.AddMigrationStatus(migrationId)
	defer migration.RemoveMigrationStatus(migrationId)

	handler := &managedClusterMigrationHandler{}

	event := cloudevents.NewEvent()
	event.SetSource("hub1")
	event.SetType("com.example.migration")
	event.SetSubject(constants.CloudEventGlobalHubClusterName)
	event.SetExtension(constants.CloudEventExtensionKeyMigrationId, migrationId)
	event.SetExtension(constants.CloudEventExtensionKeyMigrationStage, migrationv1alpha1.PhaseValidating)
	require.NoError(t, event.SetData(cloudevents.ApplicationJSON, migrationbundle.MigrationStatusBundle{
		ManagedClusters: []string{"c1", "c2"},
	}))

	err := handler.handle(context.Background(), &event)
	assert.NoError(t, err)
	assert.Equal(t, []string{"c1", "c2"}, migration.GetClusterList(migrationId))
	assert.True(t, migration.GetFinished(migrationId, "hub1", migrationv1alpha1.PhaseValidating))
}

func TestHandleMigrationEventErrorMessage(t *testing.T) {
	migrationId := "error-msg-123"
	migration.AddMigrationStatus(migrationId)
	defer migration.RemoveMigrationStatus(migrationId)

	handler := &managedClusterMigrationHandler{}

	event := cloudevents.NewEvent()
	event.SetSource("hub1")
	event.SetType("com.example.migration")
	event.SetSubject(constants.CloudEventGlobalHubClusterName)
	event.SetExtension(constants.CloudEventExtensionKeyMigrationId, migrationId)
	event.SetExtension(constants.CloudEventExtensionKeyMigrationStage, migrationv1alpha1.PhaseDeploying)
	require.NoError(t, event.SetData(cloudevents.ApplicationJSON, migrationbundle.MigrationStatusBundle{
		ErrMessage: "deploy failed",
		ClusterErrors: map[string]string{
			"c1": "timeout",
		},
	}))

	err := handler.handle(context.Background(), &event)
	assert.NoError(t, err)
	assert.Equal(t, "deploy failed", migration.GetErrorMessage(migrationId, "hub1", migrationv1alpha1.PhaseDeploying))
	assert.Equal(t, map[string]string{"c1": "timeout"},
		migration.GetClusterErrors(migrationId, "hub1", migrationv1alpha1.PhaseDeploying))
	assert.False(t, migration.GetFinished(migrationId, "hub1", migrationv1alpha1.PhaseDeploying))
}

func TestHandleMigrationEventRejectsWrongSubject(t *testing.T) {
	handler := &managedClusterMigrationHandler{}

	event := cloudevents.NewEvent()
	event.SetSource("hub1")
	event.SetType("com.example.migration")
	event.SetSubject("wrong-subject")
	event.SetExtension(constants.CloudEventExtensionKeyMigrationId, "id-1")
	event.SetExtension(constants.CloudEventExtensionKeyMigrationStage, migrationv1alpha1.PhaseInitializing)
	require.NoError(t, event.SetData(cloudevents.ApplicationJSON, migrationbundle.MigrationStatusBundle{}))

	err := handler.handle(context.Background(), &event)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected to get the subject")
}

func TestHandleMigrationEventMissingMigrationId(t *testing.T) {
	handler := &managedClusterMigrationHandler{}

	event := cloudevents.NewEvent()
	event.SetSource("hub1")
	event.SetType("com.example.migration")
	event.SetSubject(constants.CloudEventGlobalHubClusterName)
	event.SetExtension(constants.CloudEventExtensionKeyMigrationStage, migrationv1alpha1.PhaseInitializing)
	require.NoError(t, event.SetData(cloudevents.ApplicationJSON, migrationbundle.MigrationStatusBundle{}))

	err := handler.handle(context.Background(), &event)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "migrationId")
}
