package conflator

import (
	"errors"
	"sync"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/statistics"
)

const (
	invalidPriority = -1
)

var errNoReadyBundle = errors.New("no bundle is ready to be processed")

// ConflationUnit abstracts the conflation of prioritized multiple bundles with dependencies between them.
type ConflationUnit struct {
	log                  logr.Logger
	ElementPriorityQueue []ConflationElement
	eventTypeToPriority  map[string]ConflationPriority
	readyQueue           *ConflationReadyQueue
	// requireInitialDependencyChecks bool
	isInReadyQueue bool
	lock           sync.Mutex
	statistics     *statistics.Statistics
}

func newConflationUnit(name string, readyQueue *ConflationReadyQueue,
	registrations map[string]*ConflationRegistration, statistics *statistics.Statistics,
) *ConflationUnit {
	conflationUnit := &ConflationUnit{
		log:                  ctrl.Log.WithName(name),
		ElementPriorityQueue: make([]ConflationElement, len(registrations)),
		eventTypeToPriority:  make(map[string]ConflationPriority),
		readyQueue:           readyQueue,
		// requireInitialDependencyChecks: requireInitialDependencyChecks,
		isInReadyQueue: false,
		lock:           sync.Mutex{},
		statistics:     statistics,
	}

	for _, registration := range registrations {
		if registration.syncMode == enum.CompleteStateMode {
			conflationUnit.ElementPriorityQueue[registration.priority] = NewCompleteElement(name, registration)
		}
		if registration.syncMode == enum.DeltaStateMode {
			conflationUnit.ElementPriorityQueue[registration.priority] = NewDeltaElement(name, registration)
		}

		conflationUnit.eventTypeToPriority[registration.eventType] = registration.priority
	}

	return conflationUnit
}

// insert is an internal function, new bundles are inserted only via conflation manager.
func (cu *ConflationUnit) insert(event *cloudevents.Event, eventMetadata ConflationMetadata) {
	cu.lock.Lock()
	defer cu.lock.Unlock()

	priority := cu.eventTypeToPriority[event.Type()]
	conflationElement := cu.ElementPriorityQueue[priority]
	if conflationElement == nil {
		cu.log.Info("the conflationElement hasn't been registered to conflation unit", "eventType", event.Type())
		return
	}

	if !conflationElement.Predicate(eventMetadata.Version()) {
		return
	}

	// for the delta element, insert the ready queue directly and process one by one

	// start conflation unit metric for specific bundle type - overwrite it each time new bundle arrives
	// cu.statistics.StartConflationUnitMetrics(event)

	// if we got here, we got bundle with newer version
	// update the bundle in the priority queue.
	conflationElement.Update(event, eventMetadata)

	if conflationElement.SyncMode() == enum.CompleteStateMode {
		cu.addCUToReadyQueueIfNeeded()
	}
	if conflationElement.SyncMode() == enum.DeltaStateMode {
		cu.readyQueue.DeltaEventJobChan <- conflationElement.GetProcessJob(cu)
	}
}

// GetNext returns the next ready to be processed bundle and its transport metadata.
func (cu *ConflationUnit) GetNext() (*ConflationJob, error) {
	cu.lock.Lock()
	defer cu.lock.Unlock()

	nextElementPriority := cu.getNextReadyBundlePriority()
	if nextElementPriority == invalidPriority { // CU adds itself to RQ only when it has ready to process bundle
		return nil, errNoReadyBundle // therefore this shouldn't happen
	}

	conflationElement := cu.ElementPriorityQueue[nextElementPriority]

	cu.isInReadyQueue = false
	job := conflationElement.GetProcessJob(cu)

	// stop conflation unit metric for specific bundle type - evaluated once bundle is fetched from the priority queue
	// cu.statistics.StopConflationUnitMetrics(job.Event, nil)

	return job, nil
}

// ReportResult is used to report the result of bundle handling job.
func (cu *ConflationUnit) ReportResult(metadata ConflationMetadata, err error) {
	cu.lock.Lock()
	defer cu.lock.Unlock()

	priority := cu.eventTypeToPriority[metadata.EventType()] // priority of the bundle that was processed
	conflationElement := cu.ElementPriorityQueue[priority]

	conflationElement.PostProcess(metadata, err)

	if conflationElement.SyncMode() == enum.CompleteStateMode {
		cu.addCUToReadyQueueIfNeeded()
	}
}

func (cu *ConflationUnit) addCUToReadyQueueIfNeeded() {
	if cu.isInReadyQueue {
		return // allow CU to appear only once in RQ/processing
	}
	// if we reached here, CU is not in RQ, then get next element(isn't processing)
	nextReadyElementPriority := cu.getNextReadyBundlePriority()
	if nextReadyElementPriority != invalidPriority { // there is a ready to be processed bundle
		cu.readyQueue.ConflationUnitChan <- cu // let the dispatcher know this CU has a ready to be processed bundle
		cu.isInReadyQueue = true
	}
}

// returns next ready priority or invalidPriority (-1) in case no priority has a ready to be processed bundle.
func (cu *ConflationUnit) getNextReadyBundlePriority() int {
	// going over priority queue according to priorities.
	for priority, conflationElement := range cu.ElementPriorityQueue {
		if conflationElement.IsReadyToProcess(cu) {
			return priority
		}
	}
	return invalidPriority
}

// getBundleStatues provides collections of the CU's bundle transport-metadata.
func (cu *ConflationUnit) getMetadatas() []ConflationMetadata {
	cu.lock.Lock()
	defer cu.lock.Unlock()

	metadatas := make([]ConflationMetadata, 0, len(cu.ElementPriorityQueue))

	for _, element := range cu.ElementPriorityQueue {
		if element.Metadata() == nil {
			continue
		}
		metadatas = append(metadatas, element.Metadata())
	}

	return metadatas
}

// // function to determine whether the transport component requires initial-dependencies between bundles to be checked
// // (on load). If the returned is false, then we may assume that dependency of the initial bundle of
// // each type is met. Otherwise, there are no guarantees and the dependencies must be checked.
// func requireInitialDependencyChecks(transportType string) bool {
// 	switch transportType {
// 	case string(transport.Kafka):
// 		return false
// 		// once kafka consumer loads up, it starts reading from the earliest un-processed bundle,
// 		// as in all bundles that precede the latter have been processed, which include its dependency
// 		// bundle.

// 		// the order guarantee also guarantees that if while loading this component, a new bundle is written to a-
// 		// partition, then surely its dependency was written before it (leaf-hub-status-sync on kafka guarantees).
// 	default:
// 		return true
// 	}
// }
