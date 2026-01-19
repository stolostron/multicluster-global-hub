package status

import (
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/dispatcher"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/handlers"
	"github.com/stolostron/multicluster-global-hub/pkg/statistics"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var statusCtrlStarted = false

// AddStatusSyncers performs the initial setup required before starting the runtime manager.
// adds controllers and/or runnables to the manager, registers handler to conflation manager
func AddStatusSyncers(
	mgr ctrl.Manager,
	consumer transport.Consumer,
	requester transport.Requester,
	managerConfig *configs.ManagerConfig,
) error {
	if statusCtrlStarted {
		return nil
	}
	// create statistics
	stats := statistics.NewStatistics(managerConfig.StatisticsConfig)
	if err := mgr.Add(stats); err != nil {
		return err
	}

	// manage all Conflation Units and handlers
	conflationManager := conflator.NewConflationManager(stats, requester)
	handlers.RegisterHandlers(mgr, conflationManager)

	// start consume message from transport to conflation manager
	if err := dispatcher.AddTransportDispatcher(mgr, consumer, managerConfig, conflationManager, stats); err != nil {
		return err
	}

	// start persist event from conflation manager to database with registered handlers
	if err := dispatcher.AddConflationDispatcher(mgr, conflationManager, managerConfig, stats); err != nil {
		return err
	}

	// add kafka offset to the database periodically
	committer := conflator.NewKafkaConflationCommitter(conflationManager.GetMetadatas)
	if err := mgr.Add(committer); err != nil {
		return fmt.Errorf("failed to start the offset committer: %w", err)
	}
	statusCtrlStarted = true
	return nil
}
