package status

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"gorm.io/gorm/clause"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

const KafkaPartitionDelimiter = "@"

var kafkaPositions map[string]KafkaPosition = map[string]KafkaPosition{} // for kafka: topic@partition: offset

func addToCommit(topic string, partition int32, offset int64) {
	key := fmt.Sprintf("%s@%d", topic, partition)
	p, ok := kafkaPositions[key]
	if ok && p.Offset >= offset {
		return
	}
	kafkaPositions[key] = KafkaPosition{
		Topic:     topic,
		Partition: partition,
		Offset:    offset,
	}
}

type KafkaPosition struct {
	Topic     string `json:"-"`
	Partition int32  `json:"partition"`
	Offset    int64  `json:"agent"`
}

type kafkaStatusCommitter struct {
	log                logr.Logger
	committedPositions map[string]int64
}

func NewKafkaStatusCommitter() *kafkaStatusCommitter {
	return &kafkaStatusCommitter{
		log:                ctrl.Log.WithName("kafka-status-committer"),
		committedPositions: map[string]int64{},
	}
}

func (k *kafkaStatusCommitter) Start(ctx context.Context) error {
	go func() {
		ticker := time.NewTicker(time.Second * 5)

		defer ticker.Stop()
		for {
			select {
			case <-ticker.C: // wait for next time interval
				err := k.commit()
				if err != nil {
					k.log.Info("failed to commit offset", "error", err)
				}
				// ticker.Reset()
			case <-ctx.Done():
				k.log.Info("context canceled, exiting committer...")
				return
			}
		}
	}()
	return nil
}

func (k *kafkaStatusCommitter) commit() error {
	db := database.GetGorm()
	batchStatus := []models.Transport{}
	for key, position := range kafkaPositions {
		committedOffset, ok := k.committedPositions[key]
		if ok && committedOffset >= position.Offset {
			continue
		}

		payload, err := json.Marshal(position)
		if err != nil {
			return err
		}
		batchStatus = append(batchStatus, models.Transport{
			Name:    key,
			Payload: payload,
		})
		k.committedPositions[key] = position.Offset
	}

	if len(batchStatus) > 0 {
		err := db.Clauses(clause.OnConflict{
			UpdateAll: true,
		}).CreateInBatches(batchStatus, 100).Error
		if err != nil {
			return err
		}
	}
	return nil
}
