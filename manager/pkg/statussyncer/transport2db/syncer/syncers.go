package syncer

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/config"
	statusbundle "github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/bundle"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/db/workerpool"
	configctl "github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/syncer/config"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/syncer/dbsyncer"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/syncer/dispatcher"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/helpers"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/statistics"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/consumer"
)

// AddTransport2DBSyncers performs the initial setup required before starting the runtime manager.
// adds controllers and/or runnables to the manager, registers handler functions within the dispatcher
//
//	and create bundle functions within the bundle.
func AddTransport2DBSyncers(mgr ctrl.Manager, managerConfig *config.ManagerConfig) error {
	stats := getStatistic(mgr, managerConfig)
	// register statistics within the runtime manager
	if err := mgr.Add(stats); err != nil {
		return fmt.Errorf("failed to add statistics to manager - %w", err)
	}

	// conflationReadyQueue is shared between conflation manager and dispatcher
	conflationReadyQueue := conflator.NewConflationReadyQueue(stats)
	// manage all Conflation Units
	conflationManager := conflator.NewConflationManager(conflationReadyQueue,
		requireInitialDependencyChecks(managerConfig.TransportConfig.TransportType), stats)

	// create consumer
	consumer, err := consumer.NewGenericConsumer(managerConfig.TransportConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize transport consumer: %w", err)
	}
	if err := mgr.Add(consumer); err != nil {
		return fmt.Errorf("failed to add transport consumer to manager: %w", err)
	}

	// consume message from consumer and dispatcher it to conflation manager
	transportDispatcher := dispatcher.NewTransportDispatcher(
		ctrl.Log.WithName("transport-dispatcher"), consumer,
		conflationManager, stats)
	if err := mgr.Add(transportDispatcher); err != nil {
		return fmt.Errorf("failed to add transport dispatcher to runtime manager: %w", err)
	}

	// database layer initialization - worker pool + connection pool
	dbWorkerPool, err := workerpool.NewDBWorkerPool(managerConfig.DatabaseConfig.TransportBridgeDatabaseURL, stats)
	if err != nil {
		return fmt.Errorf("failed to initialize DBWorkerPool: %w", err)
	}
	if err := mgr.Add(dbWorkerPool); err != nil {
		return fmt.Errorf("failed to add DB worker pool: %w", err)
	}

	// add ConflationDispatcher to the runtime manager
	if err := mgr.Add(dispatcher.NewConflationDispatcher(
		ctrl.Log.WithName("conflation-dispatcher"),
		conflationReadyQueue, dbWorkerPool)); err != nil {
		return fmt.Errorf("failed to add conflation dispatcher to runtime manager: %w", err)
	}

	// register config controller within the runtime manager
	config, err := addConfigController(mgr)
	if err != nil {
		return fmt.Errorf("failed to add config controller to manager - %w", err)
	}

	// register db syncers create bundle functions within transport and handler functions within dispatcher
	dbSyncers := []dbsyncer.DBSyncer{
		dbsyncer.NewManagedClustersDBSyncer(ctrl.Log.WithName("managed-clusters-db-syncer")),
		dbsyncer.NewPoliciesDBSyncer(ctrl.Log.WithName("policies-db-syncer"), config),
		dbsyncer.NewPlacementRulesDBSyncer(ctrl.Log.WithName("placement-rules-db-syncer")),
		dbsyncer.NewPlacementsDBSyncer(ctrl.Log.WithName("placements-db-syncer")),
		dbsyncer.NewPlacementDecisionsDBSyncer(ctrl.Log.WithName("placement-decisions-db-syncer")),
		dbsyncer.NewSubscriptionStatusesDBSyncer(ctrl.Log.WithName("subscription-statuses-db-syncer")),
		dbsyncer.NewSubscriptionReportsDBSyncer(ctrl.Log.WithName("subscription-reports-db-syncer")),
		dbsyncer.NewLocalSpecDBSyncer(ctrl.Log.WithName("local-spec-db-syncer"), config),
		dbsyncer.NewControlInfoDBSyncer(ctrl.Log.WithName("control-info-db-syncer")),
	}

	for _, dbsyncerObj := range dbSyncers {
		dbsyncerObj.RegisterCreateBundleFunctions(transportDispatcher)
		dbsyncerObj.RegisterBundleHandlerFunctions(conflationManager)
	}

	return nil
}

func getStatistic(mgr ctrl.Manager, managerConfig *config.ManagerConfig) *statistics.Statistics {
	// create statistics
	return statistics.NewStatistics(ctrl.Log.WithName("statistics"), managerConfig.StatisticsConfig,
		[]string{
			helpers.GetBundleType(&statusbundle.ManagedClustersStatusBundle{}),
			helpers.GetBundleType(&statusbundle.ClustersPerPolicyBundle{}),
			helpers.GetBundleType(&statusbundle.CompleteComplianceStatusBundle{}),
			helpers.GetBundleType(&statusbundle.DeltaComplianceStatusBundle{}),
			helpers.GetBundleType(&statusbundle.MinimalComplianceStatusBundle{}),
			helpers.GetBundleType(&statusbundle.PlacementRulesBundle{}),
			helpers.GetBundleType(&statusbundle.PlacementsBundle{}),
			helpers.GetBundleType(&statusbundle.PlacementDecisionsBundle{}),
			helpers.GetBundleType(&statusbundle.SubscriptionStatusesBundle{}),
			helpers.GetBundleType(&statusbundle.SubscriptionReportsBundle{}),
			helpers.GetBundleType(&statusbundle.ControlInfoBundle{}),
			helpers.GetBundleType(&statusbundle.LocalPolicySpecBundle{}),
			helpers.GetBundleType(&statusbundle.LocalClustersPerPolicyBundle{}),
			helpers.GetBundleType(&statusbundle.LocalCompleteComplianceStatusBundle{}),
			helpers.GetBundleType(&statusbundle.LocalPlacementRulesBundle{}),
		})
}

func addConfigController(mgr ctrl.Manager) (*corev1.ConfigMap, error) {
	config := &corev1.ConfigMap{Data: map[string]string{"aggregationLevel": "full"}}
	// default value is full until the config is read from the CR

	if err := configctl.AddConfigController(mgr,
		ctrl.Log.WithName("multicluster-global-hub-config"),
		config,
	); err != nil {
		return nil, fmt.Errorf("failed to add config controller: %w", err)
	}

	return config, nil
}

// function to determine whether the transport component requires initial-dependencies between bundles to be checked
// (on load). If the returned is false, then we may assume that dependency of the initial bundle of
// each type is met. Otherwise, there are no guarantees and the dependencies must be checked.
func requireInitialDependencyChecks(transportType string) bool {
	switch transportType {
	case string(transport.Kafka):
		return false
		// once kafka consumer loads up, it starts reading from the earliest un-processed bundle,
		// as in all bundles that precede the latter have been processed, which include its dependency
		// bundle.

		// the order guarantee also guarantees that if while loading this component, a new bundle is written to a-
		// partition, then surely its dependency was written before it (leaf-hub-status-sync on kafka guarantees).
	default:
		return true
	}
}
