package specsyncer

import (
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/config"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/db/postgresql"
	specsyncer "github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/syncer"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/spec2db"
)

func AddSpecSyncers(mgr ctrl.Manager, managerConfig *config.ManagerConfig,
	processPostgreSQL *postgresql.PostgreSQL,
) error {
	if err := spec2db.AddSpec2DBControllers(mgr, processPostgreSQL); err != nil {
		return fmt.Errorf("failed to add spec-to-db controllers: %w", err)
	}

	if err := specsyncer.AddDB2TransportSyncers(mgr, processPostgreSQL, managerConfig); err != nil {
		return fmt.Errorf("failed to add db-to-transport syncers: %w", err)
	}

	if err := specsyncer.AddStatusDBWatchers(mgr, processPostgreSQL,
		managerConfig.SyncerConfig.DeletedLabelsTrimmingInterval); err != nil {
		return fmt.Errorf("failed to add status db watchers: %w", err)
	}
	return nil
}
