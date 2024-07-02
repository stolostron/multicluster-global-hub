// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/config"
	specController "github.com/stolostron/multicluster-global-hub/agent/pkg/spec/controller"
	statusController "github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller"
)

type crdController struct {
	mgr         ctrl.Manager
	log         logr.Logger
	restConfig  *rest.Config
	agentConfig *config.AgentConfig
}

func (c *crdController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := c.log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.V(2).Info("crd controller", "NamespacedName:", request.NamespacedName)

	if err := statusController.AddControllers(ctx, c.mgr, c.agentConfig); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to add status syncer: %w", err)
	}

	// only enable the status syncer with the standalone mode
	if c.agentConfig.Standalone {
		return ctrl.Result{}, nil
	}

	// add spec controllers
	if err := specController.AddToManager(c.mgr, c.agentConfig); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to add spec syncer: %w", err)
	}

	// Need this controller to update the value of clusterclaim version.open-cluster-management.io
	if err := AddVersionClusterClaimController(c.mgr); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to add controllers: %w", err)
	}

	if err := config.AddHoHLeaseUpdater(c.mgr, c.agentConfig.PodNameSpace,
		"multicluster-global-hub-controller"); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to add lease updater: %w", err)
	}

	return ctrl.Result{}, nil
}

// this controller is used to watch the multiclusterhub crd or clustermanager crd
// if the crd exists, then add controllers to the manager dynamically
func AddCRDController(mgr ctrl.Manager, restConfig *rest.Config, agentConfig *config.AgentConfig) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&apiextensionsv1.CustomResourceDefinition{}, builder.WithPredicates(predicate.Funcs{
			// trigger the reconciler only if the crd is created
			CreateFunc: func(e event.CreateEvent) bool {
				return true
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				return false
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return false
			},
			GenericFunc: func(e event.GenericEvent) bool {
				return false
			},
		})).
		Complete(&crdController{
			mgr:         mgr,
			restConfig:  restConfig,
			agentConfig: agentConfig,
			log:         ctrl.Log.WithName("crd-controller"),
		})
}
