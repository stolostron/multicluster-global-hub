package consumer

import (
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata/status"
)

// NewBundleMetadata returns a new instance of BundleMetadata.
func NewBundleMetadata(partition int32, offset kafka.Offset) *bundleMetadata {
	return &bundleMetadata{
		GenericBundleStatus: status.NewGenericBundleStatus(),
		partition:           partition,
		offset:              offset,
	}
}

// bundleMetadata wraps the info required for the associated bundle to be used for committing purposes.
type bundleMetadata struct {
	*status.GenericBundleStatus
	partition int32
	offset    kafka.Offset
}
