package controller

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	clustersv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/helper"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/controller/syncers"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/controller/workers"
	consumer "github.com/stolostron/multicluster-global-hub/agent/pkg/transport/consumer"
)

// AddToScheme adds only resources that have to be fetched.
// (no need to add scheme of resources that are applied as a part of generic bundles).
func AddToScheme(scheme *runtime.Scheme) error {
	if err := clustersv1.Install(scheme); err != nil {
		return fmt.Errorf("failed to install scheme: %w", err)
	}
	return nil
}

// AddSpecSyncers adds spec syncers to the Manager.
func AddSyncersToManager(manager ctrl.Manager, consumer consumer.Consumer, configManager helper.ConfigManager) error {
	workerPool, err := workers.AddWorkerPool(ctrl.Log.WithName("workers-pool"),
		configManager.SpecWorkPoolSize, manager)
	if err != nil {
		return fmt.Errorf("failed to add worker pool to runtime manager: %w", err)
	}

	if err = syncers.AddGenericBundleSyncer(ctrl.Log.WithName("generic-bundle-syncer"), manager,
		configManager.SpecEnforceHohRbac, consumer, workerPool); err != nil {
		return fmt.Errorf("failed to add bundles spec syncer to runtime manager: %w", err)
	}

	// add managed cluster labels syncer to mgr
	if err = syncers.AddManagedClusterLabelsBundleSyncer(
		ctrl.Log.WithName("managed-clusters-labels-syncer"), manager,
		consumer, workerPool); err != nil {
		return fmt.Errorf("failed to add managed cluster labels syncer to runtime manager: %w", err)
	}

	return nil
}
