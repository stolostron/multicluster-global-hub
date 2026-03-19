package spec

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/hubha"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/migration"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/syncers"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
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

	// register single migration syncer that routes internally by payload
	sourceSyncer := migration.NewMigrationSourceSyncer(mgr.GetClient(),
		mgr.GetConfig(), transportClient, agentConfig)
	targetSyncer := migration.NewMigrationTargetSyncer(mgr.GetClient(),
		transportClient, agentConfig)
	dispatcher.RegisterSyncer(string(enum.ManagedClusterMigrationType),
		migration.NewMigrationSyncer(sourceSyncer, targetSyncer))
	if err := migration.ResyncMigrationEvent(context, transportClient, agentConfig.TransportConfig); err != nil {
		return fmt.Errorf("failed to resync migration event: %w", err)
	}

	// Register Hub HA standby syncer if this is a standby hub
	if agentConfig.HubRole == constants.GHHubRoleStandby {
		log.Info("registering Hub HA standby syncer - this is a standby hub")
		dispatcher.RegisterSyncer(constants.HubHAResourcesMsgKey,
			hubha.NewHubHAStandbySyncer(mgr.GetClient()))
	}

	dispatcher.RegisterSyncer(constants.ResyncMsgKey, syncers.NewResyncer())

	log.Info("added the spec controllers to manager")
	return nil
}
