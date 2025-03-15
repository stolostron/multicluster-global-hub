package spec

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/syncers"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/workers"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

func AddToManager(context context.Context, mgr ctrl.Manager, transportClient transport.TransportClient,
	agentConfig *configs.AgentConfig,
) error {
	log := logger.DefaultZapLogger()

	if transportClient.GetConsumer() == nil {
		log.Info("the consumer is not initialized for the spec controllers")
		return fmt.Errorf("the consumer is not initialized")
	}
	if transportClient.GetProducer() == nil {
		log.Info("the producer is not initialized for the spec controllers")
		return fmt.Errorf("the producer is not initialized")
	}

	// add worker pool to manager
	workers, err := workers.AddWorkerPoolToMgr(mgr, agentConfig.SpecWorkPoolSize, mgr.GetConfig())
	if err != nil {
		return fmt.Errorf("failed to add k8s workers pool to runtime manager: %w", err)
	}

	// add bundle dispatcher to manager
	dispatcher, err := AddGenericDispatcher(mgr, transportClient.GetConsumer(), *agentConfig)
	if err != nil {
		return fmt.Errorf("failed to add bundle dispatcher to runtime manager: %w", err)
	}

	log.Infof("agentConfig.EnableGlobalResource:%v", agentConfig.EnableGlobalResource)
	// register syncer to the dispatcher
	if agentConfig.EnableGlobalResource {
		dispatcher.RegisterSyncer(constants.GenericSpecMsgKey,
			syncers.NewGenericSyncer(workers, agentConfig))
		log.Debugf("regist ManagedClustersLabelsMsgKey")
		dispatcher.RegisterSyncer(constants.ManagedClustersLabelsMsgKey,
			syncers.NewManagedClusterLabelSyncer(workers))
	}

	dispatcher.RegisterSyncer(constants.CloudEventTypeMigrationFrom,
		syncers.NewManagedClusterMigrationFromSyncer(mgr.GetClient(), transportClient))
	dispatcher.RegisterSyncer(constants.CloudEventTypeMigrationTo,
		syncers.NewManagedClusterMigrationToSyncer(mgr.GetClient()))
	dispatcher.RegisterSyncer(constants.ResyncMsgKey, syncers.NewResyncer())

	log.Info("added the spec controllers to manager")
	return nil
}
