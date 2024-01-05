package conflator

import (
	"testing"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata/status"
	"github.com/stretchr/testify/assert"
)

func TestCommitOffset(t *testing.T) {
	transportMetadatas1 := getTransportMetadatas("topic1", []int64{1, 2, 3}, []int64{5, 6, 4, 7})
	transportMetadatas2 := getTransportMetadatas("topic2", []int64{12, 13, 11}, nil)
	transportMetadatas3 := getTransportMetadatas("topic3", nil, []int64{9, 6, 8, 7})

	transportMetadatas := append(append(transportMetadatas1, transportMetadatas2...), transportMetadatas3...)

	metadatas := metadataToCommit(transportMetadatas)
	assert.Greater(t, len(metadatas), 0)

	assert.Contains(t, metadatas, positionKey("topic1", 0))
	assert.Contains(t, metadatas, positionKey("topic2", 0))
	assert.Contains(t, metadatas, positionKey("topic3", 0))

	// get the offset to commit
	assert.Equal(t, metadatas[positionKey("topic1", 0)].Offset, kafka.Offset(4))
	assert.Equal(t, metadatas[positionKey("topic2", 0)].Offset, kafka.Offset(14))
	assert.Equal(t, metadatas[positionKey("topic3", 0)].Offset, kafka.Offset(6))
}

func getTransportMetadatas(topic string, processedOffsets []int64, unprocessedOffsets []int64) []metadata.BundleStatus {
	transportMetadatas := make([]metadata.BundleStatus, len(unprocessedOffsets)+len(processedOffsets))
	for _, offset := range unprocessedOffsets {
		transportMetadatas = append(transportMetadatas,
			status.NewThresholdBundleStatusFromPosition(3,
				&metadata.TransportPosition{
					Topic:     topic,
					Partition: 0,
					Offset:    offset,
				}))
	}
	for _, offset := range processedOffsets {
		transportMetadatas = append(transportMetadatas,
			status.NewThresholdBundleStatusFromPosition(0,
				&metadata.TransportPosition{
					Topic:     topic,
					Partition: 0,
					Offset:    offset,
				}))
	}
	return transportMetadatas
}
