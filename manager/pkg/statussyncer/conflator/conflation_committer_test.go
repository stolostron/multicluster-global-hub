package conflator

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/conflator/metadata"
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
	assert.Equal(t, metadatas[positionKey("topic1", 0)].Offset, int64(4))
	assert.Equal(t, metadatas[positionKey("topic2", 0)].Offset, int64(14))
	assert.Equal(t, metadatas[positionKey("topic3", 0)].Offset, int64(6))
}

func getTransportMetadatas(topic string, processedOffsets []int64, unprocessedOffsets []int64) []ConflationMetadata {
	transportMetadatas := make([]ConflationMetadata, len(unprocessedOffsets)+len(processedOffsets))
	for _, offset := range unprocessedOffsets {
		transportMetadatas = append(transportMetadatas,
			metadata.NewThresholdMetadataFromPosition(3,
				&metadata.TransportPosition{
					Topic:     topic,
					Partition: 0,
					Offset:    offset,
				}))
	}
	for _, offset := range processedOffsets {
		transportMetadatas = append(transportMetadatas,
			metadata.NewThresholdMetadataFromPosition(0,
				&metadata.TransportPosition{
					Topic:     topic,
					Partition: 0,
					Offset:    offset,
				}))
	}
	return transportMetadatas
}
