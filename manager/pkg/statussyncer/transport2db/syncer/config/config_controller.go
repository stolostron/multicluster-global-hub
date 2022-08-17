package config

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

const (
	requeuePeriodSeconds = 5
)

// AddConfigController creates a new instance of config controller and adds it to the manager.
func AddConfigController(mgr ctrl.Manager, log logr.Logger, config *corev1.ConfigMap) error {
	if err := mgr.GetAPIReader().Get(context.Background(), client.ObjectKey{
		Namespace: constants.HohSystemNamespace,
		Name:      constants.HoHConfigName,
	}, config); err != nil {
		return fmt.Errorf("failed to read config - %w", err)
	}

	hubOfHubsConfigCtrl := &hubOfHubsConfigController{
		client: mgr.GetClient(),
		log:    log,
		config: config,
	}

	configPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return object.GetNamespace() == constants.HohSystemNamespace &&
			object.GetName() == constants.HoHConfigName
	})

	if err := ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}).
		WithEventFilter(configPredicate).
		Complete(hubOfHubsConfigCtrl); err != nil {
		return fmt.Errorf("failed to add config controller to manager - %w", err)
	}

	return nil
}

type hubOfHubsConfigController struct {
	client client.Client
	log    logr.Logger
	config *corev1.ConfigMap
}

func (c *hubOfHubsConfigController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := c.log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)

	if err := c.client.Get(ctx, request.NamespacedName, c.config); apierrors.IsNotFound(err) {
		return ctrl.Result{}, nil
	} else if err != nil {
		reqLogger.Info(fmt.Sprintf("Reconciliation failed: %s", err))
		return ctrl.Result{Requeue: true, RequeueAfter: requeuePeriodSeconds * time.Second},
			fmt.Errorf("reconciliation failed: %w", err)
	}

	reqLogger.Info("Reconciliation complete.")

	return ctrl.Result{}, nil
}
