package statussyncer

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/config"
	statusbundle "github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/bundle"
	configctl "github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/config"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/dispatcher"
	dbsyncer "github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/syncers"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/helpers"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator/db/workerpool"
	"github.com/stolostron/multicluster-global-hub/pkg/statistics"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/consumer"
)

// AddStatusSyncers performs the initial setup required before starting the runtime manager.
// adds controllers and/or runnables to the manager, registers handler functions within the dispatcher
//
//	and create bundle functions within the bundle.
func AddStatusSyncers(mgr ctrl.Manager, managerConfig *config.ManagerConfig) (
	dbsyncer.BundleRegisterable, error,
) {
	// register statistics within the runtime manager
	stats, err := addStatisticController(mgr, managerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to add statistics to manager - %w", err)
	}

	// conflationReadyQueue is shared between conflation manager and dispatcher
	conflationReadyQueue := conflator.NewConflationReadyQueue(stats)
	// manage all Conflation Units
	conflationManager := conflator.NewConflationManager(conflationReadyQueue,
		requireInitialDependencyChecks(managerConfig.TransportConfig.TransportType), stats)

	// database layer initialization - worker pool + connection pool
	dbWorkerPool, err := workerpool.NewDBWorkerPool(managerConfig.DatabaseConfig, stats)
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

	// register config controller within the runtime manager
	config, err := addConfigController(mgr)
	if err != nil {
		return nil, fmt.Errorf("failed to add config controller to manager - %w", err)
	}

	// register db syncers create bundle functions within transport and handler functions within dispatcher
	dbSyncers := []dbsyncer.DBSyncer{
		dbsyncer.NewManagedClustersDBSyncer(ctrl.Log.WithName("managed-clusters-db-syncer")),
		dbsyncer.NewPoliciesDBSyncer(ctrl.Log.WithName("policies-db-syncer"), config),
		dbsyncer.NewPlacementRulesDBSyncer(ctrl.Log.WithName("placement-rules-db-syncer")),
		dbsyncer.NewPlacementsDBSyncer(ctrl.Log.WithName("placements-db-syncer")),
		dbsyncer.NewPlacementDecisionsDBSyncer(ctrl.Log.WithName("placement-decisions-db-syncer")),
		dbsyncer.NewHubClusterInfoDBSyncer(ctrl.Log.WithName("hub-cluster-info-db-syncer")),
		dbsyncer.NewSubscriptionStatusesDBSyncer(ctrl.Log.WithName("subscription-statuses-db-syncer")),
		dbsyncer.NewSubscriptionReportsDBSyncer(ctrl.Log.WithName("subscription-reports-db-syncer")),
		dbsyncer.NewLocalSpecPlacementruleSyncer(ctrl.Log.WithName(
			"local-spec-placementrule-syncer"), config),
		dbsyncer.NewLocalSpecPoliciesSyncer(ctrl.Log.WithName("local-spec-policy-syncer"), config),
		dbsyncer.NewControlInfoDBSyncer(ctrl.Log.WithName("control-info-db-syncer")),
	}

	for _, dbsyncerObj := range dbSyncers {
		dbsyncerObj.RegisterCreateBundleFunctions(transportDispatcher)
		dbsyncerObj.RegisterBundleHandlerFunctions(conflationManager)
	}

	return transportDispatcher, nil
}

// the transport dispatcher implement the BundleRegister() method, which can dispatch message to syncers
// both kafkaConsumer and Cloudevents transport dispatcher will forward message to conflation manager
func getTransportDispatcher(mgr ctrl.Manager, conflationManager *conflator.ConflationManager,
	managerConfig *config.ManagerConfig, stats *statistics.Statistics,
) (dbsyncer.BundleRegisterable, error) {
	if managerConfig.TransportConfig.TransportFormat == string(transport.KafkaMessageFormat) {
		kafkaConsumer, err := consumer.NewKafkaConsumer(
			managerConfig.TransportConfig.KafkaConfig,
			ctrl.Log.WithName("message-consumer"))
		if err != nil {
			return nil, fmt.Errorf("failed to create kafka-consumer: %w", err)
		}
		kafkaConsumer.SetConflationManager(conflationManager)
		kafkaConsumer.SetCommitter(consumer.NewCommitter(
			managerConfig.TransportConfig.CommitterInterval,
			managerConfig.TransportConfig.KafkaConfig.ConsumerConfig.ConsumerTopic, kafkaConsumer.Consumer(),
			conflationManager.GetBundlesMetadata, ctrl.Log.WithName("message-consumer")),
		)
		kafkaConsumer.SetStatistics(stats)
		if err := mgr.Add(kafkaConsumer); err != nil {
			return nil, fmt.Errorf("failed to add status transport bridge: %w", err)
		}
		return kafkaConsumer, nil
	} else {
		consumer, err := consumer.NewGenericConsumer(managerConfig.TransportConfig)
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

func addStatisticController(mgr ctrl.Manager, managerConfig *config.ManagerConfig) (*statistics.Statistics, error) {
	// create statistics
	stats := statistics.NewStatistics(ctrl.Log.WithName("statistics"), managerConfig.StatisticsConfig,
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
			helpers.GetBundleType(&status.BaseLeafHubClusterInfoStatusBundle{}),
		})
	if err := mgr.Add(stats); err != nil {
		return nil, fmt.Errorf("failed to add statistics to manager - %w", err)
	}
	return stats, nil
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
