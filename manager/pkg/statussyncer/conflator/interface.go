package conflator

import (
	cloudevents "github.com/cloudevents/sdk-go/v2"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

// may include metadata that relates to transport - e.g. commit offset.
type ConflationMetadata interface {
	// MarkAsProcessed function that marks the metadata as processed.
	MarkAsProcessed()
	// Processed returns whether the bundle was processed or not.
	Processed() bool
	// MarkAsUnprocessed function that marks the metadata as unprocessed.
	MarkAsUnprocessed()
	// the event version
	Version() *version.Version
	// the event dependencyVersion
	DependencyVersion() *version.Version
	// the event type
	EventType() string
	// the transport offset...
	TransportPosition() *transport.EventPosition
}

// ResultReporter is an interface used to report the result of the handler function after its invocation.
// the idea is to have a clear separation of concerns and make sure dispatcher can only request for bundles and
// DB workers can only report results and not request for additional bundles.
// this makes sure DB workers get their input only via the dispatcher which is the entity responsible for reading
// bundles and invoking the handler functions using DB jobs.
// (using this interfaces verifies no developer violates the design that was intended).
type ResultReporter interface {
	ReportResult(m ConflationMetadata, err error)
}

// Handler interface for registering business logic needed for handling bundles.
type Handler interface {
	// RegisterHandler registers event handler functions within the conflation manager.
	RegisterHandler(conflationManager *ConflationManager)
}

type ConflationElement interface {
	Name() string
	Metadata() ConflationMetadata
	SyncMode() enum.EventSyncMode

	// Predicate assert the received eventMetdata should be processed based on the current state
	Predicate(eventVersion *version.Version) bool

	// Update is to update element payload
	Update(event *cloudevents.Event, metadata ConflationMetadata)

	// IsReadyToProcess indacates the element is ready to be process by the worker, only for complete mode
	IsReadyToProcess(cu *ConflationUnit) bool

	// GetProcessJob will get a job from the element payload and state, once it's ready
	GetProcessJob(cu *ConflationUnit) *ConflationJob

	// PostProcess is to update the conflation element state after processing the event
	PostProcess(metadata ConflationMetadata, err error)
}
