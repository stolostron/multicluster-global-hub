package emitters

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	cecontext "github.com/cloudevents/sdk-go/v2/context"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	genericbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
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
	filter        func(client.Object) bool // filter events by the predicate
	// transform converts a object to an event object. It can return nil to indicate that the object should be skipped.
	// It can return a list of events(like replicated policy events) or a single event.
	transform func(client.Client, client.Object) interface{}
	postSend  func([]interface{}) error
	events    []interface{}
	version   *eventversion.Version
	mu        sync.Mutex
}

func NewEventEmitter(
	eventType enum.EventType,
	producer transport.Producer,
	runtimeClient client.Client,
	filter func(client.Object) bool,
	transform func(client.Client, client.Object) interface{},
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
		filter:        filter,
		transform:     transform,
		postSend:      nil, // Will be set by options if needed
		events:        make([]interface{}, 0),
		version:       eventversion.NewVersion(),
	}

	for _, opt := range opts {
		opt(e)
	}

	return e
}

func (e *EventEmitter) EventType() string {
	return string(e.eventType)
}

func (e *EventEmitter) Predicate() predicate.Predicate {
	return predicate.NewPredicateFuncs(e.filter)
}

func (e *EventEmitter) Update(obj client.Object) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.filter(obj) {
		return nil
	}

	event := e.transform(e.runtimeClient, obj)
	if event == nil {
		return nil
	}

	newEvents := toSlice(event)
	for _, evt := range newEvents {
		added, err := e.addEvent(evt)
		if err != nil {
			return err
		}
		if !added {
			// Bundle is full, send current bundle and retry
			log.Infow("Update: bundle is full, sending current bundle before adding new event", "count", len(e.events))
			if err := e.sendEvents(false); err != nil {
				return err
			}
			// Re-add the event
			added, err = e.addEvent(evt)
			if err != nil {
				return fmt.Errorf("failed to re-add event after sending: %w", err)
			}
			if !added {
				return fmt.Errorf("failed to add event to empty bundle")
			}
		}
	}
	e.version.Incr()
	return nil
}

// addEvent adds an event to the bundle.
// In batch mode: checks bundle size and returns false if size limit exceeded (caller should send and retry).
// In single mode: directly adds event without size checking (events are sent individually anyway).
// Returns (true, nil) if event was added successfully.
// Returns (false, nil) if bundle is full and needs to be sent first (batch mode only).
// Returns (false, error) if an error occurred.
func (e *EventEmitter) addEvent(event interface{}) (bool, error) {
	// In single mode, events are sent individually, so no bundle size checking needed
	if configs.GetAgentConfig().EventMode == string(constants.EventSendModeSingle) {
		e.events = append(e.events, event)
		return true, nil
	}

	// Batch mode: check bundle size before adding
	wasEmptyBeforeAdd := len(e.events) == 0
	e.events = append(e.events, event)

	size, err := e.bundleSize()
	if err != nil {
		e.events = e.events[:len(e.events)-1]
		return false, fmt.Errorf("failed to calculate bundle size: %w", err)
	}

	if size > genericbundle.MaxBundleBytes {
		e.events = e.events[:len(e.events)-1]

		if wasEmptyBeforeAdd {
			return false, fmt.Errorf("single event exceeds bundle size limit: %d bytes", size)
		}

		return false, nil
	}
	return true, nil
}

// bundleSize returns the JSON-encoded size of current events in bytes
func (e *EventEmitter) bundleSize() (int, error) {
	data, err := json.Marshal(e.events)
	if err != nil {
		return 0, err
	}
	return len(data), nil
}

func (e *EventEmitter) Delete(obj client.Object) error {
	// Events are typically append-only
	return nil
}

func (e *EventEmitter) Resync(objects []client.Object) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, obj := range objects {
		if !e.filter(obj) {
			continue
		}
		event := e.transform(e.runtimeClient, obj)
		if event == nil {
			continue
		}
		for _, evt := range toSlice(event) {
			added, err := e.addEvent(evt)
			if err != nil {
				return fmt.Errorf("failed to add event during resync: %w", err)
			}
			if !added {
				// Bundle is full, send current bundle and retry
				log.Infow("Resync bundle is full, sending current bundle before adding new event", "count", len(e.events))
				if err := e.sendEvents(false); err != nil {
					return err
				}
				// Re-add the event
				added, err = e.addEvent(evt)
				if err != nil {
					return fmt.Errorf("failed to re-add event during resync: %w", err)
				}
				if !added {
					return fmt.Errorf("failed to add event to empty bundle during resync")
				}
			}
		}
	}

	if err := e.sendEvents(false); err != nil {
		return fmt.Errorf("failed to send events after resync: %w", err)
	}
	return nil
}

func (e *EventEmitter) Send() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.sendEvents(true); err != nil {
		log.Errorw("failed to send events", "error", err)
		return err
	}

	e.version.Next()
	return nil
}

// sendEvents sends the current events as a CloudEvent. and clears the events after sending.
func (e *EventEmitter) sendEvents(withPostSend bool) error {
	if len(e.events) == 0 {
		return nil
	}
	log.Debugw("before send events", "events", e.events, "version", e.version.String())

	var err error
	switch configs.GetAgentConfig().EventMode {
	case string(constants.EventSendModeSingle):
		err = e.sendEventsIndividually()
	default: // batch is default
		err = e.sendEventBundle()
	}
	if err != nil {
		return err
	}

	if withPostSend && e.postSend != nil {
		if err := e.postSend(e.events); err != nil {
			log.Errorw("postSend callback failed", "error", err)
			// Don't return error as events were already sent successfully
		}
	}

	// Clear events after successful send and postSend
	e.events = e.events[:0]

	log.Debugw("after send events", "events", e.events, "version", e.version.String())
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
	evt.SetExtension(constants.CloudEventExtensionSendMode, string(mode))
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

func toSlice(v interface{}) []interface{} {
	if v == nil {
		return nil
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
		result := make([]interface{}, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			result[i] = rv.Index(i).Interface()
		}
		return result
	}
	return []interface{}{v}
}
