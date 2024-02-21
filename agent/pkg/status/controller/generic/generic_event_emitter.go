package generic

import (
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/go-logr/logr"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	ctrl "sigs.k8s.io/controller-runtime"
)

var _ EventEmitter = &genericEventEmitter{}

type genericEventEmitter struct {
	log logr.Logger

	eventType       enum.EventType
	topic           string
	currentVersion  *metadata.BundleVersion
	lastSentVersion metadata.BundleVersion
	payload         bundle.Payload
}

func NewGenericEventEmitter(name string, eventType enum.EventType, topic string, payload bundle.Payload,
	version *metadata.BundleVersion) EventEmitter {
	return &genericEventEmitter{
		log:             ctrl.Log.WithName(name),
		eventType:       eventType,
		topic:           topic,
		currentVersion:  version,
		lastSentVersion: *metadata.NewBundleVersion(),
		payload:         payload,
	}
}

func (h *genericEventEmitter) PreSend() bool {
	return h.currentVersion.NewerThan(&h.lastSentVersion)
}

func (h *genericEventEmitter) Topic() string {
	return h.topic
}

func (h *genericEventEmitter) ToCloudEvent() *cloudevents.Event {
	e := cloudevents.NewEvent()
	e.SetType(string(h.eventType))
	e.SetExtension(metadata.ExtVersion, h.currentVersion.String())
	err := e.SetData(cloudevents.ApplicationJSON, h.payload)
	if err != nil {
		h.log.Error(err, "failed to set the payload to cloudvents.Data")
	}
	return &e
}

func (h *genericEventEmitter) PostSend() {
	// update version
	h.currentVersion.Next()
	h.lastSentVersion = *h.currentVersion
}
