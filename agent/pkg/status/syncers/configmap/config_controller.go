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
	ctx       context.Context // Long-lived manager context for syncer goroutines
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
// The ctx parameter should be a long-lived manager-level context, not a request-scoped reconcile context
func SetHubHASyncerManager(ctx context.Context, mgr ctrl.Manager, producer transport.Producer, startFunc HubHASyncerStartFunc) {
	hubHASyncerOnce.Do(func() {
		hubHASyncerManager = &HubHASyncerManager{
			ctx:       ctx,
			mgr:       mgr,
			producer:  producer,
			startFunc: startFunc,
		}
	})
}

// StartHubHASyncerIfActive starts the Hub HA syncer if the agent is in active role
// This should be called after SetHubHASyncerManager to handle first boot
func StartHubHASyncerIfActive() {
	if hubHASyncerManager == nil {
		return
	}

	agentConfig := configs.GetAgentConfig()
	if agentConfig == nil {
		return
	}

	hubRole, _ := agentConfig.GetHubRoleAndStandbyHub()
	if hubRole != constants.GHHubRoleActive {
		return
	}

	// Use the manager's lock to avoid race conditions
	hubHASyncerManager.mu.Lock()
	defer hubHASyncerManager.mu.Unlock()

	// Only start if not already running
	if hubHASyncerManager.cancel != nil {
		return
	}

	syncerCtx, cancel := context.WithCancel(hubHASyncerManager.ctx)
	hubHASyncerManager.cancel = cancel

	go func() {
		if err := hubHASyncerManager.startFunc(syncerCtx, hubHASyncerManager.mgr, hubHASyncerManager.producer); err != nil {
			logger.DefaultZapLogger().Errorw("Failed to start Hub HA active syncer", "error", err)
		}
	}()

	logger.DefaultZapLogger().Info("Started Hub HA active syncer on first boot")
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
		// Determine new values from configmap
		var newHubRole, newStandbyHub string
		if hubRole, found := agentConfigMap.Data[AgentHubRoleKey]; found && hubRole != "" {
			newHubRole = hubRole
			reqLogger.Infof("Updated agent hub role to: %s", hubRole)
		}
		if standbyHub, found := agentConfigMap.Data["standbyHub"]; found && standbyHub != "" {
			newStandbyHub = standbyHub
			reqLogger.Infof("Updated agent standby hub to: %s", standbyHub)
		}

		// Atomically update both fields and get previous values
		previousHubRole, previousStandbyHub := agentConfig.UpdateHubRoleAndStandbyHub(newHubRole, newStandbyHub)

		// Dynamic Hub HA syncer management: restart syncer when:
		// 1. Hub role changes to/from active
		// 2. Hub is active and standby hub target changes
		roleChanged := previousHubRole != newHubRole
		standbyChanged := previousStandbyHub != newStandbyHub
		isActiveRole := newHubRole == constants.GHHubRoleActive || previousHubRole == constants.GHHubRoleActive

		if (roleChanged || standbyChanged) && isActiveRole {
			reqLogger.Infow("Hub HA configuration changed, restarting syncer",
				"previousRole", previousHubRole,
				"newRole", newHubRole,
				"previousStandbyHub", previousStandbyHub,
				"newStandbyHub", newStandbyHub)

			// Try to restart syncer; if manager not initialized yet, requeue to retry later
			if !c.restartHubHASyncer(reqLogger) {
				reqLogger.Info("Hub HA syncer manager not ready, will retry in 5 seconds")
				return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
			}
		}
	}

	reqLogger.Debug("Reconciliation complete.")
	return ctrl.Result{}, nil
}

// restartHubHASyncer stops any existing Hub HA syncer and starts a new one if role is active
// Returns true if successful (or manager not needed), false if manager not initialized yet (caller should requeue)
func (c *hubOfHubsConfigController) restartHubHASyncer(log *zap.SugaredLogger) bool {
	if hubHASyncerManager == nil {
		log.Warn("Hub HA syncer manager not initialized, cannot restart syncer")
		return false
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
	if agentConfig != nil {
		hubRole, standbyHub := agentConfig.GetHubRoleAndStandbyHub()
		if hubRole == constants.GHHubRoleActive {
			log.Infow("Starting Hub HA active syncer",
				"hubRole", hubRole,
				"standbyHub", standbyHub)

			// Create cancellable context for the syncer from the manager-level context
			// Do NOT use the reconcile ctx parameter as it is request-scoped and will be
			// cancelled when reconciliation completes, terminating the long-lived syncer
			syncerCtx, cancel := context.WithCancel(hubHASyncerManager.ctx)
			hubHASyncerManager.cancel = cancel

			// Start the syncer using the provided start function
			go func() {
				if err := hubHASyncerManager.startFunc(syncerCtx, hubHASyncerManager.mgr, hubHASyncerManager.producer); err != nil {
					log.Errorw("Failed to start Hub HA active syncer", "error", err)
				}
			}()
		}
	}
	return true
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
