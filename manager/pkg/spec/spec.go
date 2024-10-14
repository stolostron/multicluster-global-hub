package spec

import (
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/spec/controller"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/spec/specdb/gorm"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var specCtrlStarted = false

func AddToManager(mgr ctrl.Manager, config *configs.ManagerConfig, producer transport.Producer) error {
	if specCtrlStarted {
		return nil
	}

	// spec to db
	if err := AddControllers(mgr); err != nil {
		return fmt.Errorf("failed to add spec-to-db controllers: %w", err)
	}

	// db to transport
	if err := transport.AddDBSyncers(mgr, config, producer); err != nil {
		return fmt.Errorf("failed to add db-to-transport syncers: %w", err)
	}

	if err := transport.AddManagedClusterLabelDBSyncer(mgr,
		config.SyncerConfig.DeletedLabelsTrimmingInterval); err != nil {
		return fmt.Errorf("failed to add status db watchers: %w", err)
	}

	specCtrlStarted = true
	return nil
}

// AddControllersToDatabase adds all the spec-to-db controllers to the Manager.
func AddControllers(mgr ctrl.Manager) error {
	addControllerFunctions := []func(ctrl.Manager, db.SpecDB) error{
		controller.AddPolicyController,
		controller.AddPlacementRuleController,
		controller.AddPlacementBindingController,
		controller.AddApplicationController,
		controller.AddSubscriptionController,
		controller.AddChannelController,
		controller.AddManagedClusterSetController,
		controller.AddManagedClusterSetBindingController,
		controller.AddPlacementController,
	}
	specDB := gorm.NewGormSpecDB()
	for _, addControllerFunction := range addControllerFunctions {
		if err := addControllerFunction(mgr, specDB); err != nil {
			return fmt.Errorf("failed to add controller: %w", err)
		}
	}

	return nil
}
