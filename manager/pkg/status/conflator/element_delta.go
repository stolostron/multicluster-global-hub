package conflator

import (
	cloudevents "github.com/cloudevents/sdk-go/v2"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator/dependency"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

type deltaElement struct {
	// state
	eventType            string
	syncMode             enum.EventSyncMode
	handlerFunction      EventHandleFunc
	dependency           *dependency.Dependency
	isInProcess          bool
	lastProcessedVersion *version.Version

	// the metadata of the event
	metadata ConflationMetadata
}

func NewDeltaElement(leafHubName string, registration *ConflationRegistration) *deltaElement {
	return &deltaElement{
		eventType:            registration.eventType,
		syncMode:             registration.syncMode,
		handlerFunction:      registration.handleFunc,
		dependency:           registration.dependency, // nil if there is no dependency
		isInProcess:          false,
		lastProcessedVersion: version.NewVersion(),
	}
}

func (e *deltaElement) Name() string {
	return e.eventType
}

func (e *deltaElement) Metadata() ConflationMetadata {
	return e.metadata
}

func (e *deltaElement) SyncMode() enum.EventSyncMode {
	return e.syncMode
}

func (e *deltaElement) Predicate(eventVersion *version.Version) bool {
	if eventVersion.InitGen() {
		e.lastProcessedVersion = version.NewVersion()
		log.Infow("resetting delta element version", "type", enum.ShortenEventType(e.eventType),
			"version", eventVersion)
	}
	if !eventVersion.NewerThan(e.lastProcessedVersion) {
		if e.metadata != nil {
			log.Debugw("drop delta bundle: get version %s, current hold metadata %s", eventVersion, e.metadata.Version())
		} else {
			log.Debugw("drop delta bundle: get version %s, current hold metadata nil", eventVersion)
		}
		return false
	}
	log.Debugw("inserting event", "version", eventVersion)
	return true
}

func (e *deltaElement) AddToReadyQueue(event *cloudevents.Event, metadata ConflationMetadata, cu *ConflationUnit) {
	cu.readyQueue.DeltaEventJobChan <- NewConflationJob(event, metadata, e.handlerFunction, cu, nil)
	e.metadata = metadata
}

// Success is to update the conflation element state after processing the event
func (e *deltaElement) PostProcess(metadata ConflationMetadata, err error) {
	if err != nil {
		log.Errorw("report error for the event", "type", e.eventType, "version", metadata.Version(), "error", err)
		return
	}

	// update state: lastProcessedVersion
	if metadata.Processed() && metadata.Version().NewerThan(e.lastProcessedVersion) {
		e.lastProcessedVersion = metadata.Version()
	}
}
