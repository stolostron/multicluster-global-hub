package hubofhubs

import (
	"context"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"k8s.io/apimachinery/pkg/api/equality"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *MulticlusterGlobalHubReconciler) reconcileManagedHubs(ctx context.Context) error {

	clusters := &clusterv1.ManagedClusterList{}
	if err := r.List(ctx, clusters, &client.ListOptions{}); err != nil {
		return err
	}

	for idx, managedHub := range clusters.Items {
		if managedHub.Name == constants.LocalClusterName {
			continue
		}
		annotations := managedHub.GetAnnotations()
		clusters.Items[idx].SetAnnotations(map[string]string{
			constants.AnnotationONMulticlusterHub:       "true",
			constants.AnnotationPolicyONMulticlusterHub: "true",
		})
		if !equality.Semantic.DeepEqual(annotations, clusters.Items[idx].GetAnnotations()) {
			if err := r.Update(ctx, &clusters.Items[idx], &client.UpdateOptions{}); err != nil {
				return err
			}
		}
	}

	return nil

}

func (r *MulticlusterGlobalHubReconciler) pruneManagedHubs(ctx context.Context) error {

	clusters := &clusterv1.ManagedClusterList{}
	if err := r.List(ctx, clusters, &client.ListOptions{}); err != nil {
		return err
	}

	for idx, managedHub := range clusters.Items {
		if managedHub.Name == constants.LocalClusterName {
			continue
		}
		annotations := managedHub.GetAnnotations()
		delete(annotations, constants.AnnotationONMulticlusterHub)
		delete(annotations, constants.AnnotationPolicyONMulticlusterHub)
		if !equality.Semantic.DeepEqual(annotations, clusters.Items[idx].GetAnnotations()) {
			if err := r.Update(ctx, &clusters.Items[idx], &client.UpdateOptions{}); err != nil {
				return err
			}
		}
	}

	return nil

}
