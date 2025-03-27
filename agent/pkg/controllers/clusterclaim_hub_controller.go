// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controllers

import (
	"context"
	"fmt"

	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clustersv1alpha1 "open-cluster-management.io/api/cluster/v1alpha1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var clusterClaimCtrlStared = false

type hubClusterClaimController struct {
	client client.Client
}

func (c *hubClusterClaimController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	log.Debug("NamespacedName: ", request.NamespacedName)

	_, err := updateHubClusterClaim(ctx, c.client, request.NamespacedName)
	return ctrl.Result{}, err
}

func AddHubClusterClaimController(mgr ctrl.Manager) error {
	// the controller is only to trigger create hub clusterClaim at the beginning
	// do nothing if the hub clusterClaim existed
	if clusterClaimCtrlStared {
		return nil
	}
	clusterClaimPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		if object.GetName() == constants.HubClusterClaimName {
			return false
		}
		clusterClaim, _ := utils.GetClusterClaim(context.Background(), mgr.GetClient(), constants.HubClusterClaimName)
		return clusterClaim == nil
	})
	err := ctrl.NewControllerManagedBy(mgr).Named("hubclusterclaim-controller").
		For(&clustersv1alpha1.ClusterClaim{}, builder.WithPredicates(clusterClaimPredicate)).
		Complete(&hubClusterClaimController{
			client: mgr.GetClient(),
		})
	if err != nil {
		return err
	}
	clusterClaimCtrlStared = true
	return nil
}

func updateHubClusterClaim(ctx context.Context, k8sClient client.Client,
	namespacedName types.NamespacedName,
) (*mchv1.MultiClusterHub, error) {
	mch, err := getMCH(ctx, k8sClient, namespacedName)
	if err != nil {
		return nil, fmt.Errorf("failed to get MCH instance. err = %v", err)
	}

	hubValue := constants.HubNotInstalled
	if mch == nil {
		clusterManager, err := getClusterManager(ctx, k8sClient)
		if err != nil {
			return nil, fmt.Errorf("failed to get clusterManager instance. err = %v", err)
		}

		if clusterManager != nil {
			hubValue = constants.HubInstalledByUser
		}
		return nil, updateClusterClaim(ctx, k8sClient, constants.HubClusterClaimName, hubValue)
	}

	hubValue = constants.HubInstalledByUser
	if mch.GetLabels()[constants.GlobalHubOwnerLabelKey] == constants.GlobalHubOwnerLabelVal {
		hubValue = constants.HubInstalledByGlobalHub
	}
	return mch, updateClusterClaim(ctx, k8sClient, constants.HubClusterClaimName, hubValue)
}

func updateClusterClaim(ctx context.Context, k8sClient client.Client, name, value string) error {
	clusterClaim, err := utils.GetClusterClaim(ctx, k8sClient, name)
	if err != nil {
		return err
	}
	if clusterClaim == nil {
		return k8sClient.Create(context.Background(), newClusterClaim(name, value))
	}
	clusterClaim.Spec.Value = value
	return k8sClient.Update(ctx, clusterClaim)
}

func getClusterManager(ctx context.Context, client client.Client) (*operatorv1.ClusterManager, error) {
	clusterManager := &operatorv1.ClusterManager{}
	namespacedName := types.NamespacedName{Name: "cluster-manager"}
	err := client.Get(ctx, namespacedName, clusterManager)
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if meta.IsNoMatchError(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return clusterManager, nil
}

func getMCH(ctx context.Context, client client.Client,
	NamespacedName types.NamespacedName,
) (*mchv1.MultiClusterHub, error) {
	if NamespacedName.Name == "" || NamespacedName.Namespace == "" {
		return utils.ListMCH(ctx, client)
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

func newClusterClaim(name, value string) *clustersv1alpha1.ClusterClaim {
	return &clustersv1alpha1.ClusterClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"velero.io/exclude-from-backup":  "true",
				constants.GlobalHubOwnerLabelKey: constants.GHAgentOwnerLabelValue,
			},
		},
		Spec: clustersv1alpha1.ClusterClaimSpec{
			Value: value,
		},
	}
}
