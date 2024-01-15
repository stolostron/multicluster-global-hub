package syncer

import (
	"fmt"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/config"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/db"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/db/gorm"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/syncer/dbsyncer"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

// AddDB2TransportSyncers adds the controllers that send info from DB to transport layer to the Manager.
func AddDB2TransportSyncers(mgr ctrl.Manager, managerConfig *config.ManagerConfig, producer transport.Producer) error {
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
	specDB := gorm.NewGormSpecDB()
	for _, addDBSyncerFunction := range addDBSyncerFunctions {
		if err := addDBSyncerFunction(mgr, specDB, producer, specSyncInterval); err != nil {
			return fmt.Errorf("failed to add DB Syncer: %w", err)
		}
	}
	return nil
}

// AddManagedClusterLabelSyncer update the label table by the managed cluster table
func AddManagedClusterLabelSyncer(mgr ctrl.Manager, deletedLabelsTrimmingInterval time.Duration) error {
	if err := dbsyncer.AddManagedClusterLabelsSyncer(mgr, deletedLabelsTrimmingInterval); err != nil {
		return fmt.Errorf("failed to add status watcher: %w", err)
	}
	return nil
}
