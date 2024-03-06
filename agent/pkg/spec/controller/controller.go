package controller

import (
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/controller/syncers"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/controller/workers"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	genericconsumer "github.com/stolostron/multicluster-global-hub/pkg/transport/consumer"
)

func AddToManager(mgr ctrl.Manager, agentConfig *config.AgentConfig) error {
	// add consumer to manager
	consumer, err := genericconsumer.NewGenericConsumer(agentConfig.TransportConfig,
		[]string{agentConfig.TransportConfig.KafkaConfig.Topics.SpecTopic},
	)
	if err != nil {
		return fmt.Errorf("failed to initialize transport consumer: %w", err)
	}
	if err := mgr.Add(consumer); err != nil {
		return fmt.Errorf("failed to add transport consumer to manager: %w", err)
	}

	// add worker pool to manager
	workers := workers.NewWorkerPool(agentConfig.SpecWorkPoolSize, mgr.GetConfig())
	if err := mgr.Add(workers); err != nil {
		return fmt.Errorf("failed to add k8s workers pool to runtime manager: %w", err)
	}

	// add bundle dispatcher to manager
	dispatcher := syncers.NewGenericDispatcher(consumer, *agentConfig)
	if err := mgr.Add(dispatcher); err != nil {
		return fmt.Errorf("failed to add bundle dispatcher to runtime manager: %w", err)
	}

	// register syncer to the dispatcher
	if agentConfig.EnableGlobalResource {
		dispatcher.RegisterSyncer(syncers.GenericMessageKey,
			syncers.NewGenericSyncer(workers, agentConfig))
		dispatcher.RegisterSyncer(constants.ManagedClustersLabelsMsgKey,
			syncers.NewManagedClusterLabelSyncer(workers))
	}

	dispatcher.RegisterSyncer(constants.ResyncMsgKey, syncers.NewResyncSyncer())
	return nil
}
