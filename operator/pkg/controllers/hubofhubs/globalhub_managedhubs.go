package hubofhubs

import (
	"context"
	"strings"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *MulticlusterGlobalHubReconciler) reconcileManagedHubs(ctx context.Context) error {

	clusters := &clusterv1.ManagedClusterList{}
	if err := r.List(ctx, clusters, &client.ListOptions{}); err != nil {
		return err
	}

	for _, managedHub := range clusters.Items {
		if managedHub.Name == constants.LocalClusterName {
			continue
		}
		annotations := managedHub.GetAnnotations()
		if val, ok := annotations[constants.AnnotationONMulticlusterHub]; ok {
			if !strings.EqualFold(val, "true") {
				managedHub.SetAnnotations(map[string]string{
					constants.AnnotationONMulticlusterHub: "true",
				})
				if err := r.Update(ctx, &managedHub, &client.UpdateOptions{}); err != nil {
					return err
				}
			}
			continue
		}
		// does not have the annotation, add it
		managedHub.SetAnnotations(map[string]string{
			constants.AnnotationONMulticlusterHub: "true",
		})
		if err := r.Update(ctx, &managedHub, &client.UpdateOptions{}); err != nil {
			return err
		}
	}

	return nil

}
