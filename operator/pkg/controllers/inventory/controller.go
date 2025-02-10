package inventory

import (
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

var (
	log                 = logger.DefaultZapLogger()
	inventoryReconciler *InventoryReconciler
)

func StartController(initOption config.ControllerOption) (config.ControllerInterface, error) {
	if inventoryReconciler != nil {
		return inventoryReconciler, nil
	}
	if !config.WithInventory(initOption.MulticlusterGlobalHub) {
		return nil, nil
	}
	if config.GetTransporterConn() == nil {
		return nil, nil
	}
	if config.GetStorageConnection() == nil {
		return nil, nil
	}
	log.Info("start inventory controller")

	inventoryReconciler = NewInventoryReconciler(initOption.Manager,
		initOption.KubeClient)
	err := inventoryReconciler.SetupWithManager(initOption.Manager)
	if err != nil {
		inventoryReconciler = nil
		return nil, err
	}
	log.Infof("inited inventory controller")
	return inventoryReconciler, nil
}
