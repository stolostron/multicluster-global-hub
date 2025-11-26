package conflator

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm/clause"

	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/config"
)

type MetadataFunc func() []ConflationMetadata

const KafkaPartitionDelimiter = "@"

func positionKey(topic string, partition int32) string {
	return fmt.Sprintf("%s@%d", topic, partition)
}

type ConflationCommitter struct {
	log                        *zap.SugaredLogger
	retrieveMetadataFunc       MetadataFunc
	committedPositions         map[string]int64 // topic@partition -> offset
	committedPositionsMu       sync.RWMutex
	lastCommittedKafkaIdentity string
}

func NewKafkaConflationCommitter(metadataFunc MetadataFunc) *ConflationCommitter {
	return &ConflationCommitter{
		log:                        logger.DefaultZapLogger(),
		retrieveMetadataFunc:       metadataFunc,
		committedPositions:         map[string]int64{},
		lastCommittedKafkaIdentity: config.GetKafkaOwnerIdentity(),
	}
}

func (k *ConflationCommitter) Start(ctx context.Context) error {
	go func() {
		ticker := time.NewTicker(time.Second * 10)

		defer ticker.Stop()

		for {
			select {
			case <-ticker.C: // wait for next time interval
				err := k.commit()
				if err != nil {
					k.log.Infow("failed to commit offset", "error", err)
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
	// transPositions := metadataToCommit(transportMetadatas)
	transPositions := k.getPositionsToCommit()
	if len(transPositions) == 0 {
		return nil
	}

	databaseTransports := []models.Transport{}
	for _, transPosition := range transPositions {
		k.log.Debugw("commit offset to database", "topic@partition",
			positionKey(transPosition.Topic, transPosition.Partition), "offset", transPosition.Offset)
		payload, err := json.Marshal(transport.EventPosition{
			OwnerIdentity: config.GetKafkaOwnerIdentity(),
			Topic:         transPosition.Topic,
			Partition:     transPosition.Partition,
			Offset:        int64(transPosition.Offset),
		})
		if err != nil {
			return err
		}
		databaseTransports = append(databaseTransports, models.Transport{
			Name:    transPosition.Topic,
			Payload: payload,
		})
	}

	db := database.GetGorm()
	conn := database.GetConn()

	err := database.Lock(conn)
	defer database.Unlock(conn)
	if err != nil {
		return err
	}
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

func (k *ConflationCommitter) getPositionsToCommit() []*transport.EventPosition {
	k.committedPositionsMu.Lock()
	defer k.committedPositionsMu.Unlock()

	transportPositionMap := make(map[string]*transport.EventPosition)
	if k.lastCommittedKafkaIdentity != config.GetKafkaOwnerIdentity() {
		// if the kafka identity is changed, clear the committed history to start from the beginning
		k.committedPositions = make(map[string]int64)
		k.lastCommittedKafkaIdentity = config.GetKafkaOwnerIdentity()
		k.log.Infow("kafka identity changed, clear the committed history to start from the beginning")
		return nil
	}
	for _, metadata := range k.retrieveMetadataFunc() {
		if metadata == nil {
			continue
		}
		position := metadata.TransportPosition()
		if position == nil {
			continue
		}
		key := positionKey(position.Topic, position.Partition)
		committedOffset, found := k.committedPositions[key]
		if found && committedOffset >= int64(position.Offset) {
			continue
		}
		transportPositionMap[key] = position
		k.committedPositions[key] = int64(position.Offset)
	}
	if len(transportPositionMap) == 0 {
		return nil
	}
	transportPositionSlice := make([]*transport.EventPosition, 0, len(transportPositionMap))
	for _, position := range transportPositionMap {
		transportPositionSlice = append(transportPositionSlice, position)
	}
	return transportPositionSlice
}

func metadataToCommit(metadataArray []ConflationMetadata) map[string]*transport.EventPosition {
	// extract the lowest per partition in the pending bundles, the highest per partition in the processed bundles
	pendingLowestMetadataMap := make(map[string]*transport.EventPosition)
	processedHighestMetadataMap := make(map[string]*transport.EventPosition)

	for _, metadata := range metadataArray {
		if metadata == nil {
			continue
		}

		// metadata := bundleStatus.GetTransportMetadata()
		position := metadata.TransportPosition()
		key := positionKey(position.Topic, position.Partition)

		if !metadata.Processed() {
			// this belongs to a pending bundle, update the lowest-offsets-map
			lowestMetadata, found := pendingLowestMetadataMap[key]
			if found && position.Offset >= lowestMetadata.Offset {
				continue // offset is not the lowest in partition, skip
			}

			pendingLowestMetadataMap[key] = position
		} else {
			// this belongs to a processed bundle, update the highest-offsets-map
			highestMetadata, found := processedHighestMetadataMap[key]
			if found && position.Offset <= highestMetadata.Offset {
				continue // offset is not the highest in partition, skip
			}

			processedHighestMetadataMap[key] = position
		}
	}

	// increment processed offsets so they are not re-read on kafka consumer restart
	for key := range processedHighestMetadataMap {
		processedHighestMetadataMap[key] = &transport.EventPosition{
			OwnerIdentity: processedHighestMetadataMap[key].OwnerIdentity,
			Topic:         processedHighestMetadataMap[key].Topic,
			Partition:     processedHighestMetadataMap[key].Partition,
			Offset:        processedHighestMetadataMap[key].Offset + 1,
		}
	}

	// patch the processed offsets map with that of the pending ones, so that if a topic@partition
	// has both types, the pending bundle gains priority (overwrites).
	for key, metadata := range pendingLowestMetadataMap {
		processedHighestMetadataMap[key] = metadata
	}

	return processedHighestMetadataMap
}
