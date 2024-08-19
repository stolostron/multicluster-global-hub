// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controllers

import (
	"context"
	"reflect"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// certController is used to watch if the kafka cert(constants.KafkaCertSecretName) changed,
// if changed, restart agent pod
type certController struct {
	kubeClient kubernetes.Interface
	log        logr.Logger
}

// Restart the agent pod when secret data changed
func (c *certController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := c.log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.V(2).Info("cert controller", "NamespacedName:", request.NamespacedName)
	err := utils.RestartPod(ctx, c.kubeClient, constants.GHAgentNamespace, constants.AgentDeploymentName)
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func AddCertController(mgr ctrl.Manager, kubeClient kubernetes.Interface) error {
	return ctrl.NewControllerManagedBy(mgr).Named("cert-controller").
		For(&corev1.Secret{}, builder.WithPredicates(predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return false
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				if e.ObjectNew.GetName() != constants.KafkaCertSecretName {
					return false
				}
				newSecret := e.ObjectNew.(*corev1.Secret)
				oldSecret := e.ObjectOld.(*corev1.Secret)
				// only enqueue the obj when secret data changed
				return !reflect.DeepEqual(newSecret.Data, oldSecret.Data)
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return true
			},
		})).
		Complete(&certController{
			kubeClient: kubeClient,
			log:        ctrl.Log.WithName("cert-controller"),
		})
}
