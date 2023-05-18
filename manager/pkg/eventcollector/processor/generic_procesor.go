package processor

import "github.com/resmoio/kubernetes-event-exporter/pkg/kube"

type EventProcessor interface {
	Process(event *kube.EnhancedEvent)
}
