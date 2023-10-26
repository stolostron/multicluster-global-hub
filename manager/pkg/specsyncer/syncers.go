package specsyncer

import (
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/config"
	specsyncer "github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/syncer"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/spec2db"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/spec2db/controller"
)

func AddGlobalResourceSpecSyncers(mgr ctrl.Manager, managerConfig *config.ManagerConfig) error {
	if err := spec2db.AddSpec2DBControllers(mgr); err != nil {
		return fmt.Errorf("failed to add spec-to-db controllers: %w", err)
	}

	if err := specsyncer.AddDB2TransportSyncers(mgr, managerConfig); err != nil {
		return fmt.Errorf("failed to add db-to-transport syncers: %w", err)
	}

	if err := specsyncer.AddManagedClusterLabelSyncer(mgr,
		managerConfig.SyncerConfig.DeletedLabelsTrimmingInterval); err != nil {
		return fmt.Errorf("failed to add status db watchers: %w", err)
	}
	return nil
}

func AddBasicSpecSyncers(mgr ctrl.Manager) error {
	return controller.AddManagedHubController(mgr)
}
