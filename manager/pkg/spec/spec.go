package spec

import (
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/spec/controller"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/spec/db2transport/db"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/spec/db2transport/db/gorm"
	specsyncer "github.com/stolostron/multicluster-global-hub/manager/pkg/spec/db2transport/syncer"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var specCtrlStarted = false

func AddToManager(mgr ctrl.Manager, managerConfig *configs.ManagerConfig, producer transport.Producer) error {
	if specCtrlStarted {
		return nil
	}

	if err := AddSpecToDBControllers(mgr); err != nil {
		return fmt.Errorf("failed to add spec-to-db controllers: %w", err)
	}

	if err := specsyncer.AddDB2TransportSyncers(mgr, managerConfig, producer); err != nil {
		return fmt.Errorf("failed to add db-to-transport syncers: %w", err)
	}

	if err := specsyncer.AddManagedClusterLabelSyncer(mgr,
		managerConfig.SyncerConfig.DeletedLabelsTrimmingInterval); err != nil {
		return fmt.Errorf("failed to add status db watchers: %w", err)
	}

	specCtrlStarted = true
	return nil
}

// AddSpecToDBControllers adds all the spec-to-db controllers to the Manager.
func AddSpecToDBControllers(mgr ctrl.Manager) error {
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
