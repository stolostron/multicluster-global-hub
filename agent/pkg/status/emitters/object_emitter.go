package emitters

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	cecontext "github.com/cloudevents/sdk-go/v2/context"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	genericbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var log = logger.DefaultZapLogger()

type ObjectEmitter struct {
	eventType enum.EventType
	topic     string
	producer  transport.Producer
	tweakFunc func(client.Object)
	// Predicate used by the controller-runtime to select target objects for this emitter
	objectPredicate predicate.Predicate
	// Filter to process only objects matched by the predicate for this emitter
	filter       func(client.Object) bool
	metadataFunc func(client.Object) *genericbundle.ObjectMetadata
	bundle       *genericbundle.GenericBundle[client.Object]
	version      *eventversion.Version
	mu           sync.Mutex
	keyFunc      func(client.Object) string
}

// NewObjectEmitter creates a new ObjectEmitter with the provided event type and producer.
func NewObjectEmitter(
	eventType enum.EventType,
	producer transport.Producer,
	opts ...EmitterOption,
) *ObjectEmitter {
	e := &ObjectEmitter{
		eventType: eventType,
		producer:  producer,
		version:   eventversion.NewVersion(),
		bundle:    genericbundle.NewGenericBundle[client.Object](),
		keyFunc: func(obj client.Object) string {
			return obj.GetNamespace() + "/" + obj.GetName()
		},
		filter: func(obj client.Object) bool {
			return true
		},
	}
	// apply the options
	for _, fn := range opts {
		fn(e)
	}
	return e
}

func (e *ObjectEmitter) EventType() string {
	return string(e.eventType)
}

func (e *ObjectEmitter) Predicate() predicate.Predicate {
	return e.objectPredicate
}

// Update modifies the bundle with the provided object.
// Returns an error if the update fails.
// Example cloudevents:
//
//	{
//	  "data": { "update": [obj1, obj2] },
//	  "extversion": "2.1",
//	  "type": "io.open-cluster-management.operator.multiclusterglobalhubs.managedcluster"
//	}
func (e *ObjectEmitter) Update(obj client.Object) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.filter(obj) {
		return nil
	}

	// if the object is in update array, update it
	for i, existingObj := range e.bundle.Update {
		if e.keyFunc(existingObj) == e.keyFunc(obj) {
			e.bundle.Update[i] = obj
			e.version.Incr()
			return nil
		}
	}

	return e.handleDeltaEvent(obj, e.bundle.AddUpdate)
}

// Delete removes the bundle associated with the provided object.
// Returns an error if the deletion fails.
// Example cloudevents:
//
//	{
//	  "data": { "delete": [obj1, obj2] },
//	  "extversion": "2.8",
//	  "type": "io.open-cluster-management.operator.multiclusterglobalhubs.managedcluster"
//	}
func (e *ObjectEmitter) Delete(obj client.Object) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.filter(obj) {
		return nil
	}

	tweaked, err := applyTweak(obj, e.tweakFunc)
	if err != nil {
		return err
	}
	// if the object is in update array, remove it
	for i, existingObj := range e.bundle.Update {
		if e.keyFunc(existingObj) == e.keyFunc(tweaked) {
			e.bundle.Update = append(e.bundle.Update[:i], e.bundle.Update[i+1:]...)
			e.version.Incr()
			break
		}
	}

	// if already in delete array, do nothing
	for _, existingObj := range e.bundle.Delete {
		if (existingObj.Namespace + "/" + existingObj.Name) == e.keyFunc(tweaked) {
			return nil
		}
	}

	metadata := &genericbundle.ObjectMetadata{
		ID:        string(tweaked.GetUID()),
		Namespace: tweaked.GetNamespace(),
		Name:      tweaked.GetName(),
	}

	if e.metadataFunc != nil {
		metadata = e.metadataFunc(tweaked)
	}

	added, err := e.bundle.AddDelete(*metadata)
	if err != nil {
		return fmt.Errorf("failed to add delete event to bundle: %v", err)
	}
	if !added {
		log.Info("Delete bundle is full, sending current bundle before adding new object")
		if err := e.sendBundle(); err != nil {
			return err
		}
		// re-add
		added, err = e.bundle.AddDelete(*metadata)
		if err != nil {
			return fmt.Errorf("failed to re-add delete event to bundle: %v", err)
		}
		if !added {
			return fmt.Errorf("failed to re-add delete event to bundle for obj: %v ", tweaked)
		}
	}
	e.version.Incr()
	return nil
}

// Resync periodically reconciles the state of the given objects.
// Example cloudevents:
//
//	{
//	  "data": { "resync": [obj1, obj2] },
//	  "extversion": "3.1",
//	  "type": "io.open-cluster-management.operator.multiclusterglobalhubs.managedcluster"
//	}
//
//	{
//	  "data": { "resync": [obj3, obj4, obj5] },
//	  "extversion": "3.1",
//	  "type": "io.open-cluster-management.operator.multiclusterglobalhubs.managedcluster"
//	}
func (e *ObjectEmitter) Resync(objects []client.Object) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	metas := make([]genericbundle.ObjectMetadata, 0, len(objects))
	for _, obj := range objects {

		if !e.filter(obj) {
			continue
		}

		tweaked, err := applyTweak(obj, e.tweakFunc)
		if err != nil {
			return err
		}

		added, err := e.bundle.AddResync(tweaked)
		if err != nil {
			return err
		}
		if !added {
			log.Info("Resync bundle is full, sending current bundle before adding new object")
			if err := e.sendBundle(); err != nil {
				return err
			}
			// re - add
			added, err = e.bundle.AddResync(tweaked)
			if err != nil {
				return fmt.Errorf("failed to add event to resync bundle: %v", err)
			}
			if !added {
				return fmt.Errorf("failed to add event to resync bundle for obj: %v ", obj)
			}
		}
		tmp := genericbundle.ObjectMetadata{
			ID:        string(tweaked.GetUID()),
			Namespace: tweaked.GetNamespace(),
			Name:      tweaked.GetName(),
		}
		if e.metadataFunc != nil {
			tmp = *e.metadataFunc(tweaked)
		}
		metas = append(metas, tmp)
	}

	if err := e.sendBundle(); err != nil {
		return err
	}

	if err := e.bundle.AddResyncMetadata(metas); err != nil {
		return err
	}
	return e.sendBundle()
}

// Send triggers the emission of an event.
// It sends the current bundle as a CloudEvent and increments the version.
// Returns an error if sending fails.
// Example cloudevents:
//
//	{
//	  "data": { "create": [obj1, obj2], "update": [obj3], "delete": [obj4] },
//	  "extversion": "2.9",
//	  "type": "io.open-cluster-management.operator.multiclusterglobalhubs.managedcluster"
//	}
//	{
//	  "data": { "resync": [obj1, obj2, obj3]},
//	  "extversion": "3.1	",
//	  "type": "io.open-cluster-management.operator.multiclusterglobalhubs.managedcluster"
//	}
//
// ...
//
//	{
//	  "data": { "resync": [obj5, obj6]},
//	  "extversion": "3.1	",
//	  "type": "io.open-cluster-management.operator.multiclusterglobalhubs.managedcluster"
//	}
//
// {
// "data": {"resync_metadata": [metadata1, metadata2, metadat3, metadata4, metadata5, metadata6]},
// "extversion": "3.1",
// "type": "io.open-cluster-management.operator.multiclusterglobalhubs.managedcluster",
// }
func (e *ObjectEmitter) Send() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.sendBundle(); err != nil {
		return err
	}
	e.version.Next()
	return nil
}

// sendBundle sends the current bundle as a CloudEvent.
// If updateGeneration is true, it will increment the version of generation version.
func (e *ObjectEmitter) sendBundle() error {
	if e.bundle.IsEmpty() {
		return nil
	}
	evt := cloudevents.NewEvent()
	evt.SetSource(configs.GetLeafHubName())
	evt.SetType(string(e.eventType))
	evt.SetExtension(eventversion.ExtVersion, e.version.String())

	payload, err := json.Marshal(e.bundle)
	if err != nil {
		return fmt.Errorf("failed to marshal bundle: %v", err)
	}
	err = evt.SetData(cloudevents.ApplicationJSON, payload)
	if err != nil {
		return fmt.Errorf("failed to load bundle into cloudevent: %v", err)
	}

	log.Debugf("sending cloudevents: %s", evt)

	ctx := context.Background()
	if e.topic != "" {
		ctx = cecontext.WithTopic(ctx, e.topic)
	}
	if err = e.producer.SendEvent(ctx, evt); err != nil {
		return fmt.Errorf("failed to send event: %v", err)
	}
	log.Debugw("sending",
		"type", enum.ShortenEventType(string(e.eventType)),
		"create", len(e.bundle.Create),
		"update", len(e.bundle.Update),
		"delete", len(e.bundle.Delete),
		"resync", len(e.bundle.Resync),
		"resync_metadata", len(e.bundle.ResyncMetadata))
	e.bundle.Clean()
	return nil
}

type EmitterOption func(*ObjectEmitter)

func WithMetadataFunc(metadataFunc func(client.Object) *genericbundle.ObjectMetadata) EmitterOption {
	return func(e *ObjectEmitter) {
		e.metadataFunc = metadataFunc
	}
}

func WithTweakFunc(tweakFunc func(client.Object)) EmitterOption {
	return func(e *ObjectEmitter) {
		e.tweakFunc = tweakFunc
	}
}

func WithPredicateFunc(eventFilter predicate.Predicate) EmitterOption {
	return func(e *ObjectEmitter) {
		e.objectPredicate = eventFilter
	}
}

func WithFilterFunc(filter func(client.Object) bool) EmitterOption {
	return func(e *ObjectEmitter) {
		e.filter = filter
	}
}

func WithTopic(topic string) EmitterOption {
	return func(e *ObjectEmitter) {
		e.topic = topic
	}
}

func WithVersion(version *eventversion.Version) EmitterOption {
	return func(e *ObjectEmitter) {
		e.version = version
	}
}

func applyTweak(obj client.Object, tweak func(client.Object)) (client.Object, error) {
	if tweak == nil {
		return obj, nil
	}
	if rObj, ok := obj.(runtime.Object); ok {
		if copyObj, ok := rObj.DeepCopyObject().(client.Object); ok {
			tweak(copyObj)
			return copyObj, nil
		}
	}
	return nil, fmt.Errorf("failed to deepcopy object for tweak")
}

func (e *ObjectEmitter) handleDeltaEvent(obj client.Object, addFunc func(client.Object) (bool, error)) error {
	tweaked, err := applyTweak(obj, e.tweakFunc)
	if err != nil {
		return err
	}

	added, err := addFunc(tweaked)
	if err != nil {
		return err
	}
	if added {
		e.version.Incr()
		return nil
	} else {
		if err := e.sendBundle(); err != nil {
			return err
		}
		// re-add the object after sending the bundle
		added, err = addFunc(tweaked)
		if err != nil {
			return fmt.Errorf("failed to add event to bundle: %v", err)
		}
		// Rare case: add the safeguard to ensure the object is added to the bundle successfully.
		if !added {
			return fmt.Errorf("failed to add event to bundle after resend: %v ", obj)
		}
		e.version.Incr()
	}

	return nil
}
