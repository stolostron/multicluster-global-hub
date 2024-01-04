package conflator

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/go-logr/logr"
	"gorm.io/gorm/clause"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata/status"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

const KafkaPartitionDelimiter = "@"

func positionKey(topic string, partition int32) string {
	return fmt.Sprintf("%s@%d", topic, partition)
}

type ConflationCommitter struct {
	log                logr.Logger
	conflationManager  *ConflationManager
	committedPositions map[string]int64
}

func NewKafkaConflationCommitter(cm *ConflationManager) *ConflationCommitter {
	return &ConflationCommitter{
		log:                ctrl.Log.WithName("kafka-conflation-committer"),
		conflationManager:  cm,
		committedPositions: map[string]int64{},
	}
}

func (k *ConflationCommitter) Start(ctx context.Context) error {
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

func (k *ConflationCommitter) commit() error {
	// get metadata (both pending and processed)
	transportMetadatas := k.conflationManager.GetTransportMetadatas()

	// extract the lowest per partition in the pending bundles, the highest per partition in the processed bundles
	pendingMetadataToCommit, processedMetadataToCommit := k.filterMetadata(transportMetadatas)

	// patch the processed offsets map with that of the pending ones, so that if a topic@partition
	// has both types, the pending bundle gains priority (overwrites).
	for key, metadata := range pendingMetadataToCommit {
		processedMetadataToCommit[key] = metadata
	}

	databaseTransports := []models.Transport{}
	for key, metadata := range processedMetadataToCommit {
		// skip request if already committed this offset
		committedOffset, found := k.committedPositions[key]
		if found && committedOffset >= int64(metadata.Offset) {
			continue
		}

		k.log.Info("commit offset to database", "topic@partition", key, "offset", metadata.Offset)
		payload, err := json.Marshal(models.KafkaPosition{
			Topic:     *metadata.Topic,
			Partition: metadata.Partition,
			Offset:    int64(metadata.Offset),
		})
		if err != nil {
			return err
		}
		databaseTransports = append(databaseTransports, models.Transport{
			Name:    *metadata.Topic,
			Payload: payload,
		})
		k.committedPositions[key] = int64(metadata.Offset)
	}

	db := database.GetGorm()
	if len(databaseTransports) > 0 {
		err := db.Clauses(clause.OnConflict{
			UpdateAll: true,
		}).CreateInBatches(databaseTransports, 100).Error
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *ConflationCommitter) filterMetadata(metadataArray []metadata.BundleStatus) (
	map[string]*kafka.TopicPartition,
	map[string]*kafka.TopicPartition,
) {
	// assumes all are in the same topic.
	pendingLowestMetadataMap := make(map[string]*kafka.TopicPartition)
	processedHighestMetadataMap := make(map[string]*kafka.TopicPartition)

	for _, transportMetadata := range metadataArray {
		bundleStatus, ok := transportMetadata.(*status.ThresholdBundleStatus)
		if !ok {
			continue // shouldn't happen
		}

		metadata := bundleStatus.GetTransportMetadata()
		key := positionKey(*metadata.Topic, metadata.Partition)

		if !bundleStatus.Processed() {
			// this belongs to a pending bundle, update the lowest-offsets-map
			lowestMetadata, found := pendingLowestMetadataMap[key]
			if found && metadata.Offset >= lowestMetadata.Offset {
				continue // offset is not the lowest in partition, skip
			}

			pendingLowestMetadataMap[key] = metadata
		} else {
			// this belongs to a processed bundle, update the highest-offsets-map
			highestMetadata, found := processedHighestMetadataMap[key]
			if found && metadata.Offset <= highestMetadata.Offset {
				continue // offset is not the highest in partition, skip
			}

			processedHighestMetadataMap[key] = metadata
		}
	}

	// increment processed offsets so they are not re-read on kafka consumer restart
	for key := range processedHighestMetadataMap {
		processedHighestMetadataMap[key] = &kafka.TopicPartition{
			Topic:     processedHighestMetadataMap[key].Topic,
			Partition: processedHighestMetadataMap[key].Partition,
			Offset:    processedHighestMetadataMap[key].Offset + 1,
		}
	}

	return pendingLowestMetadataMap, processedHighestMetadataMap
}
