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
		return fmt.Errorf("the consumer is not initialized")
	}
	if transportClient.GetProducer() == nil {
		return fmt.Errorf("the producer is not initialized")
	}

	// add worker pool to manager
	_, err := workers.AddWorkerPoolToMgr(mgr, agentConfig.SpecWorkPoolSize, mgr.GetConfig())
	if err != nil {
		return fmt.Errorf("failed to add k8s workers pool to runtime manager: %w", err)
	}

	// add bundle dispatcher to manager
	dispatcher, err := AddGenericDispatcher(mgr, transportClient.GetConsumer(), *agentConfig)
	if err != nil {
		return fmt.Errorf("failed to add bundle dispatcher to runtime manager: %w", err)
	}

	// register syncer to the dispatcher

	dispatcher.RegisterSyncer(constants.MigrationSourceMsgKey,
		syncers.NewMigrationSourceSyncer(mgr.GetClient(), mgr.GetConfig(), transportClient, agentConfig.TransportConfig))
	dispatcher.RegisterSyncer(constants.MigrationTargetMsgKey,
		syncers.NewMigrationTargetSyncer(mgr.GetClient(), transportClient, agentConfig.TransportConfig))

	dispatcher.RegisterSyncer(constants.ResyncMsgKey, syncers.NewResyncer())

	log.Info("added the spec controllers to manager")
	return nil
}
