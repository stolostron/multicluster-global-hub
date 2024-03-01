package hubofhubs

import (
	"context"

	"k8s.io/apimachinery/pkg/api/equality"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	operatorutils "github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	commonconstants "github.com/stolostron/multicluster-global-hub/pkg/constants"
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

		// set the annotations for the managed hub
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

	for idx := range clusters.Items {
		managedHub := &clusters.Items[idx]
		if managedHub.Name == constants.LocalClusterName {
			continue
		}

		toUpdate := false
		annotations := managedHub.GetAnnotations()
		if _, ok := annotations[constants.AnnotationONMulticlusterHub]; ok {
			delete(annotations, constants.AnnotationONMulticlusterHub)
			toUpdate = true
		}
		if _, ok := annotations[constants.AnnotationPolicyONMulticlusterHub]; ok {
			delete(annotations, constants.AnnotationPolicyONMulticlusterHub)
			toUpdate = true
		}
		if utils.Contains(managedHub.GetFinalizers(), commonconstants.GlobalHubCleanupFinalizer) {
			managedHub.SetFinalizers(operatorutils.Remove(managedHub.GetFinalizers(),
				commonconstants.GlobalHubCleanupFinalizer))
			toUpdate = true
		}

		if !toUpdate {
			continue
		}
		r.Log.Info("remove annotations/finalizer from cluster", "cluster", managedHub.GetName(),
			"annotation", managedHub.Annotations, "finalizers", managedHub.Finalizers)
		if err := r.Update(ctx, managedHub, &client.UpdateOptions{}); err != nil {
			return err
		}
	}

	return nil
}
