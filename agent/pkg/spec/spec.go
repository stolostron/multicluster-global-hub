package spec

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/migration"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/syncers"
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

	// add bundle dispatcher to manager
	dispatcher, err := AddGenericDispatcher(mgr, transportClient.GetConsumer(), *agentConfig)
	if err != nil {
		return fmt.Errorf("failed to add bundle dispatcher to runtime manager: %w", err)
	}

	// register syncer to the dispatcher
	dispatcher.RegisterSyncer(constants.MigrationSourceMsgKey,
		migration.NewMigrationSourceSyncer(mgr.GetClient(),
			mgr.GetConfig(), transportClient, agentConfig))

	dispatcher.RegisterSyncer(constants.MigrationTargetMsgKey,
		migration.NewMigrationTargetSyncer(mgr.GetClient(),
			transportClient, agentConfig))
	if err := migration.ResyncMigrationEvent(context, transportClient, agentConfig.TransportConfig); err != nil {
		return fmt.Errorf("failed to resync migration event: %w", err)
	}

	dispatcher.RegisterSyncer(constants.ResyncMsgKey, syncers.NewResyncer())

	log.Info("added the spec controllers to manager")
	return nil
}
