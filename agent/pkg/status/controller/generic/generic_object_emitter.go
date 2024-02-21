package generic

import (
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

var _ MultiEventEmitter = &genericMultiEventEmitter{}

type Option func(*genericMultiEventEmitter)

func WithTopic(topic string) Option {
	return func(p *genericMultiEventEmitter) {
		p.topic = topic
	}
}

func WithTweakFunc(tweakFunc func(obj client.Object)) Option {
	return func(p *genericMultiEventEmitter) {
		p.tweakFunc = tweakFunc
	}
}

func WithDependencyVersion(version *metadata.BundleVersion) Option {
	return func(p *genericMultiEventEmitter) {
		p.dependencyVersion = version
	}
}

func WithVersion(version *metadata.BundleVersion) Option {
	return func(p *genericMultiEventEmitter) {
		p.currentVersion = version
	}
}

type genericMultiEventEmitter struct {
	log             logr.Logger
	eventType       enum.EventType
	runtimeClient   client.Client
	currentVersion  *metadata.BundleVersion
	lastSentVersion metadata.BundleVersion

	predicate func(obj client.Object) bool
	payload   bundle.Payload

	topic             string
	tweakFunc         func(obj client.Object)
	dependencyVersion *metadata.BundleVersion
}

func NewGenericMultiEventEmitter(
	name string,
	eventType enum.EventType,
	runtimeClient client.Client,
	predicate func(obj client.Object) bool,
	payload bundle.Payload,
	opts ...Option,
) MultiEventEmitter {
	emitter := &genericMultiEventEmitter{
		log:             ctrl.Log.WithName(name),
		eventType:       eventType,
		runtimeClient:   runtimeClient,
		currentVersion:  metadata.NewBundleVersion(),
		lastSentVersion: *metadata.NewBundleVersion(),
		predicate:       predicate,
		payload:         payload,
	}
	emitter.applyOptions(opts...)
	return emitter
}

func (e *genericMultiEventEmitter) applyOptions(opts ...Option) {
	for _, fn := range opts {
		fn(e)
	}
}

// only for root policy
func (h *genericMultiEventEmitter) Predicate(obj client.Object) bool {
	return h.predicate(obj)
}

func (h *genericMultiEventEmitter) PreSend() bool {
	return h.currentVersion.NewerThan(&h.lastSentVersion)
}

func (h *genericMultiEventEmitter) Topic() string {
	return h.topic
}

func (h *genericMultiEventEmitter) Update(obj client.Object) {
	if h.tweakFunc != nil {
		h.tweakFunc(obj)
	}
	if h.payload.Update(obj) {
		h.currentVersion.Incr()
	}
}

func (h *genericMultiEventEmitter) Delete(obj client.Object) {
	if h.payload.Delete(obj) {
		h.currentVersion.Incr()
	}
}

func (h *genericMultiEventEmitter) ToCloudEvent() *cloudevents.Event {
	e := cloudevents.NewEvent()
	e.SetType(string(h.eventType))
	e.SetExtension(metadata.ExtVersion, h.currentVersion.String())
	if h.dependencyVersion != nil {
		e.SetExtension(metadata.ExtDependencyVersion, h.dependencyVersion.String())
	}
	err := e.SetData(cloudevents.ApplicationJSON, h.payload)
	if err != nil {
		h.log.Error(err, "failed to set the payload to cloudvents.Data")
	}
	return &e
}

func (h *genericMultiEventEmitter) PostSend() {
	// update version
	h.currentVersion.Next()
	h.lastSentVersion = *h.currentVersion
}
