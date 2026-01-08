package consumer

import (
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
	transportConfigChan := make(chan *transport.TransportInternalConfig)

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
