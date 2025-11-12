package conflator

import (
	"fmt"
	"sync"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"go.uber.org/zap"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/statistics"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/config"
)

// ConflationManager implements conflation units management.
type ConflationManager struct {
	log             *zap.SugaredLogger
	conflationUnits map[string]*ConflationUnit // map from leaf hub to conflation unit
	// requireInitialDependencyChecks bool
	registrations map[string]*ConflationRegistration
	readyQueue    *ConflationReadyQueue
	lock          sync.Mutex
	statistics    *statistics.Statistics
	Requster      transport.Requester
}

// NewConflationManager creates a new instance of ConflationManager.
func NewConflationManager(statistics *statistics.Statistics,
	requster transport.Requester,
) *ConflationManager {
	// conflationReadyQueue is shared between conflation manager and dispatcher
	conflationUnitsReadyQueue := NewConflationReadyQueue(statistics)

	return &ConflationManager{
		log:             logger.ZapLogger("conflation-manager"),
		conflationUnits: make(map[string]*ConflationUnit), // map from leaf hub to conflation unit
		// requireInitialDependencyChecks: requireInitialDependencyChecks,
		registrations: make(map[string]*ConflationRegistration),
		readyQueue:    conflationUnitsReadyQueue,
		lock:          sync.Mutex{}, // lock to be used to find/create conflation units
		statistics:    statistics,
		Requster:      requster,
	}
}

// Register registers bundle type with priority and handler function within the conflation manager.
func (cm *ConflationManager) Register(registration *ConflationRegistration) {
	cm.registrations[registration.eventType] = registration
	cm.log.Infow("registered event type", "type", enum.ShortenEventType(registration.eventType))
	cm.statistics.Register(registration.eventType)
}

// Insert function inserts the bundle to the appropriate conflation unit.
func (cm *ConflationManager) Insert(evt *cloudevents.Event) {
	// validate the event
	if _, ok := cm.registrations[evt.Type()]; !ok {
		cm.log.Infow("unregistered event type", "type", enum.ShortenEventType(evt.Type()))
		fmt.Print(evt)
		return
	}
	// metadata
	conflationMetadata := metadata.NewThresholdMetadata(config.GetKafkaOwnerIdentity(), 3, evt)
	if conflationMetadata == nil {
		return
	}

	cm.getConflationUnit(evt.Source()).insert(evt, conflationMetadata)
}

// GetTransportMetadatas provides collections of the CU's bundle transport-metadata.
func (cm *ConflationManager) GetMetadatas() []ConflationMetadata {
	metadata := make([]ConflationMetadata, 0)
	for _, cu := range cm.conflationUnits {
		metadata = append(metadata, cu.getMetadatas()...)
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
	conflationUnit := newConflationUnit(leafHubName, cm.readyQueue, cm.registrations, cm.statistics)
	cm.conflationUnits[leafHubName] = conflationUnit
	cm.statistics.IncrementNumberOfConflations()
	return conflationUnit
}

func (cm *ConflationManager) GetReadyQueue() *ConflationReadyQueue {
	return cm.readyQueue
}
