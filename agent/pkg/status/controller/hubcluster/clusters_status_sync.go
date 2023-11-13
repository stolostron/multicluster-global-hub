package hubcluster

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	routev1 "github.com/openshift/api/route/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/bundle"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/bundle/hubcluster"
	agentstatusconfig "github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

type hubClusterController struct {
	client             client.Client
	log                logr.Logger
	bundle             bundle.Bundle
	leafHubName        string
	transportBundleKey string
	transport          transport.Producer
}

// AddHubClusterController creates a controller and adds it to the manager.
// this controller is responsible for syncing the hub cluster status.
// right now, it only syncs the openshift console url.
func AddHubClusterController(mgr ctrl.Manager, producer transport.Producer) error {
	leafHubName := agentstatusconfig.GetLeafHubName()
	hubClusterController := &hubClusterController{
		client:             mgr.GetClient(),
		leafHubName:        leafHubName,
		transportBundleKey: fmt.Sprintf("%s.%s", leafHubName, constants.HubClusterInfoMsgKey),
		transport:          producer,
		bundle:             hubcluster.NewLeafHubClusterInfoStatusBundle(leafHubName),
		log:                ctrl.Log.WithName("hub-cluster-status-sync"),
	}

	hubClusterControllerPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return (object.GetNamespace() == constants.OpenShiftConsoleNamespace &&
			object.GetName() == constants.OpenShiftConsoleRouteName) ||
			(object.GetNamespace() == constants.ObservabilityNamespace &&
				object.GetName() == constants.ObservabilityGrafanaRouteName)
	})

	if err := ctrl.NewControllerManagedBy(mgr).
		For(&routev1.Route{}).
		WithEventFilter(hubClusterControllerPredicate).
		Complete(hubClusterController); err != nil {
		return fmt.Errorf("failed to add hub cluster controller to the manager - %w", err)
	}

	return nil
}

func (c *hubClusterController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := c.log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)

	consoleRoute := &routev1.Route{}

	if err := c.client.Get(ctx, request.NamespacedName, consoleRoute); apiErrors.IsNotFound(err) {
		consoleRoute.Name = request.Name
		consoleRoute.Namespace = request.Namespace
		c.syncBundle(ctx, consoleRoute)
		return ctrl.Result{}, nil
	} else if err != nil {
		reqLogger.Info(fmt.Sprintf("Reconciliation failed: %s", err))
		return ctrl.Result{Requeue: true, RequeueAfter: 5 * time.Second},
			fmt.Errorf("reconciliation failed: %w", err)
	}

	c.syncBundle(ctx, consoleRoute)

	reqLogger.V(2).Info("Reconciliation complete.")

	return ctrl.Result{}, nil
}

func (c *hubClusterController) syncBundle(ctx context.Context, route *routev1.Route) {
	c.bundle.UpdateObject(route)

	payloadBytes, err := json.Marshal(c.bundle)
	if err != nil {
		c.log.Error(err, "marshal hub cluster info bundle error", "transportBundleKey", c.transportBundleKey)
	}

	if err := c.transport.Send(ctx, &transport.Message{
		Key:     c.transportBundleKey,
		ID:      c.transportBundleKey,
		MsgType: constants.StatusBundle,
		Version: c.bundle.GetBundleVersion().String(),
		Payload: payloadBytes,
	}); err != nil {
		c.log.Error(err, "send hub cluster info error", "messageId", c.transportBundleKey)
	}
}
