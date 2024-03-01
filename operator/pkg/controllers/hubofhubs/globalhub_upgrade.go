package hubofhubs

import (
	"context"

	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	operatorutils "github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	commonconstants "github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func (r *MulticlusterGlobalHubReconciler) upgrade(ctx context.Context) error {
	clusters := &clusterv1.ManagedClusterList{}
	if err := r.List(ctx, clusters, &client.ListOptions{}); err != nil {
		return err
	}

	for idx := range clusters.Items {
		managedHub := &clusters.Items[idx]
		if managedHub.Name == constants.LocalClusterName {
			continue
		}

		if utils.Contains(managedHub.GetFinalizers(), commonconstants.GlobalHubCleanupFinalizer) {
			managedHub.SetFinalizers(operatorutils.Remove(managedHub.GetFinalizers(),
				commonconstants.GlobalHubCleanupFinalizer))
			r.Log.Info("remove cleanup finalizer from cluster", "cluster", managedHub.GetName(),
				"finalizers", managedHub.Finalizers)
			if err := r.Update(ctx, managedHub, &client.UpdateOptions{}); err != nil {
				return err
			}
		}
	}

	return nil
}
