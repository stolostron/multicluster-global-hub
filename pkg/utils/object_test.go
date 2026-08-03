package utils

import (
	"encoding/json"
	"testing"
	"time"

	cetypes "github.com/cloudevents/sdk-go/v2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func TestToMigrationEvent(t *testing.T) {
	const (
		eventType   = "migration.event"
		source      = "global-hub"
		subject     = "target-hub"
		migrationID = "migration-abc"
		stage       = "Deploying"
	)
	expireAfter := 15 * time.Minute
	payload := map[string]string{"cluster": "cluster1"}

	before := time.Now()
	evt := ToMigrationEvent(eventType, source, subject, migrationID, stage, expireAfter, payload)
	after := time.Now().Add(expireAfter)

	assert.Equal(t, eventType, evt.Type())
	assert.Equal(t, source, evt.Source())
	assert.Equal(t, subject, evt.Subject())

	clusterName, err := cetypes.ToString(evt.Extensions()[constants.CloudEventExtensionKeyClusterName])
	require.NoError(t, err)
	assert.Equal(t, subject, clusterName)

	gotMigrationID, err := cetypes.ToString(evt.Extensions()[constants.CloudEventExtensionKeyMigrationId])
	require.NoError(t, err)
	assert.Equal(t, migrationID, gotMigrationID)

	gotStage, err := cetypes.ToString(evt.Extensions()[constants.CloudEventExtensionKeyMigrationStage])
	require.NoError(t, err)
	assert.Equal(t, stage, gotStage)

	expireStr, err := cetypes.ToString(evt.Extensions()[constants.CloudEventExtensionKeyExpireTime])
	require.NoError(t, err)
	expireTime, err := time.Parse(time.RFC3339, expireStr)
	require.NoError(t, err)
	assert.True(t, !expireTime.Before(before))
	assert.True(t, !expireTime.After(after))

	var decoded map[string]string
	require.NoError(t, json.Unmarshal(evt.Data(), &decoded))
	assert.Equal(t, payload, decoded)
}

func TestToCloudEventSetsClusterNameExtension(t *testing.T) {
	evt := ToCloudEvent("Policy", "hub1", "cluster1", nil)
	clusterName, err := cetypes.ToString(evt.Extensions()[constants.CloudEventExtensionKeyClusterName])
	require.NoError(t, err)
	assert.Equal(t, "cluster1", clusterName)
}
