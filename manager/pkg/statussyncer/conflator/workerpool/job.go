package workerpool

import (
	cloudevents "github.com/cloudevents/sdk-go/v2"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/conflator"
)

// NewDBJob creates a new instance of DBJob.
func NewDBJob(event *cloudevents.Event, eventMetadata conflator.ConflationMetadata,
	handleFunc conflator.EventHandleFunc,
	conflationUnitResultReporter conflator.ResultReporter,
) *DBJob {
	return &DBJob{
		event:                        event,
		eventMetadata:                eventMetadata,
		handleFunc:                   handleFunc,
		conflationUnitResultReporter: conflationUnitResultReporter,
	}
}

// DBJob represents the job to be run by a DBWorker from the pool.
type DBJob struct {
	event                        *cloudevents.Event
	eventMetadata                conflator.ConflationMetadata
	handleFunc                   conflator.EventHandleFunc
	conflationUnitResultReporter conflator.ResultReporter
}
