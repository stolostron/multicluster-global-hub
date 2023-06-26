package config

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

const (
	RequeuePeriod                     = 5 * time.Second
	DefaultStatusSyncInterval         = 5 * time.Second
	DefaultControllerInfoSyncInterval = 60 * time.Second
)

type hubOfHubsConfigController struct {
	client            client.Client
	log               logr.Logger
	configObject      *corev1.ConfigMap
	syncIntervalsData *SyncIntervals
}

// AddConfigController creates a new instance of config controller and adds it to the manager.
func AddConfigController(mgr ctrl.Manager, configObject *corev1.ConfigMap, syncIntervals *SyncIntervals) error {
	hubOfHubsConfigCtrl := &hubOfHubsConfigController{
		client:            mgr.GetClient(),
		log:               ctrl.Log.WithName("multicluster-global-hub-agent-config"),
		configObject:      configObject,
		syncIntervalsData: syncIntervals,
	}

	configMapPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return object.GetNamespace() == constants.GHSystemNamespace &&
			object.GetName() == constants.GHAgentConfigCMName
	})
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}).
		WithEventFilter(configMapPredicate).
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

	c.setSyncInterval(c.configObject, "managedClusters", &c.syncIntervalsData.managedClusters)
	c.setSyncInterval(c.configObject, "policies", &c.syncIntervalsData.policies)
	c.setSyncInterval(c.configObject, "controlInfo", &c.syncIntervalsData.controlInfo)

	reqLogger.Info("Reconciliation complete.")
	return ctrl.Result{}, nil
}

func (c *hubOfHubsConfigController) setSyncInterval(configMap *v1.ConfigMap, key string, syncInterval *time.Duration) {
	intervalStr, found := configMap.Data[key]
	if !found {
		c.log.Info(fmt.Sprintf("%s sync interval not defined, using %s", key, syncInterval.String()))
		return
	}

	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		c.log.Info(fmt.Sprintf("%s sync interval has invalid format, using %s", key, syncInterval.String()))
		return
	}

	*syncInterval = interval
}
