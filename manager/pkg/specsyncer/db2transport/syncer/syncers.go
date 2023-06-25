package syncer

import (
	"fmt"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/config"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/db"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/syncer/dbsyncer"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/syncer/statuswatcher"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/producer"
)

// AddDB2TransportSyncers adds the controllers that send info from DB to transport layer to the Manager.
func AddDB2TransportSyncers(mgr ctrl.Manager, specDB db.SpecDB, managerConfig *config.ManagerConfig) error {
	producer, err := producer.NewGenericProducer(managerConfig.TransportConfig)
	if err != nil {
		return fmt.Errorf("failed to init spec transport bridge: %w", err)
	}
	specSyncInterval := managerConfig.SyncerConfig.SpecSyncInterval

	addDBSyncerFunctions := []func(ctrl.Manager, db.SpecDB, transport.Producer, time.Duration) error{
		// dbsyncer.AddHoHConfigDBToTransportSyncer,
		dbsyncer.AddPoliciesDBToTransportSyncer,
		dbsyncer.AddPlacementRulesDBToTransportSyncer,
		dbsyncer.AddPlacementBindingsDBToTransportSyncer,
		dbsyncer.AddApplicationsDBToTransportSyncer,
		dbsyncer.AddSubscriptionsDBToTransportSyncer,
		dbsyncer.AddChannelsDBToTransportSyncer,
		dbsyncer.AddManagedClusterLabelsDBToTransportSyncer,
		dbsyncer.AddPlacementsDBToTransportSyncer,
		dbsyncer.AddManagedClusterSetsDBToTransportSyncer,
		dbsyncer.AddManagedClusterSetBindingsDBToTransportSyncer,
	}
	for _, addDBSyncerFunction := range addDBSyncerFunctions {
		if err := addDBSyncerFunction(mgr, specDB, producer, specSyncInterval); err != nil {
			return fmt.Errorf("failed to add DB Syncer: %w", err)
		}
	}

	return nil
}

// AddStatusDBWatchers adds the controllers that watch the status DB to update the spec DB to the Manager.
func AddStatusDBWatchers(mgr ctrl.Manager, db db.DB, deletedLabelsTrimmingInterval time.Duration) error {
	if err := statuswatcher.AddManagedClusterLabelsStatusWatcher(mgr, db, db,
		deletedLabelsTrimmingInterval); err != nil {
		return fmt.Errorf("failed to add status watcher: %w", err)
	}

	return nil
}
