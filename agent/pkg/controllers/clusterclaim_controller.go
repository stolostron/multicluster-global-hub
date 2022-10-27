// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controllers

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clustersv1alpha1 "open-cluster-management.io/api/cluster/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

type clusterClaimController struct {
	client client.Client
	log    logr.Logger
}

func (c *clusterClaimController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := c.log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.V(2).Info("cluster claim controller", request.NamespacedName)

	var err error
	mch, err := getMCH(ctx, c.client, request.NamespacedName)
	if err != nil {
		reqLogger.Error(err, "failed to get MCH instance")
		return ctrl.Result{}, err
	}

	if mch == nil {
		return ctrl.Result{}, updateClusterClaim(ctx, c.client,
			constants.HubClusterClaimName, constants.HubNotInstalled)
	}

	hubValue := constants.HubInstalledWithoutSelfManagement
	if mch.GetLabels()[constants.GlobalHubOwnerLabelKey] == constants.GlobalHubOwnerLabelVal {
		hubValue = constants.HubInstalledByHoH
	} else if !mch.Spec.DisableHubSelfManagement {
		hubValue = constants.HubInstalledWithSelfManagement
	}
	if err = updateClusterClaim(ctx, c.client, constants.HubClusterClaimName, hubValue); err != nil {
		return ctrl.Result{}, err
	}

	if mch.Status.CurrentVersion != "" {
		return ctrl.Result{}, updateClusterClaim(ctx, c.client,
			constants.VersionClusterClaimName, mch.Status.CurrentVersion)
	}

	// requeue to wait the acm version is available
	return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 5}, nil
}

func getMCH(ctx context.Context, client client.Client,
	NamespacedName types.NamespacedName,
) (*mchv1.MultiClusterHub, error) {
	if NamespacedName.Name == "" || NamespacedName.Namespace == "" {
		return listMCH(ctx, client)
	}

	mch := &mchv1.MultiClusterHub{}
	err := client.Get(ctx, NamespacedName, mch)
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if meta.IsNoMatchError(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return mch, nil
}

func listMCH(ctx context.Context, k8sClient client.Client) (*mchv1.MultiClusterHub, error) {
	mch := &mchv1.MultiClusterHubList{}
	err := k8sClient.List(ctx, mch)
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if meta.IsNoMatchError(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if len(mch.Items) == 0 {
		return nil, err
	}

	return &mch.Items[0], nil
}

func newClusterClaim(name, value string) *clustersv1alpha1.ClusterClaim {
	return &clustersv1alpha1.ClusterClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"velero.io/exclude-from-backup":  "true",
				constants.GlobalHubOwnerLabelKey: constants.HoHAgentOwnerLabelValue,
			},
		},
		Spec: clustersv1alpha1.ClusterClaimSpec{
			Value: value,
		},
	}
}

func updateClusterClaim(ctx context.Context, k8sClient client.Client, name, value string) error {
	clusterClaim := &clustersv1alpha1.ClusterClaim{}
	err := k8sClient.Get(context.TODO(), client.ObjectKey{
		Name: name,
	}, clusterClaim)
	if errors.IsNotFound(err) {
		return k8sClient.Create(context.Background(), newClusterClaim(name, value))
	}
	if err != nil {
		return err
	}

	clusterClaim.Spec.Value = value
	return k8sClient.Update(ctx, clusterClaim)
}

func StartClusterClaimController(mgr ctrl.Manager) error {
	clusterClaimPredicate, _ := predicate.LabelSelectorPredicate(metav1.LabelSelector{
		MatchLabels: map[string]string{
			constants.GlobalHubOwnerLabelKey: constants.HoHAgentOwnerLabelValue,
		},
	})
	return ctrl.NewControllerManagedBy(mgr).
		For(&clustersv1alpha1.ClusterClaim{}, builder.WithPredicates(clusterClaimPredicate)).
		Watches(&source.Kind{Type: &mchv1.MultiClusterHub{}}, &handler.EnqueueRequestForObject{}).
		Complete(&clusterClaimController{
			client: mgr.GetClient(),
			log:    ctrl.Log.WithName("clusterclaim-controller"),
		})
}
