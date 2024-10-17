package spec

import (
	"fmt"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/spec/controllers"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/spec/specdb"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/spec/specdb/gorm"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/spec/syncers"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var specCtrlStarted = false

func AddToManager(mgr ctrl.Manager, config *configs.ManagerConfig, producer transport.Producer) error {
	if specCtrlStarted {
		return nil
	}

	// spec to db
	if err := AddDBControllers(mgr); err != nil {
		return fmt.Errorf("failed to add spec-to-db controllers: %w", err)
	}

	// db to transport
	if err := AddDatabaseSyncers(mgr, config, producer); err != nil {
		return fmt.Errorf("failed to add db-to-transport syncers: %w", err)
	}
	if err := AddManagedClusterLabelDBSyncer(mgr, config.SyncerConfig.DeletedLabelsTrimmingInterval); err != nil {
		return fmt.Errorf("failed to add status db watchers: %w", err)
	}

	specCtrlStarted = true
	return nil
}

// AddControllersToDatabase adds all the spec-to-db controllers to the Manager.
func AddDBControllers(mgr ctrl.Manager) error {
	addControllerFunctions := []func(ctrl.Manager, specdb.SpecDB) error{
		controllers.AddPolicyController,
		controllers.AddPlacementRuleController,
		controllers.AddPlacementBindingController,
		controllers.AddApplicationController,
		controllers.AddSubscriptionController,
		controllers.AddChannelController,
		controllers.AddManagedClusterSetController,
		controllers.AddManagedClusterSetBindingController,
		controllers.AddPlacementController,
	}
	specDB := gorm.NewGormSpecDB()
	for _, addControllerFunction := range addControllerFunctions {
		if err := addControllerFunction(mgr, specDB); err != nil {
			return fmt.Errorf("failed to add controller: %w", err)
		}
	}

	return nil
}

// AddDatabaseSyncers adds the controllers that send info from DB to transport layer to the Manager.
func AddDatabaseSyncers(mgr ctrl.Manager, config *configs.ManagerConfig, producer transport.Producer) error {
	specSyncInterval := config.SyncerConfig.SpecSyncInterval
	specDB := gorm.NewGormSpecDB()

	addDBSyncerFunctions := []func(ctrl.Manager, specdb.SpecDB, transport.Producer, time.Duration) error{
		// dbsyncer.AddHoHConfigDBToTransportSyncer,
		syncers.AddPoliciesDBToTransportSyncer,
		syncers.AddPlacementRulesDBToTransportSyncer,
		syncers.AddPlacementBindingsDBToTransportSyncer,
		syncers.AddApplicationsDBToTransportSyncer,
		syncers.AddSubscriptionsDBToTransportSyncer,
		syncers.AddChannelsDBToTransportSyncer,
		syncers.AddManagedClusterLabelsDBToTransportSyncer,
		syncers.AddPlacementsDBToTransportSyncer,
		syncers.AddManagedClusterSetsDBToTransportSyncer,
		syncers.AddManagedClusterSetBindingsDBToTransportSyncer,
	}
	for _, addDBSyncerFunction := range addDBSyncerFunctions {
		if err := addDBSyncerFunction(mgr, specDB, producer, specSyncInterval); err != nil {
			return fmt.Errorf("failed to add DB Syncer: %w", err)
		}
	}
	return nil
}

// AddManagedClusterLabelDBSyncer update the label table by the managed cluster table
func AddManagedClusterLabelDBSyncer(mgr ctrl.Manager, deletedLabelsTrimmingInterval time.Duration) error {
	if err := syncers.AddManagedClusterLabelsSyncer(mgr, deletedLabelsTrimmingInterval); err != nil {
		return fmt.Errorf("failed to add status watcher: %w", err)
	}
	return nil
}
