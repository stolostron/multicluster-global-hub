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

	"github.com/stolostron/multicluster-global-hub/agent/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

const (
	RequeuePeriod = 5 * time.Second
)

type hubOfHubsConfigController struct {
	client client.Client
	log    logr.Logger
}

// AddConfigController creates a new instance of config controller and adds it to the manager.
func AddConfigController(mgr ctrl.Manager, agentConfig *config.AgentConfig) error {
	hubOfHubsConfigCtrl := &hubOfHubsConfigController{
		client: mgr.GetClient(),
		log:    ctrl.Log.WithName("multicluster-global-hub-agent-config"),
	}
	leafHubName = agentConfig.LeafHubName

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

	if err := c.client.Get(ctx, request.NamespacedName, agentConfigMap); apierrors.IsNotFound(err) {
		return ctrl.Result{}, nil
	} else if err != nil {
		return ctrl.Result{Requeue: true, RequeueAfter: RequeuePeriod},
			fmt.Errorf("reconciliation failed: %w", err)
	}

	c.setSyncInterval(agentConfigMap, ManagedClusterIntervalKey)
	c.setSyncInterval(agentConfigMap, PolicyIntervalKey)
	c.setSyncInterval(agentConfigMap, ControlInfoIntervalKey)

	reqLogger.Info("Reconciliation complete.")
	return ctrl.Result{}, nil
}

func (c *hubOfHubsConfigController) setSyncInterval(configMap *v1.ConfigMap, key IntervalKey) {
	intervalStr, found := configMap.Data[string(key)]
	if !found {
		c.log.Info(fmt.Sprintf("%s sync interval not defined, using %s", key, syncIntervals[key].String()))
		return
	}

	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		c.log.Info(fmt.Sprintf("%s sync interval has invalid format, using %s", key, syncIntervals[key].String()))
		return
	}
	syncIntervals[key] = interval
}
