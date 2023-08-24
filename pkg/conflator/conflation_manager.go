package conflator

import (
	"sync"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	statusbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
	"github.com/stolostron/multicluster-global-hub/pkg/statistics"
)

// NewConflationManager creates a new instance of ConflationManager.
func NewConflationManager(conflationUnitsReadyQueue *ConflationReadyQueue, statistics *statistics.Statistics,
) *ConflationManager {
	return &ConflationManager{
		log:             ctrl.Log.WithName("conflation-manager"),
		conflationUnits: make(map[string]*ConflationUnit), // map from leaf hub to conflation unit
		// requireInitialDependencyChecks: requireInitialDependencyChecks,
		registrations: make([]*ConflationRegistration, 0),
		readyQueue:    conflationUnitsReadyQueue,
		lock:          sync.Mutex{}, // lock to be used to find/create conflation units
		statistics:    statistics,
	}
}

// ConflationManager implements conflation units management.
type ConflationManager struct {
	log             logr.Logger
	conflationUnits map[string]*ConflationUnit // map from leaf hub to conflation unit
	// requireInitialDependencyChecks bool
	registrations []*ConflationRegistration
	readyQueue    *ConflationReadyQueue
	lock          sync.Mutex
	statistics    *statistics.Statistics
}

// Register registers bundle type with priority and handler function within the conflation manager.
func (cm *ConflationManager) Register(registration *ConflationRegistration) {
	cm.registrations = append(cm.registrations, registration)
}

// Insert function inserts the bundle to the appropriate conflation unit.
func (cm *ConflationManager) Insert(bundle statusbundle.Bundle, metadata bundle.BundleMetadata) {
	cm.getConflationUnit(bundle.GetLeafHubName()).insert(bundle, metadata)
}

// GetBundlesMetadata provides collections of the CU's bundle transport-metadata.
func (cm *ConflationManager) GetBundlesMetadata() []bundle.BundleMetadata {
	metadata := make([]bundle.BundleMetadata, 0)

	for _, cu := range cm.conflationUnits {
		metadata = append(metadata, cu.getBundlesMetadata()...)
	}

	return metadata
}

// if conflation unit doesn't exist for leaf hub, creates it.
func (cm *ConflationManager) getConflationUnit(leafHubName string) *ConflationUnit {
	cm.lock.Lock() // use lock to find/create conflation units
	defer cm.lock.Unlock()

	if conflationUnit, found := cm.conflationUnits[leafHubName]; found {
		return conflationUnit
	}
	// otherwise, need to create conflation unit
	conflationUnit := newConflationUnit(cm.log, cm.readyQueue, cm.registrations, cm.statistics)
	cm.conflationUnits[leafHubName] = conflationUnit
	cm.statistics.IncrementNumberOfConflations()

	return conflationUnit
}
