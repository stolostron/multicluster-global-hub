// Copyright (c) 2026 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package migration

import (
	"context"
	"errors"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	cetypes "github.com/cloudevents/sdk-go/v2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	migrationbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

type reportStatusProducer struct {
	lastEvent cloudevents.Event
	err       error
}

func (p *reportStatusProducer) SendEvent(_ context.Context, evt cloudevents.Event) error {
	p.lastEvent = evt
	return p.err
}

func (p *reportStatusProducer) Reconnect(_ *transport.TransportInternalConfig, _ string) error {
	return nil
}

func TestReportMigrationStatus_sendsManagedClusterMigrationEvent(t *testing.T) {
	configs.SetAgentConfig(&configs.AgentConfig{LeafHubName: "hub1"})
	t.Cleanup(func() { configs.SetAgentConfig(nil) })

	producer := &reportStatusProducer{}
	transportClient := &transport.TransportClient{}
	transportClient.SetProducer(producer)

	version := eventversion.NewVersion()
	bundle := &migrationbundle.MigrationStatusBundle{
		MigrationId: "migration-123",
		Stage:       migrationv1alpha1.PhaseDeploying,
	}

	expireTime := time.Now().Add(5 * time.Minute)
	require.NoError(t, ReportMigrationStatus(context.Background(), transportClient, bundle, version, expireTime))

	assert.Equal(t, string(enum.ManagedClusterMigrationType), producer.lastEvent.Type())
	assert.Equal(t, "hub1", producer.lastEvent.Source())

	clusterName, err := cetypes.ToString(producer.lastEvent.Extensions()[constants.CloudEventExtensionKeyClusterName])
	require.NoError(t, err)
	assert.Equal(t, constants.CloudEventGlobalHubClusterName, clusterName)

	migrationID, err := cetypes.ToString(producer.lastEvent.Extensions()[constants.CloudEventExtensionKeyMigrationId])
	require.NoError(t, err)
	assert.Equal(t, "migration-123", migrationID)
}

func TestReportMigrationStatus_nilTransportClient(t *testing.T) {
	err := ReportMigrationStatus(context.Background(), nil, &migrationbundle.MigrationStatusBundle{}, eventversion.NewVersion(), time.Time{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transport client must not be nil")
}

func TestReportMigrationStatus_producerError(t *testing.T) {
	configs.SetAgentConfig(&configs.AgentConfig{LeafHubName: "hub1"})
	t.Cleanup(func() { configs.SetAgentConfig(nil) })

	producer := &reportStatusProducer{err: errors.New("send failed")}
	transportClient := &transport.TransportClient{}
	transportClient.SetProducer(producer)

	err := ReportMigrationStatus(context.Background(), transportClient,
		&migrationbundle.MigrationStatusBundle{MigrationId: "id", Stage: migrationv1alpha1.PhaseDeploying},
		eventversion.NewVersion(), time.Now().Add(time.Minute))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "send failed")
}
