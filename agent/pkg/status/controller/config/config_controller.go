package config

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/hub-of-hubs/agent/pkg/helper"
	configv1 "github.com/stolostron/hub-of-hubs/pkg/apis/config/v1"
	"github.com/stolostron/hub-of-hubs/pkg/constants"
)

const (
	configLogName = "hub-of-hubs-config"
	RequeuePeriod = 5 * time.Second
)

type hubOfHubsConfigController struct {
	client       client.Client
	log          logr.Logger
	configObject *configv1.Config
}

// AddConfigController creates a new instance of config controller and adds it to the manager.
func AddConfigController(mgr ctrl.Manager, configObject *configv1.Config) error {
	hubOfHubsConfigCtrl := &hubOfHubsConfigController{
		client:       mgr.GetClient(),
		log:          ctrl.Log.WithName(configLogName),
		configObject: configObject,
	}

	hohNamespacePredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return object.GetNamespace() == constants.HohSystemNamespace
	})
	ownerRefAnnotationPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return helper.HasAnnotation(object, constants.OriginOwnerReferenceAnnotation)
	})

	if err := ctrl.NewControllerManagedBy(mgr).
		For(&configv1.Config{}).
		WithEventFilter(predicate.And(hohNamespacePredicate, ownerRefAnnotationPredicate)).
		Complete(hubOfHubsConfigCtrl); err != nil {
		return fmt.Errorf("failed to add hub of hubs config controller to the manager - %w", err)
	}

	return nil
}

func (c *hubOfHubsConfigController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := c.log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)

	if err := c.client.Get(ctx, request.NamespacedName, c.configObject); apierrors.IsNotFound(err) {
		return ctrl.Result{}, nil
	} else if err != nil {
		reqLogger.Info(fmt.Sprintf("Reconciliation failed: %s", err))
		return ctrl.Result{Requeue: true, RequeueAfter: RequeuePeriod},
			fmt.Errorf("reconciliation failed: %w", err)
	}

	reqLogger.Info("Reconciliation complete.")

	return ctrl.Result{}, nil
}
