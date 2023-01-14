package controller

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	clustersv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/controller/syncers"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/controller/workers"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/consumer"
)

// AddToScheme adds only resources that have to be fetched.
// (no need to add scheme of resources that are applied as a part of generic bundles).
func AddToScheme(scheme *runtime.Scheme) error {
	if err := clustersv1.Install(scheme); err != nil {
		return fmt.Errorf("failed to install scheme: %w", err)
	}
	return nil
}

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

	// add bundle dispatcher to manager
	dispatcher := NewGenericDispatcher(consumer, workers, *agentConfig)
	if err := mgr.Add(dispatcher); err != nil {
		return fmt.Errorf("failed to add bundle dispatcher to runtime manager: %w", err)
	}

	// add syncer to manager
	syncer := syncers.NewGenericSyncer(consumer, workers, agentConfig.SpecEnforceHohRbac)
	if err := mgr.Add(syncer); err != nil {
		return fmt.Errorf("failed to add bundles spec syncer to runtime manager: %w", err)
	}

	return nil
}
