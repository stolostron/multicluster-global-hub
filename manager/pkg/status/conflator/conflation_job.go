package conflator

import cloudevents "github.com/cloudevents/sdk-go/v2"

func NewConflationJob(event *cloudevents.Event, metadata ConflationMetadata, handle EventHandleFunc,
	reporter ResultReporter, state *ElementState,
) *ConflationJob {
	return &ConflationJob{
		Event:        event,
		Metadata:     metadata,
		Handle:       handle,
		Reporter:     reporter,
		ElementState: state,
	}
}

// ConflationJob represents the job to be run by a DBWorker from the pool.
type ConflationJob struct {
	Event  *cloudevents.Event
	Handle EventHandleFunc

	// deprecated
	Metadata ConflationMetadata
	Reporter ResultReporter

	ElementState *ElementState
}
