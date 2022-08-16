// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clustersv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type clusterRoleController struct {
	client client.Client
	log    logr.Logger
}

const (
	HubOfHubsCreateByKey     = "hub-of-hubs.open-cluster-management.io/create-by"
	HubOfHubsCreateByValue   = "multicluster-globalhub-agent"
	HubOfHubsClusterRoleName = "open-cluster-management:multicluster-globalhub-managedcluster-creation"
)

func (c *clusterRoleController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := c.log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)

	err := c.client.Get(context.TODO(), client.ObjectKey{Name: HubOfHubsClusterRoleName}, &rbacv1.ClusterRole{})
	if errors.IsNotFound(err) {
		if err := c.client.Create(context.Background(), createClusterRole()); err != nil {
			return ctrl.Result{}, err
		}
		reqLogger.Info("Reconciliation complete.")
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, err
	}

	if err := c.client.Update(ctx, createClusterRole()); err != nil {
		reqLogger.Error(err, "failed to apply clusterRole")
		return ctrl.Result{}, err
	}
	reqLogger.Info("Reconciliation complete.")
	return ctrl.Result{}, nil
}

func createClusterRole() *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: HubOfHubsClusterRoleName,
			Labels: map[string]string{
				HubOfHubsCreateByKey: HubOfHubsCreateByValue,
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				Resources: []string{
					"managedclusters",
				},
				Verbs: []string{
					"create",
					"update",
				},
				APIGroups: []string{
					clustersv1.GroupVersion.Group,
				},
			},
			{
				Resources: []string{
					"managedclustersets/join",
					"managedclustersets/bind",
				},
				Verbs: []string{
					"create",
					"delete",
				},
				APIGroups: []string{
					clustersv1.GroupVersion.Group,
				},
			},
			{
				Resources: []string{
					"managedclusters/accept",
				},
				Verbs: []string{
					"update",
				},
				APIGroups: []string{
					"register.open-cluster-management.io",
				},
			},
		},
	}
}

func AddClusterRoleController(mgr ctrl.Manager) error {
	clusterRolePredicate, _ := predicate.LabelSelectorPredicate(metav1.LabelSelector{
		MatchLabels: map[string]string{
			HubOfHubsCreateByKey: HubOfHubsCreateByValue,
		},
	})
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&rbacv1.ClusterRole{}).
		WithEventFilter(clusterRolePredicate).
		Complete(&clusterRoleController{
			client: mgr.GetClient(),
			log:    ctrl.Log.WithName("clusterrole-controller"),
		}); err != nil {
		return fmt.Errorf("failed to add clusterrole controller to the manager: %w", err)
	}
	return nil
}

func InitClusterRole(mgr ctrl.Manager) error {
	err := mgr.GetClient().Get(context.TODO(), client.ObjectKey{Name: HubOfHubsClusterRoleName}, &rbacv1.ClusterRole{})
	if errors.IsNotFound(err) {
		if err := mgr.GetClient().Create(context.Background(), createClusterRole()); err != nil {
			return err
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get clusterrole: %w", err)
	}
	return nil
}
