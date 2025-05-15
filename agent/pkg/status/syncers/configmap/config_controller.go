package configmap

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

const (
	RequeuePeriod = 5 * time.Second
)

type hubOfHubsConfigController struct {
	client client.Client
	log    *zap.SugaredLogger
}

// AddConfigMapController creates a new instance of config controller and adds it to the manager.
func AddConfigMapController(mgr ctrl.Manager, agentConfig *configs.AgentConfig) error {
	hubOfHubsConfigCtrl := &hubOfHubsConfigController{
		client: mgr.GetClient(),
		log:    logger.DefaultZapLogger(),
	}

	configMapPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return object.GetNamespace() == agentConfig.PodNamespace && object.GetName() == constants.GHAgentConfigCMName
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
	reqLogger := c.log.With("Request.Namespace", request.Namespace, "Request.Name", request.Name)

	agentConfigMap := &corev1.ConfigMap{}
	if err := c.client.Get(ctx, request.NamespacedName, agentConfigMap); apierrors.IsNotFound(err) {
		return ctrl.Result{}, nil
	} else if err != nil {
		return ctrl.Result{Requeue: true, RequeueAfter: RequeuePeriod},
			fmt.Errorf("reconciliation failed: %w", err)
	}

	c.setSyncInterval(agentConfigMap, ManagedClusterIntervalKey)
	c.setSyncInterval(agentConfigMap, PolicyIntervalKey)
	c.setSyncInterval(agentConfigMap, HubClusterInfoIntervalKey)
	c.setSyncInterval(agentConfigMap, HubClusterHeartBeatIntervalKey)
	c.setSyncInterval(agentConfigMap, EventIntervalKey)

	c.setAgentConfig(agentConfigMap, AgentAggregationKey)
	c.setAgentConfig(agentConfigMap, EnableLocalPolicyKey)

	logLevel := agentConfigMap.Data[string(AgentLogLevelKey)]
	if logLevel != "" {
		logger.SetLogLevel(logger.LogLevel(logLevel))
	}

	reqLogger.Debug("Reconciliation complete.")
	return ctrl.Result{}, nil
}

func (c *hubOfHubsConfigController) setSyncInterval(configMap *v1.ConfigMap, key AgentConfigKey) {
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

func (c *hubOfHubsConfigController) setAgentConfig(configMap *v1.ConfigMap, configKey AgentConfigKey) {
	val, found := configMap.Data[string(configKey)]
	if !found {
		c.log.Info(fmt.Sprintf("%s not defined in agentConfig, using default value", configKey))
		return
	}
	agentConfigs[configKey] = AgentConfigValue(val)
}
