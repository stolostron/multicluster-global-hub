package generic

import (
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/controller/syncers"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

var _ Emitter = &genericEmitter{}

type EmitterOption func(*genericEmitter)

func WithTopic(topic string) EmitterOption {
	return func(g *genericEmitter) {
		g.topic = topic
	}
}

func WithTweakFunc(tweakFunc func(client.Object)) EmitterOption {
	return func(g *genericEmitter) {
		g.tweakFunc = tweakFunc
	}
}

func WithShouldUpdate(shouldUpdate func(client.Object) bool) EmitterOption {
	return func(g *genericEmitter) {
		g.shouldUpdate = shouldUpdate
	}
}

func WithDependencyVersion(version *metadata.BundleVersion) EmitterOption {
	return func(g *genericEmitter) {
		g.dependencyVersion = version
	}
}

func WithVersion(version *metadata.BundleVersion) EmitterOption {
	return func(g *genericEmitter) {
		g.currentVersion = version
	}
}

type genericEmitter struct {
	eventType       enum.EventType
	payload         interface{}
	currentVersion  *metadata.BundleVersion
	lastSentVersion metadata.BundleVersion

	topic             string
	dependencyVersion *metadata.BundleVersion
	tweakFunc         func(client.Object)
	shouldUpdate      func(client.Object) bool
}

func NewGenericEmitter(
	eventType enum.EventType,
	payload interface{},
	opts ...EmitterOption,
) *genericEmitter {
	emitter := &genericEmitter{
		eventType:       eventType,
		payload:         payload,
		currentVersion:  metadata.NewBundleVersion(),
		lastSentVersion: *metadata.NewBundleVersion(),
	}
	emitter.applyOptions(opts...)
	// support resync
	syncers.SupportResyc(string(eventType), emitter.currentVersion)
	return emitter
}

func (e *genericEmitter) applyOptions(opts ...EmitterOption) {
	for _, fn := range opts {
		fn(e)
	}
}

func (h *genericEmitter) ShouldSend() bool {
	return h.currentVersion.NewerThan(&h.lastSentVersion)
}

func (h *genericEmitter) PostSend() {
	h.currentVersion.Next()
	h.lastSentVersion = *h.currentVersion
}

func (h *genericEmitter) Topic() string {
	return h.topic
}

func (h *genericEmitter) ShouldUpdate(obj client.Object) bool {
	toUpdate := true
	if h.shouldUpdate != nil {
		toUpdate = h.shouldUpdate(obj)
	}

	if toUpdate && h.tweakFunc != nil {
		h.tweakFunc(obj)
	}
	return toUpdate
}

func (h *genericEmitter) PostUpdate() {
	h.currentVersion.Incr()
}

// func (h *genericEventEmitter) Update(obj client.Object) {
// 	if h.tweakFunc != nil {
// 		h.tweakFunc(obj)
// 	}
// 	if h.payload.Update(obj) {
// 		h.currentVersion.Incr()
// 	}
// }

// func (h *genericEventEmitter) Delete(obj client.Object) {
// 	if h.payload.Delete(obj) {
// 		h.currentVersion.Incr()
// 	}
// }

func (g *genericEmitter) ToCloudEvent() (*cloudevents.Event, error) {
	e := cloudevents.NewEvent()
	e.SetSource(config.GetLeafHubName())
	e.SetType(string(g.eventType))
	e.SetExtension(metadata.ExtVersion, g.currentVersion.String())
	if g.dependencyVersion != nil {
		e.SetExtension(metadata.ExtDependencyVersion, g.dependencyVersion.String())
	}
	err := e.SetData(cloudevents.ApplicationJSON, g.payload)
	return &e, err
}
