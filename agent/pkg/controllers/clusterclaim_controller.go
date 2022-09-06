// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clustersv1alpha1 "open-cluster-management.io/api/cluster/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
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

const (
	VersionACMName = "version.open-cluster-management.io"
)

func (c *clusterClaimController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := c.log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)

	version, err := getACMVersion(ctx, c.client, request.NamespacedName)
	if err != nil {
		return ctrl.Result{}, err
	}
	if version == "" {
		return ctrl.Result{}, nil
	}

	err = c.client.Get(context.TODO(), client.ObjectKey{Name: VersionACMName}, &clustersv1alpha1.ClusterClaim{})
	if errors.IsNotFound(err) {
		if err := c.client.Create(context.Background(), createClusterClaim(version)); err != nil {
			return ctrl.Result{}, err
		}
		reqLogger.Info("Reconciliation complete.")
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, err
	}

	if err := c.client.Update(ctx, createClusterClaim(version)); err != nil {
		reqLogger.Error(err, "failed to apply clusterClaim")
		return ctrl.Result{}, err
	}
	reqLogger.Info("Reconciliation complete.")
	return ctrl.Result{}, nil
}

func getACMVersion(ctx context.Context, client client.Client, namespacedname types.NamespacedName) (string, error) {
	mch := &mchv1.MultiClusterHub{}
	err := client.Get(ctx, namespacedname, mch)
	if errors.IsNotFound(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return mch.Status.CurrentVersion, nil
}

func createClusterClaim(version string) *clustersv1alpha1.ClusterClaim {
	return &clustersv1alpha1.ClusterClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: VersionACMName,
			Labels: map[string]string{
				"velero.io/exclude-from-backup":  "true",
				constants.GlobalHubOwnerLabelKey: constants.HoHAgentOwnerLabelValue,
			},
		},
		Spec: clustersv1alpha1.ClusterClaimSpec{
			Value: version,
		},
	}
}

func AddClusterClaimController(mgr ctrl.Manager) error {
	clusterClaimPredicate, _ := predicate.LabelSelectorPredicate(metav1.LabelSelector{
		MatchLabels: map[string]string{
			constants.GlobalHubOwnerLabelKey: constants.HoHAgentOwnerLabelValue,
		},
	})
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&clustersv1alpha1.ClusterClaim{}).
		Watches(&source.Kind{Type: &mchv1.MultiClusterHub{}}, &handler.EnqueueRequestForObject{}).
		WithEventFilter(clusterClaimPredicate).
		Complete(&clusterClaimController{
			client: mgr.GetClient(),
			log:    ctrl.Log.WithName("clusterclaim-controller"),
		}); err != nil {
		return fmt.Errorf("failed to add clusterclaim controller to the manager: %w", err)
	}
	return nil
}
