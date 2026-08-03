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

	// Create an event with an expired expirytime
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

	// Verify the event was not processed (no status set)
	assert.False(t, migration.GetFinished("expired-migration", "hub1", migrationv1alpha1.PhaseInitializing),
		"expired event should not be processed")
}

func TestHandleNonExpiredMigrationEvent(t *testing.T) {
	migrationId := "non-expired-123"
	migration.AddMigrationStatus(migrationId)
	handler := &managedClusterMigrationHandler{}

	// Create an event with a future expirytime
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

	// Verify the event was processed
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
