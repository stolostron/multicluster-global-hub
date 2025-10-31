package conflator

import (
	"errors"
	"sync"

	cloudevents "github.com/cloudevents/sdk-go/v2"

	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/statistics"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// ConflationUnit abstracts the conflation of prioritized multiple bundles with dependencies between them.
type ConflationUnit struct {
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
		ElementPriorityQueue: make([]ConflationElement, len(registrations)),
		eventTypeToPriority:  make(map[string]ConflationPriority),
		readyQueue:           readyQueue,
		// requireInitialDependencyChecks: requireInitialDependencyChecks,
		isInReadyQueue: false,
		lock:           sync.Mutex{},
		statistics:     statistics,
	}
	log.Infof("registering %d elements into conflation unit", "count", len(registrations))
	for _, registration := range registrations {
		if registration.syncMode == enum.CompleteStateMode {
			log.Infow("registering complete element", "type", enum.ShortenEventType(registration.eventType))
			conflationUnit.ElementPriorityQueue[registration.priority] = NewCompleteElement(name, registration)
		}
		if registration.syncMode == enum.DeltaStateMode {
			log.Infow("registering delta element", "type", enum.ShortenEventType(registration.eventType))
			conflationUnit.ElementPriorityQueue[registration.priority] = NewDeltaElement(name, registration)
		}

		if registration.syncMode == enum.HybridStateMode {
			log.Infow("registering hybrid element", "type", enum.ShortenEventType(registration.eventType))
			conflationUnit.ElementPriorityQueue[registration.priority] = NewHybridElement(registration)
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
		log.Debugw("the conflationElement hasn't been registered to conflation unit", "eventType", event.Type())
		return
	}

	if !conflationElement.Predicate(eventMetadata.Version()) {
		log.Infow("the conflationElement predication is false")
		utils.PrettyPrint(event)
		return
	}

	// for the delta element, insert the ready queue directly and process one by one

	// start conflation unit metric for specific bundle type - overwrite it each time new bundle arrives
	// cu.statistics.StartConflationUnitMetrics(event)

	// if we got here, we got bundle with newer version
	// update the bundle in the priority queue.
	conflationElement.AddToReadyQueue(event, eventMetadata, cu)
}

// GetNext returns the next ready to be processed bundle and its transport metadata.
func (cu *ConflationUnit) GetNext() (*ConflationJob, error) {
	cu.lock.Lock()
	defer cu.lock.Unlock()

	element := cu.getNextReadyCompleteElement()
	if element == nil { // CU adds itself to RQ only when it has ready to process bundle
		return nil, errors.New("no event(element) is ready to be processed")
	}

	cu.isInReadyQueue = false

	job := element.ProcessJob(cu)
	if job == nil {
		return nil, errors.New("no job is ready to be processed")
	}
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
	element := cu.getNextReadyCompleteElement()
	if element != nil { // there is a ready to be processed bundle
		cu.readyQueue.ConflationUnitChan <- cu // let the dispatcher know this CU has a ready to be processed bundle
		cu.isInReadyQueue = true
	}
}

// returns next ready priority or invalidPriority (-1) in case no priority has a ready to be processed bundle.
func (cu *ConflationUnit) getNextReadyCompleteElement() *completeElement {
	// going over priority queue according to priorities.
	for _, conflationElement := range cu.ElementPriorityQueue {
		// skip the delta element
		if conflationElement.SyncMode() == enum.DeltaStateMode || conflationElement.SyncMode() == enum.HybridStateMode {
			continue
		}
		complete, ok := conflationElement.(*completeElement)
		if !ok {
			log.Info("cannot process the element", "type", conflationElement.Metadata())
			continue
		}
		if complete.IsReadyToProcess(cu) {
			return complete
		}
	}
	return nil
}

// getMetadatas provides metadata collections of the element's transport-metadata from the CU.
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
