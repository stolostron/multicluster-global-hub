package conflator

import cloudevents "github.com/cloudevents/sdk-go/v2"

func NewConflationJob(event *cloudevents.Event, metadata ConflationMetadata, handle EventHandleFunc,
	reporter ResultReporter,
) *ConflationJob {
	return &ConflationJob{
		Event:    event,
		Metadata: metadata,
		Handle:   handle,
		Reporter: reporter,
	}
}

// ConflationJob represents the job to be run by a DBWorker from the pool.
type ConflationJob struct {
	Event    *cloudevents.Event
	Metadata ConflationMetadata
	Handle   EventHandleFunc
	Reporter ResultReporter
}
