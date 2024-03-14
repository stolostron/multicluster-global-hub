package conflator

import (
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
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
