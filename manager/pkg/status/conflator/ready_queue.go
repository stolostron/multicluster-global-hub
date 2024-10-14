package conflator

import (
	"github.com/stolostron/multicluster-global-hub/pkg/statistics"
)

// NewConflationReadyQueue creates a new instance of ConflationReadyQueue.
func NewConflationReadyQueue(statistics *statistics.Statistics) *ConflationReadyQueue {
	return &ConflationReadyQueue{
		statistics:         statistics,
		DeltaEventJobChan:  make(chan *ConflationJob, 1000),
		ConflationUnitChan: make(chan *ConflationUnit, 100),
	}
}

// ConflationReadyQueue is a queue of conflation units that have at least one bundle to process.
type ConflationReadyQueue struct {
	statistics *statistics.Statistics
	// create a Job chan for the detal event
	DeltaEventJobChan  chan *ConflationJob
	ConflationUnitChan chan *ConflationUnit
}
