package emitters

import (
	"context"
	"fmt"
	"sync"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	cecontext "github.com/cloudevents/sdk-go/v2/context"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

// EventEmitter manages event lists and supports batch/single sending modes
type EventEmitter struct {
	eventType    enum.EventType
	topic        string
	producer     transport.Producer
	targetObject func(client.Object) bool
	transformer  func(client.Object) interface{}
	events       []interface{}
	version      *eventversion.Version
	mu           sync.Mutex
	eventMode    constants.EventSendMode
}

func NewEventEmitter(
	eventType enum.EventType,
	producer transport.Producer,
	targetObject func(client.Object) bool,
	transformer func(client.Object) interface{},
	eventMode constants.EventSendMode,
	opts ...EventEmitterOption,
) *EventEmitter {
	e := &EventEmitter{
		eventType:    eventType,
		producer:     producer,
		targetObject: targetObject,
		transformer:  transformer,
		events:       make([]interface{}, 0),
		version:      eventversion.NewVersion(),
		eventMode:    eventMode,
	}

	for _, opt := range opts {
		opt(e)
	}

	return e
}

func (e *EventEmitter) EventType() string {
	return string(e.eventType)
}

func (e *EventEmitter) EventFilter() predicate.Predicate {
	return predicate.NewPredicateFuncs(e.targetObject)
}

func (e *EventEmitter) Update(obj client.Object) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.targetObject(obj) {
		return nil
	}

	event := e.transformer(obj)
	e.events = append(e.events, event)
	e.version.Incr()

	return nil
}

func (e *EventEmitter) Delete(obj client.Object) error {
	// Events are typically append-only
	return nil
}

func (e *EventEmitter) Resync(objects []client.Object) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, obj := range objects {
		if e.targetObject(obj) {
			event := e.transformer(obj)
			e.events = append(e.events, event)
		}
	}

	if len(e.events) > 0 {
		if err := e.sendEvents(); err != nil {
			return fmt.Errorf("failed to send events after resync: %w", err)
		}
		e.version.Next()
	}

	return nil
}

func (e *EventEmitter) Send() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(e.events) == 0 {
		return nil
	}

	if err := e.sendEvents(); err != nil {
		return err
	}
	e.version.Next()
	return nil
}

func (e *EventEmitter) sendEvents() error {
	switch e.eventMode {
	case constants.EventSendModeSingle:
		return e.sendEventsIndividually()
	default: // batch is default
		return e.sendEventBundle()
	}
}

func (e *EventEmitter) sendEventBundle() error {
	evt := cloudevents.NewEvent()
	evt.SetSource(configs.GetLeafHubName())
	evt.SetType(string(e.eventType))
	evt.SetExtension(eventversion.ExtVersion, e.version.String())
	evt.SetExtension(constants.ExtEventSendMode, string(constants.EventSendModeBatch))

	if err := evt.SetData(cloudevents.ApplicationJSON, e.events); err != nil {
		return fmt.Errorf("failed to set event data for bundle: %w", err)
	}

	ctx := context.Background()
	if e.topic != "" {
		ctx = cecontext.WithTopic(ctx, e.topic)
	}

	if err := e.producer.SendEvent(ctx, evt); err != nil {
		return fmt.Errorf("failed to send event bundle: %w", err)
	}

	log.Debugw("sent event bundle",
		"type", enum.ShortenEventType(string(e.eventType)),
		"count", len(e.events))

	e.events = e.events[:0]
	return nil
}

func (e *EventEmitter) sendEventsIndividually() error {
	ctx := context.Background()
	if e.topic != "" {
		ctx = cecontext.WithTopic(ctx, e.topic)
	}

	sentCount := 0
	for _, event := range e.events {
		evt := cloudevents.NewEvent()
		evt.SetSource(configs.GetLeafHubName())
		evt.SetType(string(e.eventType))
		evt.SetExtension(eventversion.ExtVersion, e.version.String())
		evt.SetExtension(constants.ExtEventSendMode, string(constants.EventSendModeSingle))

		if err := evt.SetData(cloudevents.ApplicationJSON, event); err != nil {
			return fmt.Errorf("failed to set event data for individual event: %w", err)
		}

		if err := e.producer.SendEvent(ctx, evt); err != nil {
			return fmt.Errorf("failed to send individual event (sent %d/%d): %w",
				sentCount, len(e.events), err)
		}
		sentCount++
	}

	log.Debugw("sent individual events",
		"type", enum.ShortenEventType(string(e.eventType)),
		"count", sentCount)

	e.events = e.events[:0]
	return nil
}

type EventEmitterOption func(*EventEmitter)
