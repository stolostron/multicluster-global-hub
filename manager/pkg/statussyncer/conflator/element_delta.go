package conflator

import (
	"fmt"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/conflator/dependency"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

type deltaElement struct {
	log logr.Logger
	// state
	eventType            string
	syncMode             enum.EventSyncMode
	handlerFunction      EventHandleFunc
	dependency           *dependency.Dependency
	isInProcess          bool
	lastProcessedVersion *version.Version

	// payload
	job *ConflationJob
}

func NewDeltaElement(leafHubName string, registration *ConflationRegistration) *deltaElement {
	elementName := strings.Replace(registration.eventType, enum.EventTypePrefix, "", -1)
	return &deltaElement{
		log: ctrl.Log.WithName(fmt.Sprintf("%s.delta.%s", leafHubName, elementName)),

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
	return nil
}

func (e *deltaElement) SyncMode() enum.EventSyncMode {
	return e.syncMode
}

func (e *deltaElement) Predicate(eventVersion *version.Version) bool {
	if eventVersion.InitGen() {
		e.lastProcessedVersion = version.NewVersion()
		e.log.Info("resetting element processed version", "version", eventVersion)
	}
	e.log.V(2).Info("inserting event", "version", eventVersion)
	return eventVersion.NewerThan(e.lastProcessedVersion)
}

// Update is to update element payload
func (e *deltaElement) Update(event *cloudevents.Event, metadata ConflationMetadata) {
	e.job = NewConflationJob(event, metadata, e.handlerFunction, nil)
}

// the delta element will be delivered to job chan directly
func (e *deltaElement) IsReadyToProcess(cu *ConflationUnit) bool {
	return false
}

func (e *deltaElement) GetProcessJob(cu *ConflationUnit) *ConflationJob {
	e.job.Reporter = cu
	return e.job
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

	// update state: update the payload
	if metadata.Version().Equals(e.job.Metadata.Version()) {
		e.job = nil
	}
}
