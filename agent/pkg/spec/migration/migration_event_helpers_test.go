// Copyright (c) 2026 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package migration

import (
	"context"
	"errors"
	"testing"
	"time"

	kafka_confluent "github.com/cloudevents/sdk-go/protocol/kafka_confluent/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

func TestExtractMigrationExtensions(t *testing.T) {
	evt := cloudevents.NewEvent()
	evt.SetExtension(constants.CloudEventExtensionKeyMigrationId, "id-1")
	evt.SetExtension(constants.CloudEventExtensionKeyMigrationStage, "Deploying")

	migrationID, stage := extractMigrationExtensions(&evt)
	assert.Equal(t, "id-1", migrationID)
	assert.Equal(t, "Deploying", stage)
}

func TestMigrationStateKey(t *testing.T) {
	t.Run("uses kafka topic extension when present", func(t *testing.T) {
		evt := cloudevents.NewEvent()
		evt.SetSource("hub1")
		evt.SetExtension(kafka_confluent.KafkaTopicKey, "gh-migration")
		assert.Equal(t, "gh-migration--hub1", migrationStateKey(&evt))
	})

	t.Run("falls back when topic extension missing", func(t *testing.T) {
		evt := cloudevents.NewEvent()
		evt.SetSource("hub2")
		assert.Equal(t, "topic--hub2", migrationStateKey(&evt))
	})
}

func TestParseExpireTime(t *testing.T) {
	assert.True(t, parseExpireTime(nil).IsZero())

	evt := cloudevents.NewEvent()
	assert.True(t, parseExpireTime(&evt).IsZero())

	expiry := time.Now().Add(5 * time.Minute).UTC()
	evt.SetExtension(constants.CloudEventExtensionKeyExpireTime, expiry.Format(time.RFC3339))
	assert.Equal(t, expiry.Format(time.RFC3339), parseExpireTime(&evt).UTC().Format(time.RFC3339))
}

func TestExpireTimeContextHelpers(t *testing.T) {
	expiry := time.Now().Add(2 * time.Minute)
	ctx := withExpireTime(context.Background(), expiry)
	assert.Equal(t, expiry, expireTimeFromContext(ctx))
	assert.True(t, expireTimeFromContext(context.Background()).IsZero())
}

func TestRemainingExpireTime(t *testing.T) {
	assert.Equal(t, 10*time.Minute, remainingExpireTime(time.Time{}))

	future := time.Now().Add(30 * time.Second)
	remaining := remainingExpireTime(future)
	assert.True(t, remaining > 0 && remaining <= 30*time.Second)

	past := time.Now().Add(-time.Minute)
	assert.Equal(t, time.Duration(0), remainingExpireTime(past))
}

func TestIsMigrationTopicAuthorizationError(t *testing.T) {
	assert.False(t, isMigrationTopicAuthorizationError(nil))
	assert.False(t, isMigrationTopicAuthorizationError(errors.New("connection reset")))

	assert.True(t, isMigrationTopicAuthorizationError(errors.New("Topic authorization failed")))
	assert.True(t, isMigrationTopicAuthorizationError(errors.New("Broker: Topic authorization failed")))
}

func TestShouldSkipMigrationEvent(t *testing.T) {
	ctx := context.Background()
	namespace := "agent-ns"
	configs.SetAgentConfig(&configs.AgentConfig{PodNamespace: namespace})
	t.Cleanup(func() { configs.SetAgentConfig(nil) })

	t.Run("skips expired events", func(t *testing.T) {
		evt := cloudevents.NewEvent()
		evt.SetSource("hub1")
		evt.SetTime(time.Now())
		evt.SetExtension(constants.CloudEventExtensionKeyExpireTime,
			time.Now().Add(-time.Minute).Format(time.RFC3339))

		skip, err := shouldSkipMigrationEvent(ctx, fake.NewClientBuilder().WithScheme(scheme.Scheme).Build(), &evt)
		require.NoError(t, err)
		assert.True(t, skip)
	})

	t.Run("skips when cached migration time is on or after event time", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configs.AGENT_SYNC_STATE_CONFIG_MAP_NAME,
				Namespace: namespace,
			},
			Data: map[string]string{
				"gh-migration--hub1": time.Now().Add(time.Minute).Format(configs.AGENT_SYNC_STATE_TIME_FORMAT_VALUE),
			},
		}).Build()

		evt := cloudevents.NewEvent()
		evt.SetSource("hub1")
		evt.SetTime(time.Now())
		evt.SetExtension(kafka_confluent.KafkaTopicKey, "gh-migration")

		skip, err := shouldSkipMigrationEvent(ctx, fakeClient, &evt)
		require.NoError(t, err)
		assert.True(t, skip)
	})

	t.Run("processes fresh events", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()

		evt := cloudevents.NewEvent()
		evt.SetSource("hub1")
		evt.SetTime(time.Now())
		evt.SetExtension(kafka_confluent.KafkaTopicKey, "gh-migration")
		evt.SetExtension(constants.CloudEventExtensionKeyExpireTime,
			time.Now().Add(10*time.Minute).Format(time.RFC3339))

		skip, err := shouldSkipMigrationEvent(ctx, fakeClient, &evt)
		require.NoError(t, err)
		assert.False(t, skip)
	})
}

func TestIsMigrationDeployingEvent_invalidStageExtension(t *testing.T) {
	evt := cloudevents.NewEvent()
	evt.SetType(string(enum.ManagedClusterMigrationType))
	evt.SetExtension(constants.CloudEventExtensionKeyMigrationStage, 123)
	assert.False(t, IsMigrationDeployingEvent(&evt))
}
