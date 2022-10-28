// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controllers

import (
	"context"

	"github.com/go-logr/logr"
	clustersv1alpha1 "open-cluster-management.io/api/cluster/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

type hubClusterClaimController struct {
	client client.Client
	log    logr.Logger
}

func (c *hubClusterClaimController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := c.log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.V(2).Info("hub clusterClaim controller", request.NamespacedName)

	var err error
	mch, err := getMCH(ctx, c.client, request.NamespacedName)
	if err != nil {
		reqLogger.Error(err, "failed to get MCH instance")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, updateHubClusterClaim(ctx, c.client, mch)
}

func StartHubClusterClaimController(mgr ctrl.Manager) error {
	// the controller is only to trigger create hub clusterClaim at the beginning
	// do nothing if the hub clusterClaim existed
	clusterClaimPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		if object.GetName() == constants.HubClusterClaimName {
			return false
		}
		clusterClaim, _ := getClusterClaim(context.Background(), mgr.GetClient(), constants.HubClusterClaimName)
		return clusterClaim == nil
	})
	return ctrl.NewControllerManagedBy(mgr).
		For(&clustersv1alpha1.ClusterClaim{}, builder.WithPredicates(clusterClaimPredicate)).
		Complete(&hubClusterClaimController{
			client: mgr.GetClient(),
			log:    ctrl.Log.WithName("hubclusterclaim-controller"),
		})
}
