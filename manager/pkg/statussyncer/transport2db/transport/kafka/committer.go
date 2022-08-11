package kafka

import (
	"context"
	"fmt"
	"time"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/go-logr/logr"

	"github.com/stolostron/multicluster-globalhub/manager/pkg/statussyncer/transport2db/transport"
	kafkaconsumer "github.com/stolostron/multicluster-globalhub/pkg/kafka/kafka-consumer"
)

// newCommitter returns a new instance of committer.
func newCommitter(committerInterval time.Duration, topic string, client *kafkaconsumer.KafkaConsumer,
	getBundlesMetadataFunc transport.GetBundlesMetadataFunc, log logr.Logger,
) (*committer, error) {
	return &committer{
		log:                    log,
		topic:                  topic,
		client:                 client,
		getBundlesMetadataFunc: getBundlesMetadataFunc,
		commitsMap:             make(map[int32]kafka.Offset),
		interval:               committerInterval,
	}, nil
}

// committer is responsible for committing offsets to transport.
type committer struct {
	log                    logr.Logger
	topic                  string
	client                 *kafkaconsumer.KafkaConsumer
	getBundlesMetadataFunc transport.GetBundlesMetadataFunc
	commitsMap             map[int32]kafka.Offset // map of partition -> offset
	interval               time.Duration
}

// start runs the committer instance.
func (c *committer) start(ctx context.Context) {
	go c.periodicCommit(ctx)
}

func (c *committer) periodicCommit(ctx context.Context) {
	ticker := time.NewTicker(c.interval)

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C: // wait for next time interval
			// get metadata (both pending and processed)
			bundlesMetadata := c.getBundlesMetadataFunc()
			// extract the lowest per partition in the pending bundles, the highest per partition in the
			// processed bundles
			pendingOffsetsToCommit, processedOffsetsToCommit :=
				c.filterMetadataPerPartition(bundlesMetadata)
			// patch the processed offsets map with that of the pending ones, so that if a partition
			// has both types, the pending bundle gains priority (overwrites).
			for partition, offset := range pendingOffsetsToCommit {
				processedOffsetsToCommit[partition] = offset
			}

			if err := c.commitOffsets(processedOffsetsToCommit); err != nil {
				c.log.Error(err, "commit offsets failed")
			}
		}
	}
}

func (c *committer) filterMetadataPerPartition(metadataArray []transport.BundleMetadata) (map[int32]kafka.Offset,
	map[int32]kafka.Offset,
) {
	// assumes all are in the same topic.
	pendingLowestOffsetsMap := make(map[int32]kafka.Offset)
	processedHighestOffsetsMap := make(map[int32]kafka.Offset)

	for _, transportMetadata := range metadataArray {
		metadata, ok := transportMetadata.(*bundleMetadata)
		if !ok {
			continue // shouldn't happen
		}

		if !metadata.Processed() {
			// this belongs to a pending bundle, update the lowest-offsets-map
			lowestOffset, found := pendingLowestOffsetsMap[metadata.partition]
			if found && metadata.offset >= lowestOffset {
				continue // offset is not the lowest in partition, skip
			}

			pendingLowestOffsetsMap[metadata.partition] = metadata.offset
		} else {
			// this belongs to a processed bundle, update the highest-offsets-map
			highestOffset, found := processedHighestOffsetsMap[metadata.partition]
			if found && metadata.offset <= highestOffset {
				continue // offset is not the highest in partition, skip
			}

			processedHighestOffsetsMap[metadata.partition] = metadata.offset
		}
	}

	// increment processed offsets so they are not re-read on kafka consumer restart
	for partition := range processedHighestOffsetsMap {
		processedHighestOffsetsMap[partition]++
	}

	return pendingLowestOffsetsMap, processedHighestOffsetsMap
}

// commitOffsets commits the given offsets per partition mapped.
func (c *committer) commitOffsets(offsets map[int32]kafka.Offset) error {
	for partition, offset := range offsets {
		// skip request if already committed this offset
		if committedOffset, found := c.commitsMap[partition]; found {
			if committedOffset >= offset {
				continue
			}
		}

		topicPartition := kafka.TopicPartition{
			Topic:     &c.topic,
			Partition: partition,
			Offset:    offset,
		}

		if _, err := c.client.Consumer().CommitOffsets([]kafka.TopicPartition{
			topicPartition,
		}); err != nil {
			return fmt.Errorf("failed to commit offset, stopping bulk commit - %w", err)
		}

		// update commitsMap
		c.commitsMap[partition] = offset
	}

	return nil
}
