package managedclusters

import (
	"fmt"

	clusterV1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/helper"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/bundle"
	agentstatusconfig "github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

const (
	clusterStatusSyncLogName = "clusters-status-sync"
)

// mgr, pro, env.LeafHubID, incarnation, config, syncIntervals
// AddClustersStatusController adds managed clusters status controller to the manager.
func AddClustersStatusController(mgr ctrl.Manager, producer transport.Producer) error {
	createObjFunction := func() bundle.Object { return &clusterV1.ManagedCluster{} }
	leafHubName := agentstatusconfig.GetLeafHubName()

	transportBundleKey := fmt.Sprintf("%s.%s", leafHubName, constants.ManagedClustersMsgKey)

	// update bundle object
	manipulateObjFunc := func(object bundle.Object) {
		helper.AddAnnotations(object, map[string]string{
			constants.ManagedClusterManagedByAnnotation: leafHubName,
		})
	}

	predicateFunc := func() bool {
		// return hubOfHubsConfig.Data["aggregationLevel"] == "full" ||
		// 	hubOfHubsConfig.Data["aggregationLevel"] == "minimal"
		// at this point send all managed clusters even if aggregation level is minimal
		return true
	}

	bundleCollection := []*generic.BundleCollectionEntry{ // single bundle for managed clusters
		generic.NewBundleCollectionEntry(transportBundleKey,
			bundle.NewGenericStatusBundle(leafHubName, manipulateObjFunc),
			predicateFunc),
	}

	if err := generic.NewGenericStatusSyncController(mgr, clusterStatusSyncLogName, producer, bundleCollection,
		createObjFunction, nil, agentstatusconfig.GetManagerClusterDuration); err != nil {
		return fmt.Errorf("failed to add managed clusters controller to the manager - %w", err)
	}

	return nil
}
