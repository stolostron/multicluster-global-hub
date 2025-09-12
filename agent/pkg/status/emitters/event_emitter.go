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
	eventType     enum.EventType
	topic         string
	producer      transport.Producer
	runtimeClient client.Client
	predicate     func(client.Object) bool // filter events by the predicate
	// transform converts a object to an event object. It can return nil to indicate that the object should be skipped.
	transform func(client.Client, client.Object) interface{}
	postSend  func([]interface{}) error
	events    []interface{}
	version   *eventversion.Version
	mu        sync.Mutex
	eventMode constants.EventSendMode
}

func NewEventEmitter(
	eventType enum.EventType,
	producer transport.Producer,
	runtimeClient client.Client,
	predicate func(client.Object) bool,
	transform func(client.Client, client.Object) interface{},
	eventMode constants.EventSendMode,
	opts ...EventEmitterOption,
) *EventEmitter {
	if eventType == "" {
		log.Error("EventEmitter created with empty event type")
		return nil
	}

	e := &EventEmitter{
		eventType:     eventType,
		producer:      producer,
		runtimeClient: runtimeClient,
		predicate:     predicate,
		transform:     transform,
		postSend:      nil, // Will be set by options if needed
		events:        make([]interface{}, 0),
		version:       eventversion.NewVersion(),
		eventMode:     eventMode,
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
	return predicate.NewPredicateFuncs(e.predicate)
}

func (e *EventEmitter) Update(obj client.Object) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.predicate(obj) {
		return nil
	}

	event := e.transform(e.runtimeClient, obj)
	if event != nil {
		e.events = append(e.events, event)
		e.version.Incr()
	}

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
		if e.predicate(obj) {
			event := e.transform(e.runtimeClient, obj)
			if event != nil {
				e.events = append(e.events, event)
			}
		}
	}

	if err := e.sendEvents(); err != nil {
		log.Errorw("failed to send events after resync", "error", err)
		return fmt.Errorf("failed to send events after resync: %w", err)
	}
	return nil
}

func (e *EventEmitter) Send() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := e.sendEvents(); err != nil {
		log.Errorw("failed to send events", "error", err)
		return err
	}
	return nil
}

func (e *EventEmitter) sendEvents() error {
	if len(e.events) == 0 {
		return nil
	}

	var err error
	switch e.eventMode {
	case constants.EventSendModeSingle:
		err = e.sendEventsIndividually()
	default: // batch is default
		err = e.sendEventBundle()
	}

	if err != nil {
		return err
	}

	if e.postSend != nil {
		if err := e.postSend(e.events); err != nil {
			log.Errorw("postSend callback failed", "error", err)
			// Don't return error as events were already sent successfully
		}
	}

	// Clear events after successful send and postSend
	e.events = e.events[:0]
	e.version.Next()
	return nil
}

func (e *EventEmitter) sendEventBundle() error {
	evt := e.createCloudEvent(constants.EventSendModeBatch)

	if err := evt.SetData(cloudevents.ApplicationJSON, e.events); err != nil {
		log.Errorw("failed to set event data for bundle", "error", err)
		return fmt.Errorf("failed to set event data for bundle: %w", err)
	}

	ctx := e.createContext()
	if err := e.producer.SendEvent(ctx, evt); err != nil {
		log.Errorw("failed to send event bundle", "error", err)
		return fmt.Errorf("failed to send event bundle: %w", err)
	}

	log.Debugw("sent event bundle",
		"type", enum.ShortenEventType(string(e.eventType)),
		"count", len(e.events))

	return nil
}

func (e *EventEmitter) sendEventsIndividually() error {
	ctx := e.createContext()
	sentCount := 0

	for _, event := range e.events {
		evt := e.createCloudEvent(constants.EventSendModeSingle)

		if err := evt.SetData(cloudevents.ApplicationJSON, event); err != nil {
			log.Errorw("failed to set event data for individual event", "error", err)
			return fmt.Errorf("failed to set event data for individual event: %w", err)
		}

		if err := e.producer.SendEvent(ctx, evt); err != nil {
			log.Errorw("failed to send individual event", "error", err, "sent", sentCount, "total", len(e.events))
			return fmt.Errorf("failed to send individual event (sent %d/%d): %w",
				sentCount, len(e.events), err)
		}
		sentCount++
	}

	log.Debugw("sent individual events",
		"type", enum.ShortenEventType(string(e.eventType)),
		"count", sentCount)

	return nil
}

// createCloudEvent creates a new CloudEvent with common fields set
func (e *EventEmitter) createCloudEvent(mode constants.EventSendMode) cloudevents.Event {
	evt := cloudevents.NewEvent()
	evt.SetSource(configs.GetLeafHubName())
	evt.SetType(string(e.eventType))
	evt.SetExtension(eventversion.ExtVersion, e.version.String())
	evt.SetExtension(constants.ExtEventSendMode, string(mode))
	return evt
}

// createContext creates a context with topic if configured
func (e *EventEmitter) createContext() context.Context {
	ctx := context.Background()
	if e.topic != "" {
		ctx = cecontext.WithTopic(ctx, e.topic)
	}
	return ctx
}

type EventEmitterOption func(*EventEmitter)

// WithPostSend adds a postSend callback that is called after events are successfully sent
func WithPostSend(postSend func([]interface{}) error) EventEmitterOption {
	return func(e *EventEmitter) {
		e.postSend = postSend
	}
}
