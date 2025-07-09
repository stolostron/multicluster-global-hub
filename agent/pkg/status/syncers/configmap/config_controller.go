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
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
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

	// Set the cluster interval by the configmap
	// sync interval - managedcluster: 5s,
	// resync interval - resync.managedcluster: 6h
	c.setSyncInterval(agentConfigMap, GetResyncKey(enum.ManagedClusterType))
	c.setSyncInterval(agentConfigMap, GetSyncKey(enum.ManagedClusterType))

	c.setSyncInterval(agentConfigMap, GetResyncKey(enum.LocalPolicySpecType))
	c.setSyncInterval(agentConfigMap, GetSyncKey(enum.LocalPolicySpecType))

	c.setSyncInterval(agentConfigMap, GetResyncKey(enum.HubClusterInfoType))
	c.setSyncInterval(agentConfigMap, GetSyncKey(enum.HubClusterInfoType))

	c.setSyncInterval(agentConfigMap, GetResyncKey(enum.HubClusterHeartbeatType))
	c.setSyncInterval(agentConfigMap, GetSyncKey(enum.HubClusterHeartbeatType))

	c.setSyncInterval(agentConfigMap, GetResyncKey(enum.ManagedClusterEventType))
	c.setSyncInterval(agentConfigMap, GetSyncKey(enum.ManagedClusterEventType))

	// Set the agent configs
	c.setAgentConfig(agentConfigMap, AgentAggregationKey)
	c.setAgentConfig(agentConfigMap, EnableLocalPolicyKey)

	logLevel := agentConfigMap.Data[string(AgentLogLevelKey)]
	if logLevel != "" {
		logger.SetLogLevel(logger.LogLevel(logLevel))
	}

	reqLogger.Debug("Reconciliation complete.")
	return ctrl.Result{}, nil
}

func (c *hubOfHubsConfigController) setSyncInterval(configMap *v1.ConfigMap, key string) {
	intervalStr, found := configMap.Data[string(key)]
	if !found {
		c.log.Infof("%s sync interval not defined in configmap, using default value", key)
		return
	}

	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		c.log.Errorf("failed to parse %s sync interval: %v", key, err)
		return
	}
	c.log.Infof("setting %s interval to %s", key, interval.String())
	SetInterval(key, interval)
}

func (c *hubOfHubsConfigController) setAgentConfig(configMap *v1.ConfigMap, configKey string) {
	val, found := configMap.Data[string(configKey)]
	if !found {
		c.log.Info(fmt.Sprintf("%s not defined in agentConfig, using default value", configKey))
		return
	}
	agentConfigs[configKey] = AgentConfigValue(val)
}
