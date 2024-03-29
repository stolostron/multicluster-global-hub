package conflator

import (
	"errors"
	"sync"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/go-logr/logr"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/conflator/dependency"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
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
	ElementPriorityQueue []*conflationElement
	eventTypeToPriority  map[string]ConflationPriority
	readyQueue           *ConflationReadyQueue
	// requireInitialDependencyChecks bool
	isInReadyQueue bool
	lock           sync.Mutex
	statistics     *statistics.Statistics
}

func newConflationUnit(log logr.Logger, readyQueue *ConflationReadyQueue,
	registrations map[string]*ConflationRegistration, statistics *statistics.Statistics,
) *ConflationUnit {
	conflationUnit := &ConflationUnit{
		log:                  log,
		ElementPriorityQueue: make([]*conflationElement, len(registrations)),
		eventTypeToPriority:  make(map[string]ConflationPriority),
		readyQueue:           readyQueue,
		// requireInitialDependencyChecks: requireInitialDependencyChecks,
		isInReadyQueue: false,
		lock:           sync.Mutex{},
		statistics:     statistics,
	}

	for _, registration := range registrations {
		conflationUnit.ElementPriorityQueue[registration.priority] = &conflationElement{
			conflationUnit:       conflationUnit,
			eventType:            registration.eventType,
			syncMode:             registration.syncMode,
			handlerFunction:      registration.handleFunc,
			dependency:           registration.dependency, // nil if there is no dependency
			isInProcess:          false,
			lastProcessedVersion: version.NewVersion(),
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

	// when the agent is started without incarnation configmap, the first message version will be 0.1. then we need to
	// 1. reset lastProcessedBundleVersion to 0
	// 2. reset the bundleInfo version to 0 (add the resetBundleVersion() function to bundleInfo interface)
	if eventMetadata.Version().InitGen() {
		conflationElement.lastProcessedVersion = version.NewVersion()
		if conflationElement.metadata != nil {
			conflationElement.metadata.Version().Reset()
		}
		cu.log.Info("resetting element processed version", "LH", event.Source(), "type", event.Type())
	}

	cu.log.V(2).Info("inserting event", "LH", event.Source(), "type", event.Type(), "version", eventMetadata.Version())

	// version validation 1: the event.Version Vs lastProcessedVersion
	if !eventMetadata.Version().NewerThan(conflationElement.lastProcessedVersion) {
		return // we got old event, a newer (or equal) event was already processed.
	}

	// version validation 2: the insertBundle with the hold conflation bundle(memory)
	if conflationElement.metadata != nil && !eventMetadata.Version().NewerThan(conflationElement.metadata.Version()) {
		return // insert event only if version we got is newer than what we have in memory, otherwise do nothing.
	}

	// start conflation unit metric for specific bundle type - overwrite it each time new bundle arrives
	cu.statistics.StartConflationUnitMetrics(event)

	// if we got here, we got bundle with newer version
	// update the bundle in the priority queue.
	conflationElement.update(event, eventMetadata)

	cu.addCUToReadyQueueIfNeeded()
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
	conflationElement.isInProcess = true

	// stop conflation unit metric for specific bundle type - evaluated once bundle is fetched from the priority queue
	cu.statistics.StopConflationUnitMetrics(conflationElement.event, nil)

	event, eventMetadata := conflationElement.getEventForProcessing()

	return NewConflationJob(event, eventMetadata, conflationElement.handlerFunction, cu), nil
}

// ReportResult is used to report the result of bundle handling job.
func (cu *ConflationUnit) ReportResult(metadata ConflationMetadata, err error) {
	cu.lock.Lock()
	defer cu.lock.Unlock()

	priority := cu.eventTypeToPriority[metadata.EventType()] // priority of the bundle that was processed
	conflationElement := cu.ElementPriorityQueue[priority]
	conflationElement.isInProcess = false // finished processing bundle

	defer func() {
		if metadata.Processed() && metadata.Version().NewerThan(conflationElement.lastProcessedVersion) {
			conflationElement.lastProcessedVersion = metadata.Version()
		}
		cu.addCUToReadyQueueIfNeeded()
	}()

	if err != nil {
		cu.log.Error(err, "report error for the event", "type", metadata.EventType(), "version", metadata.Version())
		return
	}

	if conflationElement.syncMode == enum.CompleteStateMode {
		// if this is the same event that was processed then release bundle pointer, otherwise leave
		// the current (newer one) as pending.
		if metadata.Version().Equals(conflationElement.metadata.Version()) {
			conflationElement.event = nil
		}
	}

	if conflationElement.syncMode == enum.DeltaStateMode {
		conflationElement.metadata = metadata // indicate the metada has been processed
	}
}

func (cu *ConflationUnit) isInProcess() bool {
	for _, conflationElement := range cu.ElementPriorityQueue {
		if conflationElement.isInProcess {
			return true // if any bundle is in process than conflation unit is in process
		}
	}
	return false
}

func (cu *ConflationUnit) addCUToReadyQueueIfNeeded() {
	if cu.isInReadyQueue || cu.isInProcess() {
		return // allow CU to appear only once in RQ/processing
	}
	// if we reached here, CU is not in RQ nor during processing
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
		if conflationElement.event != nil && conflationElement.metadata != nil &&
			!cu.isProcessed(conflationElement) &&
			!cu.isCurrentOrAnyDependencyInProcess(conflationElement) &&
			cu.checkDependency(conflationElement) {
			return priority // bundle in this priority is ready to be processed
		}
	}
	return invalidPriority
}

func (cu *ConflationUnit) isProcessed(conflationElement *conflationElement) bool {
	processed := false
	if conflationElement.metadata.Processed() {
		processed = true
	}
	return processed
}

// isCurrentOrAnyDependencyInProcess checks if current element or any dependency from dependency chain is in process.
func (cu *ConflationUnit) isCurrentOrAnyDependencyInProcess(conflationElement *conflationElement) bool {
	if conflationElement.isInProcess { // current conflation element is in process
		return true
	}

	if conflationElement.dependency == nil { // no more dependencies in chain, therefore no dependency in process
		return false
	}

	dependencyIndex := cu.eventTypeToPriority[conflationElement.dependency.EventType]

	return cu.isCurrentOrAnyDependencyInProcess(cu.ElementPriorityQueue[dependencyIndex])
}

// dependencies are organized in a chain.
// if a bundle has a dependency, it will be processed only after its dependency was processed(using bundle value)
// else if a bundle has no dependency, it will be processed immediately.
func (cu *ConflationUnit) checkDependency(conflationElement *conflationElement) bool {
	if conflationElement.dependency == nil {
		return true // bundle in this conflation element has no dependency
	}

	dependentIndex := cu.eventTypeToPriority[conflationElement.dependency.EventType]
	dependentElement := cu.ElementPriorityQueue[dependentIndex]

	switch conflationElement.dependency.DependencyType {
	case dependency.ExactMatch:
		return conflationElement.metadata.DependencyVersion().EqualValue(dependentElement.lastProcessedVersion)

	case dependency.AtLeast:
		fallthrough // default case is AtLeast

	default:
		return !conflationElement.metadata.DependencyVersion().NewerValueThan(dependentElement.lastProcessedVersion)
	}
}

// getBundleStatues provides collections of the CU's bundle transport-metadata.
func (cu *ConflationUnit) getMetadatas() []ConflationMetadata {
	cu.lock.Lock()
	defer cu.lock.Unlock()

	metadatas := make([]ConflationMetadata, 0, len(cu.ElementPriorityQueue))

	for _, element := range cu.ElementPriorityQueue {
		if element.metadata == nil {
			continue
		}
		metadatas = append(metadatas, element.metadata)
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
