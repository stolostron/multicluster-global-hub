package statussyncer

import (
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/config"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/conflator"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/dispatcher"
	dbsyncer "github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/syncers"
	"github.com/stolostron/multicluster-global-hub/pkg/statistics"
)

// AddStatusSyncers performs the initial setup required before starting the runtime manager.
// adds controllers and/or runnables to the manager, registers handler to conflation manager
func AddStatusSyncers(mgr ctrl.Manager, managerConfig *config.ManagerConfig) error {
	// create statistics
	stats := statistics.NewStatistics(managerConfig.StatisticsConfig)
	if err := mgr.Add(stats); err != nil {
		return err
	}

	// manage all Conflation Units and handlers
	conflationManager := conflator.NewConflationManager(stats)
	registerHandler(conflationManager, managerConfig.EnableGlobalResource)

	// start consume message from transport to conflation manager
	if err := dispatcher.AddTransportDispatcher(mgr, managerConfig, conflationManager, stats); err != nil {
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
	return nil
}

func registerHandler(cmr *conflator.ConflationManager, enableGlobalResource bool) {
	dbsyncer.NewHubClusterHeartbeatHandler().RegisterHandler(cmr)
	dbsyncer.NewHubClusterInfoHandler().RegisterHandler(cmr)
	dbsyncer.NewManagedClusterHandler().RegisterHandler(cmr)
	dbsyncer.NewManagedClusterEventHandler().RegisterHandler(cmr)
	dbsyncer.NewLocalPolicySpecHandler().RegisterHandler(cmr)
	dbsyncer.NewLocalPolicyComplianceHandler().RegisterHandler(cmr)
	dbsyncer.NewLocalPolicyCompleteHandler().RegisterHandler(cmr)
	dbsyncer.NewLocalRootPolicyEventHandler().RegisterHandler(cmr)
	dbsyncer.NewLocalReplicatedPolicyEventHandler().RegisterHandler(cmr)
	dbsyncer.NewLocalPlacementRuleSpecHandler().RegisterHandler(cmr)
	if enableGlobalResource {
		dbsyncer.NewPolicyComplianceHandler().RegisterHandler(cmr)
		dbsyncer.NewPolicyCompleteHandler().RegisterHandler(cmr)
		dbsyncer.NewPolicyDeltaComplianceHandler().RegisterHandler(cmr)
		dbsyncer.NewPolicyMiniComplianceHandler().RegisterHandler(cmr)

		dbsyncer.NewPlacementRuleHandler().RegisterHandler(cmr)
		dbsyncer.NewPlacementHandler().RegisterHandler(cmr)
		dbsyncer.NewPlacementDecisionHandler().RegisterHandler(cmr)

		dbsyncer.NewSubscriptionReportHandler().RegisterHandler(cmr)
		dbsyncer.NewSubscriptionStatusHandler().RegisterHandler(cmr)
	}
}
