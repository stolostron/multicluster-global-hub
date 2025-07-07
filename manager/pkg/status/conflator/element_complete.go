package conflator

import (
	"fmt"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator/dependency"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

var log = logger.DefaultZapLogger()

type completeElement struct {
	log logr.Logger
	// state
	eventType            string
	syncMode             enum.EventSyncMode
	handlerFunction      EventHandleFunc
	dependency           *dependency.Dependency
	isInProcess          bool
	lastProcessedVersion *version.Version
	// payload
	event    *cloudevents.Event
	metadata ConflationMetadata
}

func NewCompleteElement(leafHubName string, registration *ConflationRegistration) *completeElement {
	elementName := strings.Replace(registration.eventType, enum.EventTypePrefix, "", -1)
	return &completeElement{
		log: ctrl.Log.WithName(fmt.Sprintf("%s.complete.%s", leafHubName, elementName)),

		eventType:            registration.eventType,
		syncMode:             registration.syncMode,
		handlerFunction:      registration.handleFunc,
		dependency:           registration.dependency, // nil if there is no dependency
		isInProcess:          false,
		lastProcessedVersion: version.NewVersion(),
	}
}

func (e *completeElement) Name() string {
	return e.eventType
}

func (e *completeElement) Metadata() ConflationMetadata {
	return e.metadata
}

func (e *completeElement) SyncMode() enum.EventSyncMode {
	return e.syncMode
}

func (e *completeElement) Predicate(eventVersion *version.Version) bool {
	// when the agent is started, the first message version will be 0.1. then we need to
	// 1. reset lastProcessedBundleVersion to 0
	// 2. reset the bundleInfo version to 0 (add the resetBundleVersion() function to bundleInfo interface)
	if eventVersion.InitGen() {
		e.lastProcessedVersion = version.NewVersion()
		if e.metadata != nil {
			e.metadata.Version().Reset()
		}
		e.log.Info("resetting complete element processed version", "version", eventVersion)
	}

	// version validation
	// 1: the event.Version Vs lastProcessedVersion
	if !eventVersion.NewerThan(e.lastProcessedVersion) {
		log.Infof("drop bundle: get version %s, lastProcessedVersion %s", eventVersion, e.lastProcessedVersion)
		return false // we got old event, a newer (or equal) event was already processed.
	}

	// version validation 2: the insertBundle with the hold conflation bundle(memory)
	if e.metadata != nil && !eventVersion.NewerThan(e.metadata.Version()) {
		log.Infof("drop bundle: get version %s, current hold version%s", eventVersion, e.metadata.Version())
		return false // insert event only if version we got is newer than what we have in memory, otherwise do nothing.
	}

	return true
}

func (e *completeElement) AddToReadyQueue(event *cloudevents.Event, metadata ConflationMetadata, cu *ConflationUnit) {
	e.event = event
	e.metadata = metadata

	cu.addCUToReadyQueueIfNeeded()
}

func (e *completeElement) IsReadyToProcess(cu *ConflationUnit) bool {
	return e.event != nil && e.metadata != nil &&
		!e.isInProcess &&
		!e.metadata.Processed() &&
		!e.isCurrentOrAnyDependencyInProcess(cu) &&
		e.matchDependency(cu)
}

func (e *completeElement) ProcessJob(cu *ConflationUnit) *ConflationJob {
	if e.event == nil || e.metadata == nil {
		return nil
	}
	e.isInProcess = true
	return NewConflationJob(e.event, e.metadata, e.handlerFunction, cu, nil)
}

// Success is to update the conflation element state after processing the event
func (e *completeElement) PostProcess(metadata ConflationMetadata, err error) {
	// finished processing bundle
	e.isInProcess = false

	if err != nil {
		e.log.Error(err, "report error for the event", "type", e.eventType, "version", metadata.Version())
		return
	}

	// update state: lastProcessedVersion
	if metadata.Processed() && metadata.Version().NewerThan(e.lastProcessedVersion) {
		e.lastProcessedVersion = metadata.Version()
	}

	// update state: update the payload
	// if this is the same event that was processed then release bundle pointer, otherwise leave
	// the current (newer one) as pending.
	if metadata.Version().Equals(e.metadata.Version()) {
		e.event = nil
	}
}

// isCurrentOrAnyDependencyInProcess checks if current element or any dependency from dependency chain is in process.
func (e *completeElement) isCurrentOrAnyDependencyInProcess(cu *ConflationUnit) bool {
	if e.isInProcess { // current conflation element is in process
		return true
	}

	if e.dependency == nil { // no more dependencies in chain, therefore no dependency in process
		return false
	}

	dependencyIndex := cu.eventTypeToPriority[e.dependency.EventType]
	dependencyElement := cu.ElementPriorityQueue[dependencyIndex]

	completeDependency, ok := dependencyElement.(*completeElement)
	if !ok {
		e.log.Info("cannot handle the non completeElement type", "element", dependencyElement)
		return false
	}
	return completeDependency.isCurrentOrAnyDependencyInProcess(cu)
}

// dependencies are organized in a chain.
// if a bundle has a dependency, it will be processed only after its dependency was processed(using bundle value)
// else if a bundle has no dependency, it will be processed immediately.
func (e *completeElement) matchDependency(cu *ConflationUnit) bool {
	if e.dependency == nil {
		return true // bundle in this conflation element has no dependency
	}

	dependencyIndex := cu.eventTypeToPriority[e.dependency.EventType]
	dependencyElement := cu.ElementPriorityQueue[dependencyIndex]

	completeDependency, ok := dependencyElement.(*completeElement)
	if !ok {
		e.log.Info("cannot check the non completeElement dependency", "element", dependencyElement)
		return false
	}

	switch e.dependency.DependencyType {
	case dependency.ExactMatch:
		return e.metadata.DependencyVersion().EqualValue(completeDependency.lastProcessedVersion)

	case dependency.AtLeast:
		fallthrough // default case is AtLeast

	default:
		return !e.metadata.DependencyVersion().NewerValueThan(completeDependency.lastProcessedVersion)
	}
}
