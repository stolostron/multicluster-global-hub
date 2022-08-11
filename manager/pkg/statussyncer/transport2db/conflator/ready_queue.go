package conflator

import (
	"container/list"
	"sync"

	"github.com/stolostron/multicluster-globalhub/manager/pkg/statistics"
)

// NewConflationReadyQueue creates a new instance of ConflationReadyQueue.
func NewConflationReadyQueue(statistics *statistics.Statistics) *ConflationReadyQueue {
	lock := &sync.Mutex{}

	return &ConflationReadyQueue{
		queue:             list.New(),
		lock:              lock,
		notEmptyCondition: sync.NewCond(lock),
		statistics:        statistics,
	}
}

// ConflationReadyQueue is a queue of conflation units that have at least one bundle to process.
type ConflationReadyQueue struct {
	queue             *list.List
	lock              *sync.Mutex
	notEmptyCondition *sync.Cond
	statistics        *statistics.Statistics
}

// Enqueue inserts ConflationUnit to the end of the ready queue.
func (rq *ConflationReadyQueue) Enqueue(cu *ConflationUnit) {
	rq.lock.Lock()
	defer rq.lock.Unlock()

	rq.queue.PushBack(cu)
	rq.notEmptyCondition.Signal() // Signal wakes another goroutine waiting on BlockingDequeue

	rq.statistics.SetConflationReadyQueueSize(rq.queue.Len())
}

// BlockingDequeue pops ConflationUnit from the beginning of the queue. if no CU is ready, this call is blocking.
func (rq *ConflationReadyQueue) BlockingDequeue() *ConflationUnit {
	rq.lock.Lock()
	defer rq.lock.Unlock()

	for rq.isEmpty() { // if ready rq is empty - wait
		rq.notEmptyCondition.Wait() // wait until ready rq notEmptyCondition is true
	}

	conflationUnit, ok := rq.queue.Remove(rq.queue.Front()).(*ConflationUnit) // conflation unit is inside element.Value
	rq.statistics.SetConflationReadyQueueSize(rq.queue.Len())

	if !ok {
		return nil
	}

	return conflationUnit
}

func (rq *ConflationReadyQueue) isEmpty() bool {
	return rq.queue.Len() == 0
}
