// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controllers

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clustersv1alpha1 "open-cluster-management.io/api/cluster/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

type versionClusterClaimController struct {
	client client.Client
	log    logr.Logger
}

func (c *versionClusterClaimController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := c.log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.V(2).Info("cluster claim controller", "NamespacedName:", request.NamespacedName)

	mch, err := updateHubClusterClaim(ctx, c.client, request.NamespacedName)
	if err != nil {
		reqLogger.Error(err, "failed to update Hub clusterClaim")
		return ctrl.Result{}, err
	}

	if mch != nil && mch.Status.CurrentVersion != "" {
		return ctrl.Result{}, updateClusterClaim(ctx, c.client,
			constants.VersionClusterClaimName, mch.Status.CurrentVersion)
	}

	// requeue to wait the acm version is available
	return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 5}, nil
}

func StartVersionClusterClaimController(mgr ctrl.Manager) error {
	clusterClaimPredicate, _ := predicate.LabelSelectorPredicate(metav1.LabelSelector{
		MatchLabels: map[string]string{
			constants.GlobalHubOwnerLabelKey: constants.GHAgentOwnerLabelValue,
		},
	})
	return ctrl.NewControllerManagedBy(mgr).
		For(&clustersv1alpha1.ClusterClaim{}, builder.WithPredicates(clusterClaimPredicate)).
		Watches(&mchv1.MultiClusterHub{}, &handler.EnqueueRequestForObject{}).
		Complete(&versionClusterClaimController{
			client: mgr.GetClient(),
			log:    ctrl.Log.WithName("clusterclaim-controller"),
		})
}
