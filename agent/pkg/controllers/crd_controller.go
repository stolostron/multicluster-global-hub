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
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/security"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

const (
	clusterManagersCRDName = "clustermanagers.operator.open-cluster-management.io"
	stackRoxCentralCRDName = "centrals.platform.stackrox.io"
)

var crdCtrlStarted = false

type crdController struct {
	mgr         ctrl.Manager
	log         logr.Logger
	restConfig  *rest.Config
	agentConfig *config.AgentConfig
	producer    transport.Producer
	consumer    transport.Consumer
}

func (c *crdController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	switch {
	case request.Name == clusterManagersCRDName:
		return c.reconcileClusterManagers(ctx, request)
	case request.Name == stackRoxCentralCRDName && c.agentConfig.EnableStackroxIntegration:
		return c.reconcileStackRoxCentrals()
	default:
		return ctrl.Result{}, nil
	}
}

func (c *crdController) reconcileClusterManagers(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := c.log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.V(2).Info("crd controller", "NamespacedName:", request.NamespacedName)

	if err := statusController.AddControllers(ctx, c.mgr, c.producer, c.agentConfig); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to add status syncer: %w", err)
	}

	// only enable the status controller in the standalone mode
	if c.agentConfig.Standalone {
		return ctrl.Result{}, nil
	}

	// add spec controllers
	if err := specController.AddToManager(ctx, c.mgr, c.consumer, c.agentConfig); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to add spec syncer: %w", err)
	}
	reqLogger.V(2).Info("add spec controllers to manager")

	// Need this controller to update the value of clusterclaim version.open-cluster-management.io
	if err := AddVersionClusterClaimController(c.mgr); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to add controllers: %w", err)
	}

	if err := config.AddHoHLeaseUpdater(c.mgr, c.agentConfig.PodNamespace,
		"multicluster-global-hub-controller"); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to add lease updater: %w", err)
	}

	return ctrl.Result{}, nil
}

func (c *crdController) reconcileStackRoxCentrals() (result ctrl.Result, err error) {
	c.log.Info("Detected the presence of the StackRox central CRD")

	// Create the object that polls the StackRox API and publishes the message, then add it to the controller
	// manager so that it will be started automatically.
	syncer, err := security.NewStackRoxSyncer().
		SetLogger(c.log.WithName("stackrox-syncer")).
		SetTopic(c.agentConfig.TransportConfig.KafkaCredential.StatusTopic).
		SetProducer(c.producer).
		SetKubernetesClient(c.mgr.GetClient()).
		SetPollInterval(c.agentConfig.StackroxPollInterval).
		Build()
	if err != nil {
		return
	}
	err = c.mgr.Add(syncer)
	if err != nil {
		return
	}
	c.log.Info("Added StackRox syncer")

	// Create the controller that watches the StackRox instances and add it to the controller manager.
	err = security.AddStacRoxController(c.mgr, syncer)
	if err != nil {
		return
	}
	c.log.Info("Added StackRox controller")

	return
}

// this controller is used to watch the multiclusterhub crd or clustermanager crd
// if the crd exists, then add controllers to the manager dynamically
func AddCRDController(mgr ctrl.Manager, restConfig *rest.Config, agentConfig *config.AgentConfig,
	producer transport.Producer, consumer transport.Consumer,
) error {
	if crdCtrlStarted {
		return nil
	}
	if err := ctrl.NewControllerManagedBy(mgr).
		For(
			&apiextensionsv1.CustomResourceDefinition{},
			builder.OnlyMetadata,
			builder.WithPredicates(predicate.Funcs{
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
			}),
		).
		Complete(&crdController{
			mgr:         mgr,
			restConfig:  restConfig,
			agentConfig: agentConfig,
			producer:    producer,
			consumer:    consumer,
			log:         ctrl.Log.WithName("crd-controller"),
		}); err != nil {
		return err
	}
	crdCtrlStarted = true
	return nil
}
