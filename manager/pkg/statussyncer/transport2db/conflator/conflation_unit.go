package conflator

import (
	"errors"
	"sync"

	"github.com/go-logr/logr"

	"github.com/stolostron/multicluster-globalhub/manager/pkg/statistics"
	"github.com/stolostron/multicluster-globalhub/manager/pkg/statussyncer/transport2db/bundle"
	"github.com/stolostron/multicluster-globalhub/manager/pkg/statussyncer/transport2db/conflator/dependency"
	"github.com/stolostron/multicluster-globalhub/manager/pkg/statussyncer/transport2db/helpers"
	"github.com/stolostron/multicluster-globalhub/manager/pkg/statussyncer/transport2db/transport"
	"github.com/stolostron/multicluster-globalhub/pkg/bundle/status"
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
	ReportResult(metadata *BundleMetadata, err error)
}

func newConflationUnit(log logr.Logger, readyQueue *ConflationReadyQueue,
	registrations []*ConflationRegistration, requireInitialDependencyChecks bool,
	statistics *statistics.Statistics,
) *ConflationUnit {
	priorityQueue := make([]*conflationElement, len(registrations))
	bundleTypeToPriority := make(map[string]ConflationPriority)

	createBundleInfoFuncMap := map[status.BundleSyncMode]createBundleInfoFunc{
		status.DeltaStateMode:    newDeltaStateBundleInfo,
		status.CompleteStateMode: newCompleteStateBundleInfo,
	}

	for _, registration := range registrations {
		priorityQueue[registration.priority] = &conflationElement{
			bundleInfo:                 createBundleInfoFuncMap[registration.syncMode](),
			handlerFunction:            registration.handlerFunction,
			dependency:                 registration.dependency, // nil if there is no dependency
			isInProcess:                false,
			lastProcessedBundleVersion: noBundleVersion(),
		}

		bundleTypeToPriority[registration.bundleType] = registration.priority
	}

	return &ConflationUnit{
		log:                            log,
		priorityQueue:                  priorityQueue,
		bundleTypeToPriority:           bundleTypeToPriority,
		readyQueue:                     readyQueue,
		requireInitialDependencyChecks: requireInitialDependencyChecks,
		isInReadyQueue:                 false,
		lock:                           sync.Mutex{},
		statistics:                     statistics,
	}
}

// ConflationUnit abstracts the conflation of prioritized multiple bundles with dependencies between them.
type ConflationUnit struct {
	log                            logr.Logger
	priorityQueue                  []*conflationElement
	bundleTypeToPriority           map[string]ConflationPriority
	readyQueue                     *ConflationReadyQueue
	requireInitialDependencyChecks bool
	isInReadyQueue                 bool
	lock                           sync.Mutex
	statistics                     *statistics.Statistics
}

// insert is an internal function, new bundles are inserted only via conflation manager.
func (cu *ConflationUnit) insert(bundle bundle.Bundle, metadata transport.BundleMetadata) {
	cu.lock.Lock()
	defer cu.lock.Unlock()

	bundleType := helpers.GetBundleType(bundle)
	priority := cu.bundleTypeToPriority[bundleType]
	conflationElement := cu.priorityQueue[priority]
	conflationElementBundle := conflationElement.bundleInfo.getBundle()

	if !bundle.GetVersion().NewerThan(conflationElement.lastProcessedBundleVersion) {
		return // we got old bundle, a newer (or equal) bundle was already processed.
	}

	if conflationElementBundle != nil && !bundle.GetVersion().NewerThan(conflationElementBundle.GetVersion()) {
		return // insert bundle only if version we got is newer than what we have in memory, otherwise do nothing.
	}

	// start conflation unit metric for specific bundle type - overwrite it each time new bundle arrives
	cu.statistics.StartConflationUnitMetrics(bundle)

	// if we got here, we got bundle with newer version
	// update the bundle in the priority queue.
	if err := conflationElement.update(bundle, metadata); err != nil {
		cu.log.Error(err, "failed to insert bundle")
		return
	}
	// TODO: fix conflation mechanism:
	// - count correctly when a bundle is in processing but more than one bundle comes in
	// - conflating delta bundles is different from conflating complete-state bundles, needs to be addressed.
	cu.addCUToReadyQueueIfNeeded()
}

// GetNext returns the next ready to be processed bundle and its transport metadata.
func (cu *ConflationUnit) GetNext() (bundle.Bundle, *BundleMetadata, BundleHandlerFunc, error) {
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
	cu.statistics.StopConflationUnitMetrics(conflationElement.bundleInfo.getBundle())

	bundleToProcess, bundleMetadata := conflationElement.getBundleForProcessing()

	return bundleToProcess, bundleMetadata, conflationElement.handlerFunction, nil
}

// ReportResult is used to report the result of bundle handling job.
func (cu *ConflationUnit) ReportResult(metadata *BundleMetadata, err error) {
	cu.lock.Lock()
	defer cu.lock.Unlock()

	priority := cu.bundleTypeToPriority[metadata.bundleType] // priority of the bundle that was processed
	conflationElement := cu.priorityQueue[priority]
	conflationElement.isInProcess = false // finished processing bundle

	if err != nil {
		if deltaBundleInfo, ok := conflationElement.bundleInfo.(deltaBundleInfo); ok {
			deltaBundleInfo.handleFailure(metadata)
		}

		cu.addCUToReadyQueueIfNeeded()

		return
	}
	// otherwise, err is nil, means bundle processing finished successfully
	if metadata.bundleVersion.NewerThan(conflationElement.lastProcessedBundleVersion) {
		conflationElement.lastProcessedBundleVersion = metadata.bundleVersion
	}

	conflationElement.bundleInfo.markAsProcessed(metadata)
	cu.addCUToReadyQueueIfNeeded()
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
		if conflationElement.bundleInfo.getBundle() != nil &&
			!cu.isCurrentOrAnyDependencyInProcess(conflationElement) && cu.checkDependency(conflationElement) {
			return priority // bundle in this priority is ready to be processed
		}
	}

	return invalidPriority
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
func (cu *ConflationUnit) checkDependency(conflationElement *conflationElement) bool {
	if conflationElement.dependency == nil {
		return true // bundle in this conflation element has no dependency
	}

	dependantBundle, ok := conflationElement.bundleInfo.getBundle().(bundle.DependantBundle)
	if !ok { // this bundle declared it has a dependency but doesn't implement DependantBundle
		cu.log.Error(errDependencyCannotBeEvaluated,
			"cannot evaluate bundle dependencies, not processing bundle",
			"LeafHubName", conflationElement.bundleInfo.getBundle().GetLeafHubName(),
			"bundleType", helpers.GetBundleType(conflationElement.bundleInfo.getBundle()))

		return false
	}

	dependencyIndex := cu.bundleTypeToPriority[conflationElement.dependency.BundleType]
	dependencyLastProcessedVersion := cu.priorityQueue[dependencyIndex].lastProcessedBundleVersion

	if !cu.requireInitialDependencyChecks &&
		dependencyLastProcessedVersion.Equals(noBundleVersion()) {
		return true // transport does not require initial dependency check
	}

	switch conflationElement.dependency.DependencyType {
	case dependency.ExactMatch:
		return dependantBundle.GetDependencyVersion().Equals(dependencyLastProcessedVersion)

	case dependency.AtLeast:
		fallthrough // default case is AtLeast

	default:
		return !dependantBundle.GetDependencyVersion().NewerThan(dependencyLastProcessedVersion)
	}
}

func noBundleVersion() *status.BundleVersion {
	return status.NewBundleVersion(0, 0)
}

// getBundlesMetadata provides collections of the CU's bundle transport-metadata.
func (cu *ConflationUnit) getBundlesMetadata() []transport.BundleMetadata {
	cu.lock.Lock()
	defer cu.lock.Unlock()

	bundlesMetadata := make([]transport.BundleMetadata, 0, len(cu.priorityQueue))

	for _, element := range cu.priorityQueue {
		if transportMetadata := element.bundleInfo.getTransportMetadataToCommit(); transportMetadata != nil {
			bundlesMetadata = append(bundlesMetadata, transportMetadata)
		}
	}

	return bundlesMetadata
}
