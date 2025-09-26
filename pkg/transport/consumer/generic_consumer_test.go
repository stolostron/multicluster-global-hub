package consumer

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm/clause"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
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

func TestGetInitOffset(t *testing.T) {
	testPostgres, err := testpostgres.NewTestPostgres()
	assert.Nil(t, err)
	err = testpostgres.InitDatabase(testPostgres.URI)
	assert.Nil(t, err)

	databaseTransports := []models.Transport{}

	kafkaClusterIdentity := "clusterID"
	databaseTransports = append(databaseTransports, generateTransport(kafkaClusterIdentity, "status.hub1", 12))
	databaseTransports = append(databaseTransports, generateTransport(kafkaClusterIdentity, "status.hub2", 11))
	databaseTransports = append(databaseTransports, generateTransport(kafkaClusterIdentity, "status", 9))
	databaseTransports = append(databaseTransports, generateTransport(kafkaClusterIdentity, "spec", 9))
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
		if *offset.Topic == "spec" {
			t.Fatalf("the topic %s shouldn't be selected", "spec")
		}
		count++
	}
	assert.Equal(t, 3, count)
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
