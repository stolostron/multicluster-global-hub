package controller

import (
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/controller/syncers"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/controller/workers"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/consumer"
)

func AddToManager(mgr ctrl.Manager, agentConfig *config.AgentConfig) error {
	// add consumer to manager
	consumer, err := consumer.NewGenericConsumer(agentConfig.TransportConfig)
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

	// add generic syncer to manager
	genericSyncer := syncers.NewGenericSyncer(consumer, workers, agentConfig.SpecEnforceHohRbac)
	if err := mgr.Add(genericSyncer); err != nil {
		return fmt.Errorf("failed to add genericSyncer to runtime manager: %w", err)
	}

	// add managed cluster label syncer to manager
	clusterLabelSyncer := syncers.NewManagedClusterLabelSyncer(consumer, workers)
	if err := mgr.Add(genericSyncer); err != nil {
		return fmt.Errorf("failed to add clusterLabelSyncer to runtime manager: %w", err)
	}

	// add bundle dispatcher to manager
	dispatcher := NewGenericDispatcher(consumer, workers, *agentConfig)
	if err := mgr.Add(dispatcher); err != nil {
		return fmt.Errorf("failed to add bundle dispatcher to runtime manager: %w", err)
	}
	// register generic syncer channel to dispatcher
	dispatcher.RegisterChannel(syncers.GenericMessageKey, genericSyncer.Channel())
	// register managed cluster label syncer channel to dispatcher
	dispatcher.RegisterChannel(constants.ManagedClustersLabelsMsgKey, clusterLabelSyncer.Channel())
	return nil
}
