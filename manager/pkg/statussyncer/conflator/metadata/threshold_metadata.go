package metadata

import (
	"fmt"
	"strconv"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/types"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/kafka_confluent"
)

// using threshold to indicate the bundle processed status
// the count means the retry times.
// 0 - the default initial value
// 1, 2, 3 ... - the retry times of current bundle has been failed processed
// -1 means it processed successfully
type ThresholdMetadata struct {
	maxRetry int
	count    int

	// transport position
	kafkaPosition *TransportPosition

	// event
	eventType              string
	eventVersion           *metadata.BundleVersion
	eventDependencyVersion *metadata.BundleVersion
}

// the retry times(max) when the bundle has been failed processed
func NewThresholdMetadata(clusterIdentity string, max int, evt *cloudevents.Event) *ThresholdMetadata {
	log := ctrl.Log.WithName("event-metadata")

	topic, err := types.ToString(evt.Extensions()[kafka_confluent.KafkaTopicKey])
	if err != nil {
		log.Info("failed to parse topic from event", "error", err)
	}
	partition, err := types.ToInteger(evt.Extensions()[kafka_confluent.KafkaPartitionKey])
	if err != nil {
		log.Info("failed to parse partition from event", "error", err)
	}

	offsetStr, ok := evt.Extensions()[kafka_confluent.KafkaOffsetKey].(string)
	if !ok {
		log.Info("failed to get offset string from event", "offset", evt.Extensions()[kafka_confluent.KafkaOffsetKey])
	}

	offset, err := strconv.ParseInt(offsetStr, 10, 64)
	if err != nil {
		log.Info("failed to parse offset into int64 from event", "offset", offsetStr, "error", err)
	}

	eventVersion, err := getVersionFromEvent(evt, metadata.ExtVersion)
	if err != nil || eventVersion == nil {
		log.Error(err, "failed to parse event version")
		return nil
	}
	dependencyVersion, err := getVersionFromEvent(evt, metadata.ExtDependencyVersion)
	if err != nil {
		log.Error(err, "failed to parse dependencyVersion")
		return nil
	}

	return &ThresholdMetadata{
		maxRetry: max,
		count:    0,

		kafkaPosition: &TransportPosition{
			OwnerIdentity: clusterIdentity,
			Topic:         topic,
			Partition:     partition,
			Offset:        offset,
		},

		eventType:              evt.Type(),
		eventVersion:           eventVersion,
		eventDependencyVersion: dependencyVersion,
	}
}

func NewThresholdMetadataFromPosition(max int, pos *TransportPosition) *ThresholdMetadata {
	return &ThresholdMetadata{
		maxRetry:      max,
		count:         0,
		kafkaPosition: pos,
	}
}

// MarkAsProcessed function that marks the metadata as processed.
func (s *ThresholdMetadata) MarkAsProcessed() {
	s.count = -1
}

// Processed returns whether the bundle was processed or not.
func (s *ThresholdMetadata) Processed() bool {
	return s.count == -1 || s.count >= s.maxRetry
}

// MarkAsUnprocessed function that marks the metadata as processed.
func (s *ThresholdMetadata) MarkAsUnprocessed() {
	s.count++
}

func (s *ThresholdMetadata) TransportPosition() *TransportPosition {
	return s.kafkaPosition
}

func (s *ThresholdMetadata) Version() *metadata.BundleVersion {
	return s.eventVersion
}

func (s *ThresholdMetadata) DependencyVersion() *metadata.BundleVersion {
	return s.eventDependencyVersion
}

func (s *ThresholdMetadata) EventType() string {
	return s.eventType
}

func getVersionFromEvent(evt *cloudevents.Event, key string) (*metadata.BundleVersion, error) {
	val, ok := evt.Extensions()[key]
	if !ok {
		return nil, nil
	}
	version, ok := val.(string)
	if !ok {
		return nil, fmt.Errorf("the val: %s can't be convert to of string", val)
	}
	eventVersion, err := metadata.BundleVersionFrom(version)
	if err != nil {
		return nil, err
	}
	return eventVersion, nil
}
