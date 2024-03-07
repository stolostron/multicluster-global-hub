package conflator

import (
	"container/list"
	"fmt"
	"sync"

	"github.com/stolostron/multicluster-global-hub/pkg/statistics"
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
	fmt.Println("=============== enqueue start")
	rq.lock.Lock()
	defer rq.lock.Unlock()
	fmt.Println("=============== enqueue start1")
	rq.queue.PushBack(cu)
	rq.notEmptyCondition.Signal() // Signal wakes another goroutine waiting on BlockingDequeue
	fmt.Println("=============== enqueue start2")
	rq.statistics.SetConflationReadyQueueSize(rq.queue.Len())
	fmt.Println("=============== enqueue end")
}

// BlockingDequeue pops ConflationUnit from the beginning of the queue. if no CU is ready, this call is blocking.
func (rq *ConflationReadyQueue) BlockingDequeue() *ConflationUnit {
	fmt.Println("=============== dequeue start")
	rq.lock.Lock()
	fmt.Println("=============== dequeue 1")

	defer rq.lock.Unlock()
	fmt.Println("=============== dequeue 2")
	for rq.isEmpty() { // if ready rq is empty - wait
		fmt.Println("=============== dequeue 3")
		rq.notEmptyCondition.Wait() // wait until ready rq notEmptyCondition is true
	}

	conflationUnit, ok := rq.queue.Remove(rq.queue.Front()).(*ConflationUnit) // conflation unit is inside element.Value
	rq.statistics.SetConflationReadyQueueSize(rq.queue.Len())

	if !ok {
		fmt.Println("=============== dequeue end ~")
		return nil
	}

	fmt.Println("=============== dequeue end")
	return conflationUnit
}

func (rq *ConflationReadyQueue) isEmpty() bool {
	return rq.queue.Len() == 0
}
