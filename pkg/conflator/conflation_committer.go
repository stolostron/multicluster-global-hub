package conflator

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"gorm.io/gorm/clause"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

const KafkaPartitionDelimiter = "@"

func positionKey(topic string, partition int32) string {
	return fmt.Sprintf("%s@%d", topic, partition)
}

type ConflationCommitter struct {
	log                    logr.Logger
	transportMetadatasFunc metadata.GetBundleStatusesFunc
	committedPositions     map[string]int64
}

func NewKafkaConflationCommitter(getTransportMetadatasFunc metadata.GetBundleStatusesFunc) *ConflationCommitter {
	return &ConflationCommitter{
		log:                    ctrl.Log.WithName("kafka-conflation-committer"),
		transportMetadatasFunc: getTransportMetadatasFunc,
		committedPositions:     map[string]int64{},
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
	transportMetadatas := k.transportMetadatasFunc()

	transPositions := metadataToCommit(transportMetadatas)

	databaseTransports := []models.Transport{}
	for key, transPosition := range transPositions {
		// skip request if already committed this offset
		committedOffset, found := k.committedPositions[key]
		if found && committedOffset >= int64(transPosition.Offset) {
			continue
		}

		k.log.V(2).Info("commit offset to database", "topic@partition", key, "offset", transPosition.Offset)
		payload, err := json.Marshal(metadata.TransportPosition{
			OwnerIdentiy: transPosition.OwnerIdentiy,
			Topic:        transPosition.Topic,
			Partition:    transPosition.Partition,
			Offset:       int64(transPosition.Offset),
		})
		if err != nil {
			return err
		}
		databaseTransports = append(databaseTransports, models.Transport{
			Name:    transPosition.Topic,
			Payload: payload,
		})
		k.committedPositions[key] = int64(transPosition.Offset)
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

func metadataToCommit(metadataArray []metadata.BundleStatus) map[string]*metadata.TransportPosition {
	// extract the lowest per partition in the pending bundles, the highest per partition in the processed bundles
	pendingLowestMetadataMap := make(map[string]*metadata.TransportPosition)
	processedHighestMetadataMap := make(map[string]*metadata.TransportPosition)

	for _, transportMetadata := range metadataArray {
		bundleStatus, ok := transportMetadata.(metadata.TransportMetadata)
		if !ok {
			continue // shouldn't happen
		}

		metadata := bundleStatus.GetTransportMetadata()
		key := positionKey(metadata.Topic, metadata.Partition)

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
		processedHighestMetadataMap[key] = &metadata.TransportPosition{
			OwnerIdentiy: processedHighestMetadataMap[key].OwnerIdentiy,
			Topic:        processedHighestMetadataMap[key].Topic,
			Partition:    processedHighestMetadataMap[key].Partition,
			Offset:       processedHighestMetadataMap[key].Offset + 1,
		}
	}

	// patch the processed offsets map with that of the pending ones, so that if a topic@partition
	// has both types, the pending bundle gains priority (overwrites).
	for key, metadata := range pendingLowestMetadataMap {
		processedHighestMetadataMap[key] = metadata
	}

	return processedHighestMetadataMap
}
