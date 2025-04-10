// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controllers

import (
	"context"
	"fmt"
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/controllers/inventory"
	agentspec "github.com/stolostron/multicluster-global-hub/agent/pkg/spec"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/syncers/security"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const (
	clusterManagersCRDName = "clustermanagers.operator.open-cluster-management.io"
	stackRoxCentralCRDName = "centrals.platform.stackrox.io"
)

var (
	initCtrlStarted = false
	log             = logger.DefaultZapLogger()
)

type initController struct {
	mgr             ctrl.Manager
	restConfig      *rest.Config
	agentConfig     *configs.AgentConfig
	transportClient transport.TransportClient
}

func (c *initController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	switch {
	case request.Name == clusterManagersCRDName:
		return c.addACMController(ctx, request)
	case request.Name == stackRoxCentralCRDName && c.agentConfig.EnableStackroxIntegration:
		return c.addStackRoxCentrals()
	default:
		return ctrl.Result{}, nil
	}
}

func (c *initController) addACMController(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	log.Info("NamespacedName: ", request.NamespacedName)
	// status syncers or inventory
	var err error
	mch, err := utils.ListMCH(ctx, c.mgr.GetClient())
	if err != nil {
		return ctrl.Result{}, err
	}
	if mch != nil {
		configs.SetMCHVersion(mch.Status.CurrentVersion)
	}

	switch c.agentConfig.TransportConfig.TransportType {
	case string(transport.Kafka):
		err = status.AddToManager(ctx, c.mgr, c.transportClient, c.agentConfig)
	case string(transport.Rest):
		err = inventory.AddToManager(ctx, c.mgr, c.transportClient, c.agentConfig)
	}
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to add the syncer: %w", err)
	}

	// add spec controllers
	if err := agentspec.AddToManager(ctx, c.mgr, c.transportClient, c.agentConfig); err != nil {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, fmt.Errorf("failed to add spec syncer: %w", err)
	}

	// only enable the status controller in the standalone mode
	if c.agentConfig.Standalone {
		return ctrl.Result{}, nil
	}

	// Need this controller to update the value of clusterclaim hub.open-cluster-management.io
	// we use the value to decide whether install the ACM or not
	if err := AddHubClusterClaimController(c.mgr); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to add hub.open-cluster-management.io clusterclaim controller: %w", err)
	}
	// Need this controller to update the value of clusterclaim version.open-cluster-management.io
	if err := AddVersionClusterClaimController(c.mgr); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to add controllers: %w", err)
	}

	// all the controller started, then add the lease controller
	if err := AddLeaseController(c.mgr, c.agentConfig.PodNamespace, "multicluster-global-hub-agent"); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to add lease updater: %w", err)
	}

	return ctrl.Result{}, nil
}

func (c *initController) addStackRoxCentrals() (result ctrl.Result, err error) {
	log.Info("Detected the presence of the StackRox central CRD")

	// Create the object that polls the StackRox API and publishes the message, then add it to the controller
	// manager so that it will be started automatically.
	syncer, err := security.NewStackRoxSyncer().
		SetLogger(logger.ZapLogger("stackrox-syncer")).
		SetTopic(c.agentConfig.TransportConfig.KafkaCredential.StatusTopic).
		SetProducer(c.transportClient.GetProducer()).
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
	log.Info("Added StackRox syncer")

	// Create the controller that watches the StackRox instances and add it to the controller manager.
	err = security.AddStacRoxController(c.mgr, syncer)
	if err != nil {
		return
	}
	log.Info("Added StackRox controller")

	return
}

// this controller is used to watch the multiclusterhub crd or clustermanager crd
// if the crd exists, then add controllers to the manager dynamically
func AddInitController(mgr ctrl.Manager, restConfig *rest.Config, agentConfig *configs.AgentConfig,
	transportClient transport.TransportClient,
) error {
	if initCtrlStarted {
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
		Complete(&initController{
			mgr:             mgr,
			restConfig:      restConfig,
			agentConfig:     agentConfig,
			transportClient: transportClient,
		}); err != nil {
		return err
	}
	initCtrlStarted = true
	return nil
}
