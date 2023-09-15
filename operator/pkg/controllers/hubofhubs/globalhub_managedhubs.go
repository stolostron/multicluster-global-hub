package hubofhubs

import (
	"context"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
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
		orgAnnotations := managedHub.GetAnnotations()
		if orgAnnotations == nil {
			orgAnnotations = make(map[string]string)
		}
		annotations := make(map[string]string, len(orgAnnotations))
		utils.CopyMap(annotations, managedHub.GetAnnotations())

		//set the annotations for the managed hub
		orgAnnotations[constants.AnnotationONMulticlusterHub] = "true"
		orgAnnotations[constants.AnnotationPolicyONMulticlusterHub] = "true"
		if !equality.Semantic.DeepEqual(annotations, orgAnnotations) {
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
		orgAnnotations := managedHub.GetAnnotations()
		if orgAnnotations == nil {
			continue
		}
		annotations := make(map[string]string, len(orgAnnotations))
		utils.CopyMap(annotations, managedHub.GetAnnotations())

		delete(orgAnnotations, constants.AnnotationONMulticlusterHub)
		delete(orgAnnotations, constants.AnnotationPolicyONMulticlusterHub)
		if !equality.Semantic.DeepEqual(annotations, orgAnnotations) {
			if err := r.Update(ctx, &clusters.Items[idx], &client.UpdateOptions{}); err != nil {
				return err
			}
		}
	}

	return nil

}
