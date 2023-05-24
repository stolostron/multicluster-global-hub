package processor

import "github.com/resmoio/kubernetes-event-exporter/pkg/kube"

type EventOffset struct {
	Topic     string
	Partition int32
	Offset    int64
}

type EventProcessor interface {
	Process(event *kube.EnhancedEvent, offset *EventOffset)
}

type OffsetManager interface {
	MarkOffset(topic string, partition int32, offset int64)
}
