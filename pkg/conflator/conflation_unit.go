package conflator

import (
	"errors"
	"sync"

	"github.com/go-logr/logr"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator/dependency"
	"github.com/stolostron/multicluster-global-hub/pkg/statistics"
)

const (
	invalidPriority = -1
)

var (
	errNoReadyBundle               = errors.New("no bundle is ready to be processed")
	errDependencyCannotBeEvaluated = errors.New("bundles declares dependency in registration but doesn't " +
		"implement DependantBundle interface")
)

// ResultReporter is an interface used to report the result of the handler function after its invocation.
// the idea is to have a clear separation of concerns and make sure dispatcher can only request for bundles and
// DB workers can only report results and not request for additional bundles.
// this makes sure DB workers get their input only via the dispatcher which is the entity responsible for reading
// bundles and invoking the handler functions using DB jobs.
// (using this interfaces verifies no developer violates the design that was intended).
type ResultReporter interface {
	ReportResult(m *ConflationBundleMetadata, err error)
}

// ConflationUnit abstracts the conflation of prioritized multiple bundles with dependencies between them.
type ConflationUnit struct {
	log                  logr.Logger
	priorityQueue        []*conflationElement
	bundleTypeToPriority map[string]ConflationPriority
	readyQueue           *ConflationReadyQueue
	// requireInitialDependencyChecks bool
	isInReadyQueue bool
	lock           sync.Mutex
	statistics     *statistics.Statistics
}

func newConflationUnit(log logr.Logger, readyQueue *ConflationReadyQueue,
	registrations []*ConflationRegistration, statistics *statistics.Statistics,
) *ConflationUnit {
	priorityQueue := make([]*conflationElement, len(registrations))
	bundleTypeToPriority := make(map[string]ConflationPriority)

	createBundleInfoFuncMap := map[metadata.BundleSyncMode]createBundleInfoFunc{
		metadata.DeltaStateMode:    newDeltaConflationBundle,
		metadata.CompleteStateMode: newCompleteConflationBundle,
	}

	for _, registration := range registrations {
		priorityQueue[registration.priority] = &conflationElement{
			conflationBundle:     createBundleInfoFuncMap[registration.syncMode](),
			handlerFunction:      registration.handlerFunction,
			dependency:           registration.dependency, // nil if there is no dependency
			isInProcess:          false,
			lastProcessedVersion: metadata.NewBundleVersion(),
		}

		bundleTypeToPriority[registration.bundleType] = registration.priority
	}

	return &ConflationUnit{
		log:                  log,
		priorityQueue:        priorityQueue,
		bundleTypeToPriority: bundleTypeToPriority,
		readyQueue:           readyQueue,
		// requireInitialDependencyChecks: requireInitialDependencyChecks,
		isInReadyQueue: false,
		lock:           sync.Mutex{},
		statistics:     statistics,
	}
}

// insert is an internal function, new bundles are inserted only via conflation manager.
func (cu *ConflationUnit) insert(insertBundle bundle.ManagerBundle, bundleStatus metadata.BundleStatus) {
	cu.lock.Lock()
	defer cu.lock.Unlock()

	bundleType := bundle.GetBundleType(insertBundle)
	priority := cu.bundleTypeToPriority[bundleType]
	conflationElement := cu.priorityQueue[priority]
	conflationElementBundle := conflationElement.conflationBundle.getBundle()

	// when the agent is started without incarnation configmap, the first message version will be 0.1. then we need to
	// 1. reset lastProcessedBundleVersion to 0
	// 2. reset the bundleInfo version to 0 (add the resetBundleVersion() function to bundleInfo interface)
	if insertBundle.GetVersion().InitGen() {
		conflationElement.lastProcessedVersion = metadata.NewBundleVersion()
		if conflationElementBundle != nil {
			conflationElementBundle.GetVersion().Reset()
			cu.log.Info("resetting version", "LH", insertBundle.GetLeafHubName(), "type", bundleType,
				"receivedVersion", insertBundle.GetVersion(),
				"beforeVersion", conflationElementBundle.GetVersion(),
				"lastProcessed", conflationElement.lastProcessedVersion,
			)
		}
	}

	cu.log.V(2).Info("inserting bundle", "managedHub", insertBundle.GetLeafHubName(), "bundleType", bundleType,
		"bundleVersion", insertBundle.GetVersion().String())

	// version validation 1: the insertBundle.Version Vs lastProcessedVersion
	if !insertBundle.GetVersion().NewerThan(conflationElement.lastProcessedVersion) {
		return // we got old bundle, a newer (or equal) bundle was already processed.
	}

	// version validation 2: the insertBundle with the hold conflation bundle(memory)
	if conflationElementBundle != nil && !insertBundle.GetVersion().NewerThan(conflationElementBundle.GetVersion()) {
		return // insert bundle only if version we got is newer than what we have in memory, otherwise do nothing.
	}

	// start conflation unit metric for specific bundle type - overwrite it each time new bundle arrives
	cu.statistics.StartConflationUnitMetrics(insertBundle)

	// if we got here, we got bundle with newer version
	// update the bundle in the priority queue.
	if err := conflationElement.update(insertBundle, bundleStatus); err != nil {
		cu.log.Error(err, "failed to insert bundle")
		return
	}

	// TODO: fix conflation mechanism:
	// - count correctly when a bundle is in processing but more than one bundle comes in
	// - conflating delta bundles is different from conflating complete-state bundles, needs to be addressed.
	cu.addCUToReadyQueueIfNeeded()
}

// GetNext returns the next ready to be processed bundle and its transport metadata.
func (cu *ConflationUnit) GetNext() (bundle.ManagerBundle, *ConflationBundleMetadata, BundleHandlerFunc, error) {
	cu.lock.Lock()
	defer cu.lock.Unlock()

	nextBundleToProcessPriority := cu.getNextReadyBundlePriority()
	if nextBundleToProcessPriority == invalidPriority { // CU adds itself to RQ only when it has ready to process bundle
		return nil, nil, nil, errNoReadyBundle // therefore this shouldn't happen
	}

	conflationElement := cu.priorityQueue[nextBundleToProcessPriority]

	cu.isInReadyQueue = false
	conflationElement.isInProcess = true

	// stop conflation unit metric for specific bundle type - evaluated once bundle is fetched from the priority queue
	cu.statistics.StopConflationUnitMetrics(conflationElement.conflationBundle.getBundle(), nil)

	bundleToProcess, bundleMetadata := conflationElement.getBundleForProcessing()

	return bundleToProcess, bundleMetadata, conflationElement.handlerFunction, nil
}

// ReportResult is used to report the result of bundle handling job.
func (cu *ConflationUnit) ReportResult(metadata *ConflationBundleMetadata, err error) {
	cu.lock.Lock()
	defer cu.lock.Unlock()

	priority := cu.bundleTypeToPriority[metadata.bundleType] // priority of the bundle that was processed
	conflationElement := cu.priorityQueue[priority]
	conflationElement.isInProcess = false // finished processing bundle

	defer func() {
		if conflationElement.conflationBundle.getMetadata().bundleStatus.Processed() &&
			metadata.bundleVersion.NewerThan(conflationElement.lastProcessedVersion) {
			conflationElement.lastProcessedVersion = metadata.bundleVersion
		}

		cu.addCUToReadyQueueIfNeeded()
	}()

	if err != nil {
		conflationElement.conflationBundle.getMetadata().bundleStatus.MarkAsUnprocessed()
		if deltaBundleInfo, ok := conflationElement.conflationBundle.(deltaBundleAdapter); ok {
			deltaBundleInfo.handleFailure(metadata)
		}
	} else {
		conflationElement.conflationBundle.markAsProcessed(metadata)
	}
}

func (cu *ConflationUnit) isInProcess() bool {
	for _, conflationElement := range cu.priorityQueue {
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
	nextReadyBundlePriority := cu.getNextReadyBundlePriority()
	if nextReadyBundlePriority != invalidPriority { // there is a ready to be processed bundle
		cu.readyQueue.Enqueue(cu) // let the dispatcher know this CU has a ready to be processed bundle
		cu.isInReadyQueue = true
	}
}

// returns next ready priority or invalidPriority (-1) in case no priority has a ready to be processed bundle.
func (cu *ConflationUnit) getNextReadyBundlePriority() int {
	for priority, conflationElement := range cu.priorityQueue { // going over priority queue according to priorities.
		if conflationElement.conflationBundle.getBundle() != nil &&
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
	m := conflationElement.conflationBundle.getMetadata()
	if m != nil && m.bundleStatus != nil && m.bundleStatus.Processed() {
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

	dependencyIndex := cu.bundleTypeToPriority[conflationElement.dependency.BundleType]

	return cu.isCurrentOrAnyDependencyInProcess(cu.priorityQueue[dependencyIndex])
}

// dependencies are organized in a chain.
// if a bundle has a dependency, it will be processed only after its dependency was processed(using bundle value)
// else if a bundle has no dependency, it will be processed immediately.
func (cu *ConflationUnit) checkDependency(conflationElement *conflationElement) bool {
	if conflationElement.dependency == nil {
		return true // bundle in this conflation element has no dependency
	}

	dependantBundle, ok := conflationElement.conflationBundle.getBundle().(bundle.ManagerDependantBundle)
	if !ok { // this bundle declared it has a dependency but doesn't implement DependantBundle
		cu.log.Error(errDependencyCannotBeEvaluated,
			"cannot evaluate bundle dependencies, not processing bundle",
			"LeafHubName", conflationElement.conflationBundle.getBundle().GetLeafHubName(),
			"bundleType", bundle.GetBundleType(conflationElement.conflationBundle.getBundle()))

		return false
	}

	dependencyIndex := cu.bundleTypeToPriority[conflationElement.dependency.BundleType]
	dependencyLastProcessedVersion := cu.priorityQueue[dependencyIndex].lastProcessedVersion

	// comment: always check the dependency
	// if !cu.requireInitialDependencyChecks &&
	// 	dependencyLastProcessedVersion.Equals(statusbundle.NewBundleVersion()) {
	// 	return true // transport does not require initial dependency check
	// }

	switch conflationElement.dependency.DependencyType {
	case dependency.ExactMatch:
		return dependantBundle.GetDependencyVersion().EqualValue(dependencyLastProcessedVersion)

	case dependency.AtLeast:
		fallthrough // default case is AtLeast

	default:
		// return !dependantBundle.GetDependencyVersion().NewerThan(dependencyLastProcessedVersion)
		return !dependantBundle.GetDependencyVersion().NewerValueThan(dependencyLastProcessedVersion)
	}
}

// getBundleStatues provides collections of the CU's bundle transport-metadata.
func (cu *ConflationUnit) getBundleStatues() []metadata.BundleStatus {
	cu.lock.Lock()
	defer cu.lock.Unlock()

	bundleStatues := make([]metadata.BundleStatus, 0, len(cu.priorityQueue))

	for _, element := range cu.priorityQueue {
		if bundleStatus := element.conflationBundle.getMetadata().bundleStatus; bundleStatus != nil {
			bundleStatues = append(bundleStatues, bundleStatus)
		}
	}

	return bundleStatues
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
