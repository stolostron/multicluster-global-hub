package syncer

import (
	"fmt"

	"github.com/stolostron/hub-of-hubs/manager/pkg/statussyncer/transport2db/conflator"
	"github.com/stolostron/hub-of-hubs/manager/pkg/statussyncer/transport2db/db/workerpool"
	configctl "github.com/stolostron/hub-of-hubs/manager/pkg/statussyncer/transport2db/syncer/config"
	"github.com/stolostron/hub-of-hubs/manager/pkg/statussyncer/transport2db/syncer/dbsyncer"
	"github.com/stolostron/hub-of-hubs/manager/pkg/statussyncer/transport2db/syncer/dispatcher"
	"github.com/stolostron/hub-of-hubs/manager/pkg/statussyncer/transport2db/transport"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// AddTransport2DBSyncers performs the initial setup required before starting the runtime manager.
// adds controllers and/or runnables to the manager, registers handler functions within the dispatcher and create bundle
// functions within the transport.
func AddTransport2DBSyncers(mgr ctrl.Manager, dbWorkerPool *workerpool.DBWorkerPool, conflationManager *conflator.ConflationManager,
	conflationReadyQueue *conflator.ConflationReadyQueue, transport transport.Transport,
	statistics manager.Runnable,
) error {
	// register config controller within the runtime manager
	config, err := addConfigController(mgr)
	if err != nil {
		return fmt.Errorf("failed to add config controller to manager - %w", err)
	}
	// register statistics within the runtime manager
	if err := mgr.Add(statistics); err != nil {
		return fmt.Errorf("failed to add statistics to manager - %w", err)
	}
	// register dispatcher within the runtime manager
	if err := addDispatcher(mgr, dbWorkerPool, conflationReadyQueue); err != nil {
		return fmt.Errorf("failed to add dispatcher to manager - %w", err)
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
		dbsyncerObj.RegisterCreateBundleFunctions(transport)
		dbsyncerObj.RegisterBundleHandlerFunctions(conflationManager)
	}

	return nil
}

func addConfigController(mgr ctrl.Manager) (*corev1.ConfigMap, error) {
	config := &corev1.ConfigMap{Data: map[string]string{"aggregationLevel": "full"}} // default value is full until the config is read from the CR

	if err := configctl.AddConfigController(mgr,
		ctrl.Log.WithName("hub-of-hubs-config"),
		config,
	); err != nil {
		return nil, fmt.Errorf("failed to add config controller: %w", err)
	}

	return config, nil
}

func addDispatcher(mgr ctrl.Manager, dbWorkerPool *workerpool.DBWorkerPool,
	conflationReadyQueue *conflator.ConflationReadyQueue,
) error {
	if err := mgr.Add(dispatcher.NewDispatcher(
		ctrl.Log.WithName("dispatcher"),
		conflationReadyQueue,
		dbWorkerPool,
	)); err != nil {
		return fmt.Errorf("failed to add dispatcher: %w", err)
	}

	return nil
}
