package conflator

import (
	cloudevents "github.com/cloudevents/sdk-go/v2"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/conflator/dependency"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

type conflationElement struct {
	conflationUnit       *ConflationUnit
	eventType            string
	syncMode             enum.EventSyncMode
	event                *cloudevents.Event
	metadata             ConflationMetadata
	handlerFunction      EventHandleFunc
	dependency           *dependency.Dependency
	isInProcess          bool
	lastProcessedVersion *version.Version
}

// update function that updates bundle and metadata and returns whether any error occurred.
func (element *conflationElement) update(event *cloudevents.Event, eventMetadata ConflationMetadata) {
	// delta event: policy event
	if element.syncMode == enum.DeltaStateMode {
		deltaEventJob := NewConflationJob(event, eventMetadata, element.handlerFunction, element.conflationUnit)
		element.conflationUnit.readyQueue.DeltaEventJobChan <- deltaEventJob
		return
	}

	element.event = event
	element.metadata = eventMetadata
}

// getBundleForProcessing function to return Bundle and BundleMetadata to forward to processors.
// At the end of this call, the bundle may be released (set to nil).
func (element *conflationElement) getEventForProcessing() (*cloudevents.Event, ConflationMetadata) {
	// getBundle must be called before getMetadata since getMetadata assumes that the bundle is being forwarded
	// to processors, therefore it may release the bundle (set to nil) and apply other dispatch-related functionality.
	// return element.conflationBundle.getBundle(), element.conflationBundle.getMetadata()
	return element.event, element.metadata
}
