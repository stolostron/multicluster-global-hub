package statussyncer

import (
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/config"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/dispatcher"
	localpolicies "github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/handler/local_policies"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/hubmanagement"
	dbsyncer "github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/syncers"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/cluster"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/grc"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/placement"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator/workerpool"
	"github.com/stolostron/multicluster-global-hub/pkg/statistics"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/consumer"
)

// AddStatusSyncers performs the initial setup required before starting the runtime manager.
// adds controllers and/or runnables to the manager, registers handler functions within the dispatcher
//
//	and create bundle functions within the bundle.
func AddStatusSyncers(mgr ctrl.Manager, managerConfig *config.ManagerConfig,
	producer transport.Producer,
) (dbsyncer.BundleRegisterable, error) {
	// register statistics within the runtime manager
	stats, err := addStatisticController(mgr, managerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to add statistics to manager - %w", err)
	}

	// add hub management
	if err := hubmanagement.AddHubManagement(mgr, producer); err != nil {
		return nil, fmt.Errorf("failed to add hubmanagement to manager - %w", err)
	}

	// conflationReadyQueue is shared between conflation manager and dispatcher
	conflationReadyQueue := conflator.NewConflationReadyQueue(stats)
	// manage all Conflation Units
	conflationManager := conflator.NewConflationManager(conflationReadyQueue, stats)

	// add kafka offset to the database periodically
	committer := conflator.NewKafkaConflationCommitter(conflationManager.GetTransportMetadatas)
	if err := mgr.Add(committer); err != nil {
		return nil, fmt.Errorf("failed to add DB worker pool: %w", err)
	}

	// database layer initialization - worker pool + connection pool
	dbWorkerPool, err := workerpool.NewDBWorkerPool(stats)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize DBWorkerPool: %w", err)
	}
	if err := mgr.Add(dbWorkerPool); err != nil {
		return nil, fmt.Errorf("failed to add DB worker pool: %w", err)
	}

	transportDispatcher, err := getTransportDispatcher(mgr, conflationManager, managerConfig, stats)
	if err != nil {
		return nil, fmt.Errorf("failed to get transport dispatcher: %w", err)
	}

	// add ConflationDispatcher to the runtime manager
	if err := mgr.Add(dispatcher.NewConflationDispatcher(
		ctrl.Log.WithName("conflation-dispatcher"),
		conflationReadyQueue, dbWorkerPool)); err != nil {
		return nil, fmt.Errorf("failed to add conflation dispatcher to runtime manager: %w", err)
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

	return transportDispatcher, nil
}

// the transport dispatcher implement the BundleRegister() method, which can dispatch message to syncers
// both kafkaConsumer and Cloudevents transport dispatcher will forward message to conflation manager
func getTransportDispatcher(mgr ctrl.Manager, conflationManager *conflator.ConflationManager,
	managerConfig *config.ManagerConfig, stats *statistics.Statistics,
) (*dispatcher.TransportDispatcher, error) {
	consumeConfig := managerConfig.TransportConfig.KafkaConfig.ConsumerConfig
	consumer, err := consumer.NewGenericConsumer(managerConfig.TransportConfig,
		[]string{consumeConfig.EventTopic, consumeConfig.StatusTopic},
		consumer.EnableDatabaseOffset(true))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize transport consumer: %w", err)
	}
	if err := mgr.Add(consumer); err != nil {
		return nil, fmt.Errorf("failed to add transport consumer to manager: %w", err)
	}
	// consume message from consumer and dispatcher it to conflation manager
	transportDispatcher := dispatcher.NewTransportDispatcher(
		ctrl.Log.WithName("transport-dispatcher"), consumer,
		conflationManager, stats)
	if err := mgr.Add(transportDispatcher); err != nil {
		return nil, fmt.Errorf("failed to add transport dispatcher to runtime manager: %w", err)
	}
	return transportDispatcher, nil
}

// only statistic the local policy and managed clusters
func addStatisticController(mgr ctrl.Manager, managerConfig *config.ManagerConfig) (*statistics.Statistics, error) {
	bundleTypes := []string{
		bundle.GetBundleType(&cluster.HubClusterInfoBundle{}),
		bundle.GetBundleType(&cluster.ManagedClusterBundle{}),
		bundle.GetBundleType(&grc.LocalPolicyBundle{}),
		bundle.GetBundleType(&grc.LocalComplianceBundle{}),
		bundle.GetBundleType(&grc.LocalCompleteComplianceBundle{}),
		// bundle.GetBundleType(&grc.LocalReplicatedPolicyEventBundle{}),
		// bundle.GetBundleType(&placement.LocalPlacementRulesBundle{}),
	}

	if managerConfig.EnableGlobalResource {
		bundleTypes = append(bundleTypes,
			bundle.GetBundleType(&placement.PlacementsBundle{}),
			bundle.GetBundleType(&placement.PlacementDecisionsBundle{}),
			bundle.GetBundleType(&grc.ComplianceBundle{}),
			bundle.GetBundleType(&grc.CompleteComplianceBundle{}),
		)
	}
	// create statistics
	stats := statistics.NewStatistics(managerConfig.StatisticsConfig, bundleTypes)

	if err := mgr.Add(stats); err != nil {
		return nil, fmt.Errorf("failed to add statistics to manager - %w", err)
	}
	return stats, nil
}
