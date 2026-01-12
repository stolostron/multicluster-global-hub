package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gorm.io/gorm/clause"

	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/test/integration/utils/testpostgres"
)

func TestGenerateConsumer(t *testing.T) {
	transportConfigChan := make(chan *transport.TransportInternalConfig, 1)

	consumer, err := NewGenericConsumer(transportConfigChan, true, false)
	if err != nil {
		t.Errorf("failed to generate consumer - %v", err)
	}
	if consumer == nil {
		t.Fatal("consumer should not be nil")
	}
	if consumer.eventChan == nil {
		t.Fatal("eventChan should be initialized")
	}
}

func TestGetInitOffset(t *testing.T) {
	testPostgres, err := testpostgres.NewTestPostgres()
	assert.Nil(t, err)
	err = testpostgres.InitDatabase(testPostgres.URI)
	assert.Nil(t, err)

	databaseTransports := []models.Transport{}

	kafkaClusterIdentity := "clusterID"
	deprecatedTransport := generateTransport(kafkaClusterIdentity, "status.hub6", 9)
	deprecatedTransport.UpdatedAt = time.Now().AddDate(0, 0, -8)
	databaseTransports = append(databaseTransports, generateTransport(kafkaClusterIdentity, "status.hub1", 12))
	databaseTransports = append(databaseTransports, generateTransport(kafkaClusterIdentity, "status.hub2", 11))
	databaseTransports = append(databaseTransports, generateTransport(kafkaClusterIdentity, "status", 9))
	databaseTransports = append(databaseTransports, deprecatedTransport)
	databaseTransports = append(databaseTransports, generateTransport("", "status.hub3", 8))
	databaseTransports = append(databaseTransports, generateTransport("another", "status.hub4", 7))

	db := database.GetGorm()
	err = db.Clauses(clause.OnConflict{
		UpdateAll: true,
	}).CreateInBatches(databaseTransports, 100).Error
	assert.Nil(t, err)
	offsets, err := getInitOffset(kafkaClusterIdentity)
	assert.Nil(t, err)

	count := 0
	for _, offset := range offsets {
		fmt.Println(*offset.Topic, offset.Partition, offset.Offset)
		count++
	}
	assert.Equal(t, 4, count)
}

func generateTransport(ownerIdentity string, topic string, offset int64) models.Transport {
	payload, _ := json.Marshal(transport.EventPosition{
		OwnerIdentity: ownerIdentity,
		Topic:         topic,
		Partition:     0,
		Offset:        int64(offset),
	})
	return models.Transport{
		Name:    topic,
		Payload: payload,
	}
}

func TestNewBackoff(t *testing.T) {
	backoff := newBackoff()

	assert.Equal(t, 1*time.Second, backoff.Duration, "initial duration should be 1 second")
	assert.Equal(t, 2.0, backoff.Factor, "factor should be 2.0")
	assert.Equal(t, 0.1, backoff.Jitter, "jitter should be 0.1")
	assert.Equal(t, 30*time.Second, backoff.Cap, "cap should be 30 seconds")
}

func TestResetBackoff(t *testing.T) {
	transportConfigChan := make(chan *transport.TransportInternalConfig, 1)
	consumer, err := NewGenericConsumer(transportConfigChan, true, false)
	assert.NoError(t, err)

	// Step the backoff multiple times to increase duration
	_ = consumer.backoff.Step()
	_ = consumer.backoff.Step()
	_ = consumer.backoff.Step()

	// Reset the backoff
	consumer.resetBackoff()

	// Verify backoff is reset to initial state
	assert.Equal(t, 1*time.Second, consumer.backoff.Duration, "duration should be reset to 1 second")
}

func TestGetBackoffDuration(t *testing.T) {
	transportConfigChan := make(chan *transport.TransportInternalConfig, 1)
	consumer, err := NewGenericConsumer(transportConfigChan, false, false)
	assert.NoError(t, err)

	t.Run("exponential backoff increases duration", func(t *testing.T) {
		// Reset to ensure clean state
		consumer.resetBackoff()

		// First call should return around 1s (with some jitter)
		d1 := consumer.getBackoffDuration(time.Now())
		assert.True(t, d1 >= 900*time.Millisecond && d1 <= 1100*time.Millisecond,
			"first backoff should be around 1s, got %v", d1)

		// Second call should return around 2s (with some jitter)
		d2 := consumer.getBackoffDuration(time.Now())
		assert.True(t, d2 >= 1800*time.Millisecond && d2 <= 2200*time.Millisecond,
			"second backoff should be around 2s, got %v", d2)

		// Third call should return around 4s (with some jitter)
		d3 := consumer.getBackoffDuration(time.Now())
		assert.True(t, d3 >= 3600*time.Millisecond && d3 <= 4400*time.Millisecond,
			"third backoff should be around 4s, got %v", d3)
	})

	t.Run("backoff resets after stable connection", func(t *testing.T) {
		consumer.resetBackoff()

		// Step multiple times to increase backoff
		_ = consumer.getBackoffDuration(time.Now())
		_ = consumer.getBackoffDuration(time.Now())
		_ = consumer.getBackoffDuration(time.Now())

		// Simulate stable connection (started more than 1 minute ago)
		stableStartTime := time.Now().Add(-2 * time.Minute)

		// This should reset the backoff because connection was stable
		d := consumer.getBackoffDuration(stableStartTime)

		// After reset, duration should be around 1s again
		assert.True(t, d >= 900*time.Millisecond && d <= 1100*time.Millisecond,
			"backoff should reset to around 1s after stable connection, got %v", d)
	})

	t.Run("backoff caps at 30 seconds", func(t *testing.T) {
		consumer.resetBackoff()

		// Step many times to hit the cap
		var lastDuration time.Duration
		for range 10 {
			lastDuration = consumer.getBackoffDuration(time.Now())
		}

		// Should be capped at 30 seconds (with some jitter)
		assert.True(t, lastDuration <= 33*time.Second,
			"backoff should be capped at around 30s, got %v", lastDuration)
	})
}

func TestNewGenericConsumerForAgent(t *testing.T) {
	transportConfigChan := make(chan *transport.TransportInternalConfig, 1)

	// Test creating consumer for agent (isManager=false)
	consumer, err := NewGenericConsumer(transportConfigChan, false, false)
	assert.NoError(t, err)
	assert.NotNil(t, consumer)
	assert.NotNil(t, consumer.eventChan)
	assert.NotNil(t, consumer.assembler)
	assert.Equal(t, false, consumer.isManager)
	assert.Equal(t, 0, consumer.topicMetadataRefreshInterval)
}

func TestNewGenericConsumerForManager(t *testing.T) {
	transportConfigChan := make(chan *transport.TransportInternalConfig, 1)

	// Test creating consumer for manager (isManager=true)
	consumer, err := NewGenericConsumer(transportConfigChan, true, true)
	assert.NoError(t, err)
	assert.NotNil(t, consumer)
	assert.Equal(t, true, consumer.isManager)
	assert.Equal(t, true, consumer.enableDatabaseOffset)
	assert.NotEqual(t, 0, consumer.topicMetadataRefreshInterval)
}

func TestEventChan(t *testing.T) {
	transportConfigChan := make(chan *transport.TransportInternalConfig, 1)

	consumer, err := NewGenericConsumer(transportConfigChan, true, false)
	assert.NoError(t, err)

	// EventChan should return the same channel
	ch := consumer.EventChan()
	assert.NotNil(t, ch)
	assert.Equal(t, consumer.eventChan, ch)
}

func TestInitClientWithChanTransport(t *testing.T) {
	transportConfigChan := make(chan *transport.TransportInternalConfig, 1)

	t.Run("init client for agent with Chan transport", func(t *testing.T) {
		consumer, err := NewGenericConsumer(transportConfigChan, false, false)
		assert.NoError(t, err)

		config := &transport.TransportInternalConfig{
			TransportType: string(transport.Chan),
			KafkaCredential: &transport.KafkaConfig{
				SpecTopic:   "spec-topic",
				StatusTopic: "status-topic",
			},
			Extends: make(map[string]any),
		}

		client, err := consumer.initClient(config)
		assert.NoError(t, err)
		assert.NotNil(t, client)
		// Verify the topic channel was created in Extends
		assert.NotNil(t, config.Extends["spec-topic"])
	})

	t.Run("init client for manager with Chan transport", func(t *testing.T) {
		consumer, err := NewGenericConsumer(transportConfigChan, true, false)
		assert.NoError(t, err)

		config := &transport.TransportInternalConfig{
			TransportType: string(transport.Chan),
			KafkaCredential: &transport.KafkaConfig{
				SpecTopic:   "spec-topic",
				StatusTopic: "status-topic",
			},
			Extends: make(map[string]any),
		}

		client, err := consumer.initClient(config)
		assert.NoError(t, err)
		assert.NotNil(t, client)
		// Manager consumes from status topic
		assert.NotNil(t, config.Extends["status-topic"])
	})

	t.Run("init client with nil Extends creates new map", func(t *testing.T) {
		consumer, err := NewGenericConsumer(transportConfigChan, false, false)
		assert.NoError(t, err)

		config := &transport.TransportInternalConfig{
			TransportType: string(transport.Chan),
			KafkaCredential: &transport.KafkaConfig{
				SpecTopic:   "spec-topic",
				StatusTopic: "status-topic",
			},
			Extends: nil, // nil Extends
		}

		client, err := consumer.initClient(config)
		assert.NoError(t, err)
		assert.NotNil(t, client)
		assert.NotNil(t, config.Extends)
	})

	t.Run("init client with invalid transport type", func(t *testing.T) {
		consumer, err := NewGenericConsumer(transportConfigChan, false, false)
		assert.NoError(t, err)

		config := &transport.TransportInternalConfig{
			TransportType: "invalid",
			KafkaCredential: &transport.KafkaConfig{
				SpecTopic:   "spec-topic",
				StatusTopic: "status-topic",
			},
		}

		client, err := consumer.initClient(config)
		assert.Error(t, err)
		assert.Nil(t, client)
		assert.Contains(t, err.Error(), "is not a valid option")
	})
}

func TestStartWithChanTransport(t *testing.T) {
	t.Run("start with context cancellation", func(t *testing.T) {
		transportConfigChan := make(chan *transport.TransportInternalConfig, 1)
		consumer, err := NewGenericConsumer(transportConfigChan, false, false)
		assert.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())

		startErr := make(chan error, 1)
		go func() {
			startErr <- consumer.Start(ctx)
		}()

		// Cancel immediately
		cancel()

		select {
		case err := <-startErr:
			assert.NoError(t, err)
		case <-time.After(2 * time.Second):
			t.Fatal("Start did not return after context cancellation")
		}
	})

	t.Run("start with invalid transport type returns error", func(t *testing.T) {
		transportConfigChan := make(chan *transport.TransportInternalConfig, 1)
		consumer, err := NewGenericConsumer(transportConfigChan, false, false)
		assert.NoError(t, err)

		config := &transport.TransportInternalConfig{
			TransportType: "invalid-type",
			KafkaCredential: &transport.KafkaConfig{
				SpecTopic:   "spec-topic",
				StatusTopic: "status-topic",
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		startErr := make(chan error, 1)
		go func() {
			startErr <- consumer.Start(ctx)
		}()

		// Send invalid config
		transportConfigChan <- config

		select {
		case err := <-startErr:
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "failed to init transport client")
		case <-time.After(2 * time.Second):
			t.Fatal("Start did not return error for invalid transport type")
		}
	})
}
