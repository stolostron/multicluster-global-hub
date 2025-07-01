package conflator

import (
	"fmt"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"go.uber.org/zap"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator/dependency"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

type deltaElement struct {
	log *zap.SugaredLogger
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
	elementName := strings.Replace(registration.eventType, enum.EventTypePrefix, "", -1)
	return &deltaElement{
		log:                  logger.ZapLogger(fmt.Sprintf("%s.delta.%s", leafHubName, elementName)),
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
		e.log.Infow("resetting element processed version", "version", eventVersion)
	}
	e.log.Debugw("inserting event", "version", eventVersion)
	return eventVersion.NewerThan(e.lastProcessedVersion)
}

func (e *deltaElement) AddToReadyQueue(event *cloudevents.Event, metadata ConflationMetadata, cu *ConflationUnit) {
	cu.readyQueue.DeltaEventJobChan <- NewConflationJob(event, metadata, e.handlerFunction, cu, nil)
	e.metadata = metadata
}

// Success is to update the conflation element state after processing the event
func (e *deltaElement) PostProcess(metadata ConflationMetadata, err error) {
	if err != nil {
		e.log.Error(err, "report error for the event", "type", e.eventType, "version", metadata.Version())
		return
	}

	// update state: lastProcessedVersion
	if metadata.Processed() && metadata.Version().NewerThan(e.lastProcessedVersion) {
		e.lastProcessedVersion = metadata.Version()
	}
}
