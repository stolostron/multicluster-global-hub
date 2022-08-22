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
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

type clusterRoleBindingController struct {
	client client.Client
	log    logr.Logger
}

func (c *clusterRoleBindingController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := c.log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)

	err := c.client.Get(context.TODO(), client.ObjectKey{Name: HubOfHubsClusterRoleName}, &rbacv1.ClusterRoleBinding{})
	if errors.IsNotFound(err) {
		if err := c.client.Create(context.Background(), createClusterRoleBinding()); err != nil {
			return ctrl.Result{}, err
		}
		reqLogger.Info("Reconciliation complete.")
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, err
	}

	if err := c.client.Update(ctx, createClusterRoleBinding()); err != nil {
		reqLogger.Error(err, "failed to apply clusterRoleBinding")
		return ctrl.Result{}, err
	}
	reqLogger.Info("Reconciliation complete.")
	return ctrl.Result{}, nil
}

func createClusterRoleBinding() *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: HubOfHubsClusterRoleName,
			Labels: map[string]string{
				constants.GlobalHubOwnerLabelKey: constants.HoHAgentOwnerLabelValue,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			Name:     HubOfHubsClusterRoleName,
			APIGroup: "rbac.authorization.k8s.io",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "klusterlet-work-sa",
				Namespace: "open-cluster-management-agent",
			},
		},
	}
}

func AddClusterRoleBindingController(mgr ctrl.Manager) error {
	clusterRoleBindingPredicate, _ := predicate.LabelSelectorPredicate(metav1.LabelSelector{
		MatchLabels: map[string]string{
			constants.GlobalHubOwnerLabelKey: constants.HoHAgentOwnerLabelValue,
		},
	})
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&rbacv1.ClusterRoleBinding{}).
		WithEventFilter(clusterRoleBindingPredicate).
		Complete(&clusterRoleBindingController{
			client: mgr.GetClient(),
			log:    ctrl.Log.WithName("clusterrolebinding-controller"),
		}); err != nil {
		return fmt.Errorf("failed to add clusterrolebinding controller to the manager: %w", err)
	}

	return nil
}

func InitClusterRoleBinding(ctx context.Context, kubeClient *kubernetes.Clientset) error {
	_, err := kubeClient.RbacV1().ClusterRoleBindings().Get(
		ctx, HubOfHubsClusterRoleName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		if _, err := kubeClient.RbacV1().ClusterRoleBindings().Create(
			ctx, createClusterRoleBinding(), metav1.CreateOptions{}); err != nil {
			return err
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get clusterrole: %w", err)
	}
	return nil
}
