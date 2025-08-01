// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package clustermigration

import (
	"context"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/migration"
	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	migrationbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

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
			event.SetExtension(constants.CloudEventExtensionKeyClusterName, constants.CloudEventGlobalHubClusterName)
			require.NoError(t, event.SetData(cloudevents.ApplicationJSON, migrationbundle.MigrationStatusBundle{
				Stage:       tc.stage,
				MigrationId: migrationId,
				ErrMessage:  tc.errorMessage,
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
