package consumer

import (
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
)

// NewBundleMetadata returns a new instance of BundleMetadata.
func NewBundleMetadata(partition int32, offset kafka.Offset) *bundleMetadata {
	return &bundleMetadata{
		BaseBundleMetadata: bundle.NewBaseBundleMetadata(),
		partition:          partition,
		offset:             offset,
	}
}

// bundleMetadata wraps the info required for the associated bundle to be used for committing purposes.
type bundleMetadata struct {
	*bundle.BaseBundleMetadata
	partition int32
	offset    kafka.Offset
}
