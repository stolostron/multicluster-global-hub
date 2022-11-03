package syncintervals

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

const (
	STATUS_SYNC_INTERVAL_CONFIGMAP_NAME = "sync-intervals"
	STATUS_SYNC_INTERVAL_LOG_NAME       = "sync-intervals"
	// RequeuePeriod is the time to wait until reconciliation retry in failure cases.
	REQUEUE_PERIOD = 5 * time.Second
)

type syncIntervalsController struct {
	client            client.Client
	log               logr.Logger
	syncIntervalsData *SyncIntervals
}

// AddSyncIntervalsController creates a new instance of config map controller and adds it to the manager.
func AddSyncIntervalsController(mgr ctrl.Manager, syncIntervals *SyncIntervals) error {
	syncIntervalsCtrl := &syncIntervalsController{
		client:            mgr.GetClient(),
		log:               ctrl.Log.WithName(STATUS_SYNC_INTERVAL_LOG_NAME),
		syncIntervalsData: syncIntervals,
	}

	syncIntervalsPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return object.GetNamespace() == constants.GHSystemNamespace &&
			object.GetName() == STATUS_SYNC_INTERVAL_CONFIGMAP_NAME
	})

	if err := ctrl.NewControllerManagedBy(mgr).
		For(&v1.ConfigMap{}).
		WithEventFilter(syncIntervalsPredicate).
		Complete(syncIntervalsCtrl); err != nil {
		return fmt.Errorf("failed to add sync intervals controller to the manager - %w", err)
	}

	return nil
}

func (c *syncIntervalsController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := c.log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)

	configMap := &v1.ConfigMap{}

	if err := c.client.Get(ctx, request.NamespacedName, configMap); apiErrors.IsNotFound(err) {
		return ctrl.Result{}, nil
	} else if err != nil {
		reqLogger.Info(fmt.Sprintf("Reconciliation failed: %s", err))
		return ctrl.Result{Requeue: true, RequeueAfter: REQUEUE_PERIOD},
			fmt.Errorf("reconciliation failed: %w", err)
	}

	c.setSyncInterval(configMap, "managed_clusters", &c.syncIntervalsData.managedClusters)
	c.setSyncInterval(configMap, "policies", &c.syncIntervalsData.policies)
	c.setSyncInterval(configMap, "control_info", &c.syncIntervalsData.controlInfo)

	reqLogger.Info("Reconciliation complete.")

	return ctrl.Result{}, nil
}

func (c *syncIntervalsController) setSyncInterval(configMap *v1.ConfigMap, key string, syncInterval *time.Duration) {
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
