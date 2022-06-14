package kafka

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/stolostron/hub-of-hubs/manager/pkg/statussyncer/transport2db/transport"
)

// newBundleMetadata returns a new instance of BundleMetadata.
func newBundleMetadata(partition int32, offset kafka.Offset) *bundleMetadata {
	return &bundleMetadata{
		BaseBundleMetadata: transport.NewBaseBundleMetadata(),
		partition:          partition,
		offset:             offset,
	}
}

// bundleMetadata wraps the info required for the associated bundle to be used for committing purposes.
type bundleMetadata struct {
	*transport.BaseBundleMetadata
	partition int32
	offset    kafka.Offset
}
