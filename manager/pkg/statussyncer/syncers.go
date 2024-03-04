package statussyncer

import (
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/config"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/conflator"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/dispatcher"
	localpolicies "github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/handler/local_policies"
	dbsyncer "github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/syncers"
	"github.com/stolostron/multicluster-global-hub/pkg/statistics"
)

// AddStatusSyncers performs the initial setup required before starting the runtime manager.
// adds controllers and/or runnables to the manager, registers handler functions within the dispatcher
//
//	and create bundle functions within the bundle.
func AddStatusSyncers(mgr ctrl.Manager, managerConfig *config.ManagerConfig) error {
	// create statistics
	stats := statistics.NewStatistics(managerConfig.StatisticsConfig)
	if err := mgr.Add(stats); err != nil {
		return fmt.Errorf("failed to add statistics to manager - %w", err)
	}

	// manage all Conflation Units
	conflationManager := conflator.NewConflationManager(stats)
	// register

	if err := dispatcher.AddTransportDispatcher(mgr, conflationManager, managerConfig, stats); err != nil {
		return err
	}

	if err := dispatcher.AddConflationDispatcher(mgr, conflationManager, managerConfig, stats); err != nil {
		return err
	}

	// add kafka offset to the database periodically
	committer := conflator.NewKafkaConflationCommitter(conflationManager.GetMetadatas)
	if err := mgr.Add(committer); err != nil {
		return fmt.Errorf("failed to start the offset committer: %w", err)
	}

	// register db syncers create bundle functions within transport and handler functions within dispatcher
	dbSyncers := []dbsyncer.Syncer{
		dbsyncer.NewHubClusterHeartbeatSyncer(ctrl.Log.WithName("hub-heartbeat-syncer")),
		dbsyncer.NewHubClusterInfoDBSyncer(ctrl.Log.WithName("hub-info-syncer")),
		dbsyncer.NewManagedClustersDBSyncer(ctrl.Log.WithName("managed-cluster-syncer")),
		dbsyncer.NewCompliancesDBSyncer(ctrl.Log.WithName("compliances-syncer")),
		dbsyncer.NewLocalPolicySpecSyncer(ctrl.Log.WithName("local-policy-spec-syncer")),
		dbsyncer.NewLocalPolicyEventSyncer(ctrl.Log.WithName("local-policy-event-syncer")),
	}

	if managerConfig.EnableGlobalResource {
		dbSyncers = append(dbSyncers,
			dbsyncer.NewPlacementRulesDBSyncer(ctrl.Log.WithName("placement-rules-db-syncer")),
			dbsyncer.NewPlacementsDBSyncer(ctrl.Log.WithName("placements-db-syncer")),
			dbsyncer.NewPlacementDecisionsDBSyncer(
				ctrl.Log.WithName("placement-decisions-db-syncer")),
			dbsyncer.NewSubscriptionStatusesDBSyncer(
				ctrl.Log.WithName("subscription-statuses-db-syncer")),
			dbsyncer.NewSubscriptionReportsDBSyncer(
				ctrl.Log.WithName("subscription-reports-db-syncer")),
			dbsyncer.NewLocalSpecPlacementruleSyncer(ctrl.Log.WithName("local-spec-placementrule-syncer")),
		)
	}

	for _, dbsyncerObj := range dbSyncers {
		dbsyncerObj.RegisterCreateBundleFunctions(transportDispatcher)
		dbsyncerObj.RegisterBundleHandlerFunctions(conflationManager)
	}

	// TODO: the handler will be process on the confaltion
	transportDispatcher.RegisterEventHandler(localpolicies.NewLocalRootPolicyEventHandler())
	transportDispatcher.RegisterEventHandler(localpolicies.NewLocalReplicatedPolicyEventHandler())

	return nil
}
