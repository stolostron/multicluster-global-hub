// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

// Package hubha provides the Hub HA active-syncer lifecycle controller.
// It watches the agent ConfigMap and enables/disables the HubHAEmitter (and its
// PeriodicSyncer registration) based on the hub role and standby hub configuration.
package hubha

import (
	"context"
	"fmt"
	"sync"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	specHubHA "github.com/stolostron/multicluster-global-hub/agent/pkg/spec/hubha"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var log = logger.DefaultZapLogger()

// hubHALifecycleController manages the lifecycle of the Hub HA active syncer based on
// changes to the agent ConfigMap.  It is the single source of truth for
// HubHAEmitter.SetEnabled calls and PeriodicSyncer registration/unregistration.
type hubHALifecycleController struct {
	mgr            ctrl.Manager
	client         client.Client
	agentConfig    *configs.AgentConfig
	periodicSyncer *generic.PeriodicSyncer
	producer       transport.Producer

	// startResourceSyncerFn is the function that starts the GVK-watching controller.
	// It defaults to the real implementation but can be replaced in tests to avoid
	// requiring a live ctrl.Manager.
	startResourceSyncerFn func(emitter *specHubHA.HubHAEmitter) ([]schema.GroupVersionKind, error)

	// mu protects emitter, registered, and resourceSyncerStarted.
	mu                    sync.Mutex
	emitter               *specHubHA.HubHAEmitter
	registered            bool
	resourceSyncerStarted bool // set to true only after startFn succeeds, allowing retries on failure
}

// HubHAControllerOption configures a hubHALifecycleController.
type HubHAControllerOption func(*hubHALifecycleController)

// WithNoOpStartResourceSyncerFn sets a no-op startResourceSyncerFn so the lifecycle
// controller does not attempt to register the "hubha" GVK-watching controller.
// Use this in integration-test suites that register the resource controller directly
// (via StartHubHAResourceSyncer) to avoid "controller already exists" conflicts.
func WithNoOpStartResourceSyncerFn() HubHAControllerOption {
	return func(c *hubHALifecycleController) {
		c.startResourceSyncerFn = func(_ *specHubHA.HubHAEmitter) ([]schema.GroupVersionKind, error) {
			return nil, nil
		}
	}
}

// AddHubHAController registers the Hub HA lifecycle controller with the manager.
// It watches the same agent ConfigMap as the existing configmap controller but is
// solely responsible for the Hub HA syncer lifecycle (replacing HubHASyncerManager).
func AddHubHAController(
	mgr ctrl.Manager,
	periodicSyncer *generic.PeriodicSyncer,
	producer transport.Producer,
	agentConfig *configs.AgentConfig,
	opts ...HubHAControllerOption,
) error {
	c := &hubHALifecycleController{
		mgr:            mgr,
		client:         mgr.GetClient(),
		agentConfig:    agentConfig,
		periodicSyncer: periodicSyncer,
		producer:       producer,
	}
	for _, opt := range opts {
		opt(c)
	}

	configMapPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return object.GetNamespace() == agentConfig.PodNamespace &&
			object.GetName() == constants.GHAgentConfigCMName
	})

	if err := ctrl.NewControllerManagedBy(mgr).
		Named("hubha-lifecycle").
		For(&corev1.ConfigMap{}).
		WithEventFilter(configMapPredicate).
		Complete(c); err != nil {
		return fmt.Errorf("failed to add Hub HA lifecycle controller: %w", err)
	}
	return nil
}

// Reconcile reacts to agent ConfigMap changes and adjusts the Hub HA syncer accordingly.
func (c *hubHALifecycleController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Refresh the stored AgentConfig pointer to the current global singleton.
	// In production the singleton never changes; in integration tests BeforeEach
	// may replace it with a new pointer, so refreshing here keeps hub-role updates
	// visible to all readers of configs.GetAgentConfig().
	if current := configs.GetAgentConfig(); current != nil {
		c.agentConfig = current
	}

	agentConfigMap := &corev1.ConfigMap{}
	if err := c.client.Get(ctx, req.NamespacedName, agentConfigMap); err != nil {
		if apierrors.IsNotFound(err) {
			// ConfigMap deleted: treat as desired-disabled state so any running emitter
			// does not continue forwarding with stale role/standby information.
			if disableErr := c.disableSyncer(); disableErr != nil {
				return ctrl.Result{}, fmt.Errorf("failed to disable Hub HA syncer after ConfigMap deletion: %w", disableErr)
			}
			c.agentConfig.UpdateHubRoleAndStandbyHub("", "")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get agent ConfigMap: %w", err)
	}

	// Read hub role and standby hub from the ConfigMap data.
	newHubRole := agentConfigMap.Data["hubRole"]
	newStandbyHub := agentConfigMap.Data["standbyHub"]

	// Atomically update AgentConfig and retrieve previous values for change detection.
	previousRole, previousStandby := c.agentConfig.UpdateHubRoleAndStandbyHub(newHubRole, newStandbyHub)

	roleChanged := previousRole != newHubRole
	standbyChanged := previousStandby != newStandbyHub
	shouldEnable := newHubRole == constants.GHHubRoleActive && newStandbyHub != ""

	// Skip reconciliation only when both the ConfigMap values are unchanged AND
	// the controller is already in the desired state (prevents stale no-ops on
	// failed enableSyncer retries or first-reconcile with pre-populated AgentConfig).
	c.mu.Lock()
	registered := c.registered
	c.mu.Unlock()

	if !roleChanged && !standbyChanged && ((shouldEnable && registered) || (!shouldEnable && !registered)) {
		return ctrl.Result{}, nil
	}

	log.Infow("Hub HA ConfigMap changed",
		"previousRole", previousRole, "newRole", newHubRole,
		"standbyConfigured", newStandbyHub != "")

	if shouldEnable {
		return ctrl.Result{}, c.enableSyncer(ctx, newStandbyHub)
	}

	return ctrl.Result{}, c.disableSyncer()
}

// enableSyncer starts the resource controller (once) and registers the emitter
// with the PeriodicSyncer so that periodic resyncs fire.
func (c *hubHALifecycleController) enableSyncer(ctx context.Context, standbyHub string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Create or update the emitter.
	if c.emitter == nil {
		if c.agentConfig == nil {
			return fmt.Errorf("agent config not initialised")
		}
		c.emitter = specHubHA.NewHubHAEmitter(
			c.producer,
			c.agentConfig.TransportConfig,
			c.agentConfig.LeafHubName,
			standbyHub,
		)
		c.emitter.SetClient(c.client)
	} else {
		c.emitter.SetStandbyHub(standbyHub)
	}

	// Start the resource-watching controller exactly once.
	// Use the injectable fn so tests can avoid requiring a live ctrl.Manager.
	startFn := c.startResourceSyncerFn
	if startFn == nil {
		startFn = func(emitter *specHubHA.HubHAEmitter) ([]schema.GroupVersionKind, error) {
			allGVKs := specHubHA.GetHubHAResourcesToSync()
			return specHubHA.StartHubHAResourceSyncer(c.mgr, allGVKs, emitter)
		}
	}

	// Start the resource-watching controller only once, but retry if a previous
	// attempt failed (unlike sync.Once which would permanently suppress retries).
	if !c.resourceSyncerStarted {
		activeGVKs, err := startFn(c.emitter)
		if err != nil {
			return fmt.Errorf("failed to start Hub HA resource syncer: %w", err)
		}
		c.emitter.SetActiveResources(activeGVKs)
		c.resourceSyncerStarted = true
		log.Infof("Hub HA resource syncer started, watching %d GVKs", len(activeGVKs))
	}

	c.emitter.SetEnabled(true)

	if !c.registered {
		c.periodicSyncer.Register(&generic.EmitterRegistration{
			Emitter: c.emitter,
			// ListFunc is nil — the emitter self-lists via Resync(nil).
		})
		c.registered = true
		log.Info("Hub HA emitter registered with PeriodicSyncer")
	}

	return nil
}

// disableSyncer disables the emitter and removes it from the PeriodicSyncer.
func (c *hubHALifecycleController) disableSyncer() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.emitter != nil {
		c.emitter.SetEnabled(false)
	}

	if c.registered {
		c.periodicSyncer.Unregister(constants.HubHAResourcesMsgKey)
		c.registered = false
		log.Info("Hub HA emitter unregistered from PeriodicSyncer")
	}

	return nil
}
