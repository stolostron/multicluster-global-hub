package conflator

import (
	"sync"

	cloudevents "github.com/cloudevents/sdk-go/v2"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

type ElementState struct {
	LastProcessedVersion  *version.Version
	LastProcessedMetadata ConflationMetadata
	HandlerLock           sync.Mutex
}

type hybridElement struct {
	eventType       string
	syncMode        enum.EventSyncMode
	handlerFunction EventHandleFunc
	elementState    *ElementState
}

func NewHybridElement(registration *ConflationRegistration) *hybridElement {
	return &hybridElement{
		eventType:       registration.eventType,
		syncMode:        registration.syncMode,
		handlerFunction: registration.handleFunc,
		elementState: &ElementState{
			LastProcessedVersion: version.NewVersion(),
		},
	}
}

func (e *hybridElement) Name() string {
	return e.eventType
}

// used for commit kafka offset
func (e *hybridElement) Metadata() ConflationMetadata {
	return e.elementState.LastProcessedMetadata
}

func (e *hybridElement) SyncMode() enum.EventSyncMode {
	return e.syncMode
}

func (e *hybridElement) Predicate(eventVersion *version.Version) bool {
	if eventVersion.InitGen() {
		e.elementState.LastProcessedVersion = version.NewVersion()
		log.Infow("resetting stream element version", "type", enum.ShortenEventType(e.eventType),
			"version", eventVersion)
	}
	log.Debugw("inserting event", "version", eventVersion)
	return eventVersion.NewerGenerationThan(e.elementState.LastProcessedVersion)
}

func (e *hybridElement) AddToReadyQueue(event *cloudevents.Event, metadata ConflationMetadata, cu *ConflationUnit) {
	cu.readyQueue.DeltaEventJobChan <- NewConflationJob(event, metadata, e.handlerFunction, cu, e.elementState)
}

// Success is to update the conflation element state after processing the event
func (e *hybridElement) PostProcess(metadata ConflationMetadata, err error) {
	if err != nil {
		log.Errorw("report error for the event", "error", err, "type", e.eventType, "version", metadata.Version())
		return
	}
}
