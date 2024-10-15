package totransport

import (
	"fmt"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/spec/specdb"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/spec/specdb/gorm"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/spec/totransport/syncer"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

// AddDatabaseSyncers adds the controllers that send info from DB to transport layer to the Manager.
func AddSyncers(mgr ctrl.Manager, config *configs.ManagerConfig, producer transport.Producer) error {
	specSyncInterval := config.SyncerConfig.SpecSyncInterval
	specDB := gorm.NewGormSpecDB()

	addDBSyncerFunctions := []func(ctrl.Manager, specdb.SpecDB, transport.Producer, time.Duration) error{
		// dbsyncer.AddHoHConfigDBToTransportSyncer,
		syncer.AddPoliciesDBToTransportSyncer,
		syncer.AddPlacementRulesDBToTransportSyncer,
		syncer.AddPlacementBindingsDBToTransportSyncer,
		syncer.AddApplicationsDBToTransportSyncer,
		syncer.AddSubscriptionsDBToTransportSyncer,
		syncer.AddChannelsDBToTransportSyncer,
		syncer.AddManagedClusterLabelsDBToTransportSyncer,
		syncer.AddPlacementsDBToTransportSyncer,
		syncer.AddManagedClusterSetsDBToTransportSyncer,
		syncer.AddManagedClusterSetBindingsDBToTransportSyncer,
	}
	for _, addDBSyncerFunction := range addDBSyncerFunctions {
		if err := addDBSyncerFunction(mgr, specDB, producer, specSyncInterval); err != nil {
			return fmt.Errorf("failed to add DB Syncer: %w", err)
		}
	}
	return nil
}

// AddManagedClusterLabelSyncer update the label table by the managed cluster table
func AddManagedClusterLabelSyncer(mgr ctrl.Manager, deletedLabelsTrimmingInterval time.Duration) error {
	if err := syncer.AddManagedClusterLabelsSyncer(mgr, deletedLabelsTrimmingInterval); err != nil {
		return fmt.Errorf("failed to add status watcher: %w", err)
	}
	return nil
}
