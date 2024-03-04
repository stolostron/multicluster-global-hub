package conflator

import (
	"fmt"

	cloudevents "github.com/cloudevents/sdk-go/v2"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
)

type conflationElement struct {
	eventType            string
	syncMode             metadata.EventSyncMode
	event                *cloudevents.Event
	metadata             ConflationMetadata
	handlerFunction      EventHandleFunc
	dependency           *Dependency
	isInProcess          bool
	lastProcessedVersion *metadata.BundleVersion
}

// update function that updates bundle and metadata and returns whether any error occurred.
func (element *conflationElement) update(event *cloudevents.Event, eventMetadata ConflationMetadata) error {
	if element.syncMode == metadata.DeltaStateMode {
		return fmt.Errorf("unsupported to handle DeltaStateMode for event: %s", event.Type())
	}

	element.event = event
	element.metadata = eventMetadata
	return nil
}

// getBundleForProcessing function to return Bundle and BundleMetadata to forward to processors.
// At the end of this call, the bundle may be released (set to nil).
func (element *conflationElement) getEventForProcessing() (*cloudevents.Event, ConflationMetadata) {
	// getBundle must be called before getMetadata since getMetadata assumes that the bundle is being forwarded
	// to processors, therefore it may release the bundle (set to nil) and apply other dispatch-related functionality.
	// return element.conflationBundle.getBundle(), element.conflationBundle.getMetadata()
	return element.event, element.metadata
}
