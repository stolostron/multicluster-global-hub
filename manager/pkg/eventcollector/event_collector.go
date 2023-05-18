package eventcollector

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Shopify/sarama"
	"github.com/go-logr/logr"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/resmoio/kubernetes-event-exporter/pkg/kube"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/eventcollector/processor"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/consumer"
)

func AddEventCollector(ctx context.Context, mgr ctrl.Manager, kafkaConfig *transport.KafkaConfig,
	pool *pgxpool.Pool,
) error {
	messageChan := make(chan *sarama.ConsumerMessage)
	// add the event consumer to manager
	eventConsumer, err := consumer.NewSaramaConsumer(ctx, kafkaConfig, messageChan)
	if err != nil {
		return fmt.Errorf("failed to create event consumer: %w", err)
	}
	if err := mgr.Add(eventConsumer); err != nil {
		return fmt.Errorf("failed to add event consumer: %w", err)
	}

	// create the event dispatcher
	eventDispatcher := newEventDispatcher(messageChan)

	// register event processors with the event dispatcher
	eventDispatcher.RegisterProcessor(policyv1.Kind, processor.NewPolicyProcessor(pool))

	// add the event dispatcher to manager
	if err := mgr.Add(eventDispatcher); err != nil {
		return fmt.Errorf("failed to add event dispatcher: %w", err)
	}

	return nil
}

type eventDispatcher struct {
	log         logr.Logger
	messageChan <-chan *sarama.ConsumerMessage
	processors  map[string]processor.EventProcessor
}

func newEventDispatcher(messageChan <-chan *sarama.ConsumerMessage) *eventDispatcher {
	return &eventDispatcher{
		log:         ctrl.Log.WithName("event-dispatcher"),
		messageChan: messageChan,
		processors:  make(map[string]processor.EventProcessor),
	}
}

func (e *eventDispatcher) Start(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			e.log.Info("context cancelled, exiting event dispatcher")
			return nil
		case message := <-e.messageChan:
			e.log.Info("received message", "message", message)
			event := &kube.EnhancedEvent{}
			if err := json.Unmarshal(message.Value, &event); err != nil {
				e.log.Error(err, "failed to unmarshal message to EnhancedEvent", "message", message)
				continue
			}
			processor, ok := e.processors[event.InvolvedObject.Kind]
			if !ok {
				e.log.Info("no event processor registered for object kind",
					"objectKind", event.InvolvedObject.Kind)
				continue
			}
			processor.Process(event)
		}
	}
}

func (e *eventDispatcher) RegisterProcessor(objectKind string, processor processor.EventProcessor) {
	e.processors[objectKind] = processor
}
