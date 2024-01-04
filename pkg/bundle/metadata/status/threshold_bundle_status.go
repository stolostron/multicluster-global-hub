package status

import (
	"strconv"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/types"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/pkg/transport/kafka_confluent"
)

// using threshold to indicate the bundle processed status
// the count means the retry times.
// 0 - the default initial value
// 1, 2, 3 ... - the retry times of current bundle has been failed processed
// -1 means it processed successfully
type ThresholdBundleStatus struct {
	maxRetry int
	count    int

	// transport position
	kafkaPosition *kafka.TopicPartition
}

// the retry times(max) when the bundle has been failed processed
func NewThresholdBundleStatus(max int, evt cloudevents.Event) *ThresholdBundleStatus {
	log := ctrl.Log.WithName("threshold-bundle-status")

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
	if !ok {
		log.Info("failed to parse offset from event", "offset", offsetStr)
	}

	return &ThresholdBundleStatus{
		maxRetry: max,
		count:    0,

		kafkaPosition: &kafka.TopicPartition{
			Topic:     &topic,
			Partition: partition,
			Offset:    kafka.Offset(offset),
		},
	}
}

// MarkAsProcessed function that marks the metadata as processed.
func (s *ThresholdBundleStatus) MarkAsProcessed() {
	s.count = -1
}

// Processed returns whether the bundle was processed or not.
func (s *ThresholdBundleStatus) Processed() bool {
	return s.count == -1 || s.count >= s.maxRetry
}

// MarkAsUnprocessed function that marks the metadata as processed.
func (s *ThresholdBundleStatus) MarkAsUnprocessed() {
	s.count++
}

func (s *ThresholdBundleStatus) GetTransportMetadata() *kafka.TopicPartition {
	return s.kafkaPosition
}
