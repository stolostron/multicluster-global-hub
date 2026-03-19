package configmap

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

const (
	RequeuePeriod = 5 * time.Second
)

var (
	// Hub HA syncer management
	hubHASyncerManager *HubHASyncerManager
	hubHASyncerOnce    sync.Once
)

// HubHASyncerStartFunc is a function type to start the Hub HA active syncer
type HubHASyncerStartFunc func(context.Context, ctrl.Manager, transport.Producer) error

// HubHASyncerManager manages the Hub HA active syncer lifecycle
type HubHASyncerManager struct {
	mgr       ctrl.Manager
	producer  transport.Producer
	startFunc HubHASyncerStartFunc
	cancel    context.CancelFunc
	mu        sync.Mutex
}

type hubOfHubsConfigController struct {
	client client.Client
	log    *zap.SugaredLogger
}

// SetHubHASyncerManager initializes the Hub HA syncer manager for dynamic syncer lifecycle management
func SetHubHASyncerManager(mgr ctrl.Manager, producer transport.Producer, startFunc HubHASyncerStartFunc) {
	hubHASyncerOnce.Do(func() {
		hubHASyncerManager = &HubHASyncerManager{
			mgr:       mgr,
			producer:  producer,
			startFunc: startFunc,
		}
	})
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
	c.setAgentConfig(agentConfigMap, AgentHubRoleKey)

	logLevel := agentConfigMap.Data[string(AgentLogLevelKey)]
	if logLevel != "" {
		logger.SetLogLevel(logger.LogLevel(logLevel))
	}

	// Update AgentConfig with hubRole and standbyHub
	agentConfig := configs.GetAgentConfig()
	if agentConfig != nil {
		previousHubRole := agentConfig.HubRole

		if hubRole, found := agentConfigMap.Data[AgentHubRoleKey]; found && hubRole != "" {
			agentConfig.HubRole = hubRole
			reqLogger.Infof("Updated agent hub role to: %s", hubRole)
		} else {
			agentConfig.HubRole = ""
		}

		if standbyHub, found := agentConfigMap.Data["standbyHub"]; found && standbyHub != "" {
			agentConfig.StandbyHub = standbyHub
			reqLogger.Infof("Updated agent standby hub to: %s", standbyHub)
		} else {
			agentConfig.StandbyHub = ""
		}

		// Dynamic Hub HA syncer management: restart syncer when role changes to/from active
		if previousHubRole != agentConfig.HubRole {
			reqLogger.Infow("Hub role changed, restarting Hub HA syncer",
				"previousRole", previousHubRole,
				"newRole", agentConfig.HubRole)
			c.restartHubHASyncer(ctx, reqLogger)
		}
	}

	reqLogger.Debug("Reconciliation complete.")
	return ctrl.Result{}, nil
}

// restartHubHASyncer stops any existing Hub HA syncer and starts a new one if role is active
func (c *hubOfHubsConfigController) restartHubHASyncer(ctx context.Context, log *zap.SugaredLogger) {
	if hubHASyncerManager == nil {
		log.Warn("Hub HA syncer manager not initialized, cannot restart syncer")
		return
	}

	hubHASyncerManager.mu.Lock()
	defer hubHASyncerManager.mu.Unlock()

	// Stop existing syncer if running
	if hubHASyncerManager.cancel != nil {
		log.Info("Stopping existing Hub HA syncer")
		hubHASyncerManager.cancel()
		hubHASyncerManager.cancel = nil
	}

	// Start new syncer if role is active
	agentConfig := configs.GetAgentConfig()
	if agentConfig != nil && agentConfig.HubRole == constants.GHHubRoleActive {
		log.Infow("Starting Hub HA active syncer",
			"hubRole", agentConfig.HubRole,
			"standbyHub", agentConfig.StandbyHub)

		// Create cancellable context for the syncer
		syncerCtx, cancel := context.WithCancel(ctx)
		hubHASyncerManager.cancel = cancel

		// Start the syncer using the provided start function
		go func() {
			if err := hubHASyncerManager.startFunc(syncerCtx, hubHASyncerManager.mgr, hubHASyncerManager.producer); err != nil {
				log.Errorw("Failed to start Hub HA active syncer", "error", err)
			}
		}()
	}
}

func (c *hubOfHubsConfigController) setSyncInterval(configMap *corev1.ConfigMap, key string) {
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

func (c *hubOfHubsConfigController) setAgentConfig(configMap *corev1.ConfigMap, configKey string) {
	val, found := configMap.Data[string(configKey)]
	if !found {
		c.log.Info(fmt.Sprintf("%s not defined in agentConfig, using default value", configKey))
		return
	}
	agentConfigs[configKey] = AgentConfigValue(val)
}
