package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	ceprotocol "github.com/cloudevents/sdk-go/v2/protocol"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm/clause"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/producer"
	"github.com/stolostron/multicluster-global-hub/test/integration/utils/testpostgres"
)

func TestGenerateConsumer(t *testing.T) {
	mockKafkaCluster, err := kafka.NewMockCluster(1)
	if err != nil {
		t.Errorf("failed to init mock kafka cluster - %v", err)
	}
	transportConfig := &transport.TransportInternalConfig{
		TransportType: "kafka",
		KafkaCredential: &transport.KafkaConfig{
			BootstrapServer: mockKafkaCluster.BootstrapServers(),
			SpecTopic:       "test-topic",
			ConsumerGroupID: "test-consumer",
		},
	}
	options := []GenericConsumeOption{}
	// set consumerTopics to status or spec topic based on running in manager or not
	options = append(options, SetTopicMetadataRefreshInterval(constants.TopicMetadataRefreshInterval))

	_, err = NewGenericConsumer(transportConfig, []string{transportConfig.KafkaCredential.SpecTopic}, options...)
	if err != nil && !strings.Contains(err.Error(), "client has run out of available brokers") {
		t.Errorf("failed to generate consumer - %v", err)
	}
	// cannot get the kafka.ConfigMap from a Kafka consumer after it's created
	// The confluent-kafka-go library doesn't expose the configuration used to create the consumer.
}

func TestNewGenericConsumerSeparatesMigrationTopic(t *testing.T) {
	transportConfig := &transport.TransportInternalConfig{
		TransportType: string(transport.Chan),
		KafkaCredential: &transport.KafkaConfig{
			SpecTopic:      "gh-spec",
			MigrationTopic: "gh-migration",
		},
	}

	consumer, err := NewGenericConsumer(transportConfig, []string{"gh-spec", "gh-migration"})
	assert.NoError(t, err)
	assert.NotNil(t, consumer.migrationClient)
	assert.NotNil(t, transportConfig.Extends["gh-spec"])
	assert.NotNil(t, transportConfig.Extends["gh-migration"])
	assert.NotEqual(t, transportConfig.Extends["gh-spec"], transportConfig.Extends["gh-migration"])
}

func TestGenericConsumerReceivesMigrationEventsSeparately(t *testing.T) {
	transportConfig := &transport.TransportInternalConfig{
		TransportType: string(transport.Chan),
		KafkaCredential: &transport.KafkaConfig{
			SpecTopic:      "gh-spec",
			MigrationTopic: "gh-migration",
		},
	}

	consumer, err := NewGenericConsumer(transportConfig, []string{"gh-spec", "gh-migration"})
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- consumer.Start(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			assert.NoError(t, err)
		case <-time.After(time.Second):
			t.Error("consumer did not stop after context cancellation")
		}
	})

	producer, err := producer.NewGenericProducer(transportConfig, "gh-migration", nil)
	assert.NoError(t, err)
	event := cloudevents.NewEvent()
	event.SetType("migration.event")
	event.SetSource("test")
	assert.NoError(t, producer.SendEvent(ctx, event))

	select {
	case received := <-consumer.EventChan():
		assert.Equal(t, "migration.event", received.Type())
	case <-time.After(time.Second):
		t.Fatal("did not receive migration event")
	}
}

func TestGenericConsumerStopsMigrationRetryAfterCancellation(t *testing.T) {
	receiver := &failingCloudEventsClient{started: make(chan struct{}, 1)}
	consumer := &GenericConsumer{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		consumer.receiveMigrationEvents(ctx, receiver)
		close(done)
	}()

	select {
	case <-receiver.started:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("migration receiver was not started")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("migration receiver did not stop after context cancellation")
	}
}

func TestGenericConsumerReconnectsMigrationReceiver(t *testing.T) {
	transportConfig := &transport.TransportInternalConfig{
		TransportType: string(transport.Chan),
		KafkaCredential: &transport.KafkaConfig{
			SpecTopic:       "gh-spec",
			MigrationTopic:  "gh-migration",
			ConsumerGroupID: "test-consumer",
		},
	}

	consumer, err := NewGenericConsumer(transportConfig, []string{"gh-spec", "gh-migration"})
	assert.NoError(t, err)
	consumerStopped := make(chan struct{})
	migrationStopped := make(chan struct{})
	consumer.consumerCancel = func() { close(consumerStopped) }
	consumer.migrationCancel = func() { close(migrationStopped) }

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	assert.NoError(t, consumer.Reconnect(ctx, transportConfig, []string{"gh-spec", "gh-migration"}))
	select {
	case <-consumerStopped:
	case <-time.After(time.Second):
		t.Fatal("previous consumer was not cancelled")
	}
	select {
	case <-migrationStopped:
	case <-time.After(time.Second):
		t.Fatal("previous migration receiver was not cancelled")
	}

	producer, err := producer.NewGenericProducer(transportConfig, "gh-migration", nil)
	assert.NoError(t, err)
	event := cloudevents.NewEvent()
	event.SetType("migration.reconnect")
	event.SetSource("test")
	assert.NoError(t, producer.SendEvent(ctx, event))

	select {
	case received := <-consumer.EventChan():
		assert.Equal(t, "migration.reconnect", received.Type())
	case <-time.After(time.Second):
		t.Fatal("did not receive migration event after reconnect")
	}
}

type failingCloudEventsClient struct {
	started chan struct{}
}

func (c *failingCloudEventsClient) Send(context.Context, cloudevents.Event) ceprotocol.Result {
	return ceprotocol.ResultACK
}

func (c *failingCloudEventsClient) Request(context.Context, cloudevents.Event) (*cloudevents.Event, ceprotocol.Result) {
	return nil, ceprotocol.ResultACK
}

func (c *failingCloudEventsClient) StartReceiver(context.Context, interface{}) error {
	c.started <- struct{}{}
	return errors.New("topic authorization failed")
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
