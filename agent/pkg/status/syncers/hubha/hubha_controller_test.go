// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubha

import (
	"context"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	specHubHA "github.com/stolostron/multicluster-global-hub/agent/pkg/spec/hubha"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

const (
	testNamespace  = "open-cluster-management"
	testConfigName = constants.GHAgentConfigCMName
)

// ----- helpers -----

func buildScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatalf("failed to add corev1 to test scheme: %v", err)
	}
	return s
}

func agentCM(namespace, name, hubRole, standbyHub string) *corev1.ConfigMap {
	data := map[string]string{}
	if hubRole != "" {
		data["hubRole"] = hubRole
	}
	if standbyHub != "" {
		data["standbyHub"] = standbyHub
	}
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Data:       data,
	}
}

func newAgentConfig(role, standbyHub string) *configs.AgentConfig {
	cfg := &configs.AgentConfig{}
	if role != "" || standbyHub != "" {
		cfg.UpdateHubRoleAndStandbyHub(role, standbyHub)
	}
	return cfg
}

// stubController creates a lifecycle controller ready for unit tests.
// It injects a no-op startResourceSyncerFn to avoid needing a real ctrl.Manager.
func stubController(
	t *testing.T, cm *corev1.ConfigMap, agentCfg *configs.AgentConfig,
) (*hubHALifecycleController, *generic.PeriodicSyncer) {
	t.Helper()
	fc := fake.NewClientBuilder().WithScheme(buildScheme(t)).WithObjects(cm).Build()
	ps := &generic.PeriodicSyncer{}

	c := &hubHALifecycleController{
		client:         fc,
		agentConfig:    agentCfg,
		periodicSyncer: ps,
		// startResourceSyncerFn is injected as a no-op so tests don't need a manager.
		startResourceSyncerFn: func(emitter *specHubHA.HubHAEmitter) ([]schema.GroupVersionKind, error) {
			return nil, nil
		},
	}
	return c, ps
}

// ----- tests -----

// TestReconcile_ConfigMapNotFound returns no error and no requeue.
func TestReconcile_ConfigMapNotFound(t *testing.T) {
	fc := fake.NewClientBuilder().WithScheme(buildScheme(t)).Build() // empty store
	c := &hubHALifecycleController{
		client:         fc,
		periodicSyncer: &generic.PeriodicSyncer{},
		agentConfig:    newAgentConfig("", ""),
	}

	result, err := c.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testConfigName},
	})
	if err != nil {
		t.Fatalf("expected no error for missing CM, got %v", err)
	}
	if result.Requeue {
		t.Error("expected no requeue for missing CM")
	}
}

// TestReconcile_NoChange_AlreadyRegistered skips enable/disable when values are
// unchanged AND the controller is already in the desired state.
func TestReconcile_NoChange_AlreadyRegistered(t *testing.T) {
	cm := agentCM(testNamespace, testConfigName, constants.GHHubRoleActive, "standby-hub")
	agentCfg := newAgentConfig(constants.GHHubRoleActive, "standby-hub")

	c, ps := stubController(t, cm, agentCfg)

	// Pre-register to simulate already-enabled state.
	emitter := specHubHA.NewHubHAEmitter(nil, nil, "active", "standby-hub")
	emitter.SetEnabled(true)
	c.emitter = emitter
	c.registered = true
	c.resourceSyncerStarted = true
	ps.Register(&generic.EmitterRegistration{Emitter: emitter})

	_, err := c.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testConfigName},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// State is already correct and values unchanged — emitter count stays at 1.
	if ps.CountRegistered() != 1 {
		t.Errorf("expected 1 registration (already enabled, no change), got %d", ps.CountRegistered())
	}
}

// TestReconcile_NoChange_NotYetRegistered ensures enableSyncer is called even when
// ConfigMap values match AgentConfig but the emitter is not yet registered
// (e.g. first reconcile after agent restart).
func TestReconcile_NoChange_NotYetRegistered(t *testing.T) {
	cm := agentCM(testNamespace, testConfigName, constants.GHHubRoleActive, "standby-hub")
	agentCfg := newAgentConfig(constants.GHHubRoleActive, "standby-hub") // pre-populated

	c, ps := stubController(t, cm, agentCfg)
	// c.registered == false (new controller, emitter not yet running)

	_, err := c.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testConfigName},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// enableSyncer must run to register the emitter even though values didn't change.
	if ps.CountRegistered() != 1 {
		t.Errorf("expected 1 registration (values unchanged but not yet registered), got %d", ps.CountRegistered())
	}
}

// TestReconcile_BecomeActive_RegistersEmitter transitions from no-role to active.
func TestReconcile_BecomeActive_RegistersEmitter(t *testing.T) {
	cm := agentCM(testNamespace, testConfigName, constants.GHHubRoleActive, "standby-hub")
	agentCfg := newAgentConfig("", "") // no previous role

	c, ps := stubController(t, cm, agentCfg)

	_, err := c.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testConfigName},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ps.CountRegistered() != 1 {
		t.Errorf("expected 1 registration after becoming active, got %d", ps.CountRegistered())
	}
	if c.emitter == nil {
		t.Error("expected emitter to be initialised")
	}
	if !c.registered {
		t.Error("expected registered=true after enable")
	}
}

// TestReconcile_BecomeStandby_UnregistersEmitter transitions from active to standby.
func TestReconcile_BecomeStandby_UnregistersEmitter(t *testing.T) {
	cm := agentCM(testNamespace, testConfigName, constants.GHHubRoleStandby, "")
	agentCfg := newAgentConfig(constants.GHHubRoleActive, "standby-hub") // previously active

	c, ps := stubController(t, cm, agentCfg)

	// Pre-register an emitter to simulate the active state.
	emitter := specHubHA.NewHubHAEmitter(nil, nil, "active", "standby-hub")
	emitter.SetEnabled(true)
	c.emitter = emitter
	c.registered = true
	ps.Register(&generic.EmitterRegistration{Emitter: emitter})

	_, err := c.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testConfigName},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ps.CountRegistered() != 0 {
		t.Errorf("expected 0 registrations after becoming standby, got %d", ps.CountRegistered())
	}
	if c.registered {
		t.Error("expected registered=false")
	}
}

// TestReconcile_ActiveWithoutStandby_DoesNotRegister verifies that active role
// without a standby hub does not register the emitter.
func TestReconcile_ActiveWithoutStandby_DoesNotRegister(t *testing.T) {
	cm := agentCM(testNamespace, testConfigName, constants.GHHubRoleActive, "")
	agentCfg := newAgentConfig("", "") // no previous role

	c, ps := stubController(t, cm, agentCfg)

	_, err := c.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testConfigName},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ps.CountRegistered() != 0 {
		t.Errorf("expected 0 registrations (no standby), got %d", ps.CountRegistered())
	}
}

// TestDisableSyncer_NilEmitter is a no-op and does not panic.
func TestDisableSyncer_NilEmitter(t *testing.T) {
	c := &hubHALifecycleController{
		periodicSyncer: &generic.PeriodicSyncer{},
	}
	if err := c.disableSyncer(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestReconcile_ConfigMapDeleted_DisablesSyncer verifies that when the agent ConfigMap
// is deleted the emitter is disabled rather than left running with stale data.
func TestReconcile_ConfigMapDeleted_DisablesSyncer(t *testing.T) {
	// Build a controller with an empty store (simulates deleted CM).
	fc := fake.NewClientBuilder().WithScheme(buildScheme(t)).Build()
	ps := &generic.PeriodicSyncer{}
	emitter := specHubHA.NewHubHAEmitter(nil, nil, "active", "standby-hub")
	emitter.SetEnabled(true)
	ps.Register(&generic.EmitterRegistration{Emitter: emitter})

	c := &hubHALifecycleController{
		client:         fc,
		periodicSyncer: ps,
		agentConfig:    newAgentConfig(constants.GHHubRoleActive, "standby-hub"),
		emitter:        emitter,
		registered:     true,
		startResourceSyncerFn: func(e *specHubHA.HubHAEmitter) ([]schema.GroupVersionKind, error) {
			return nil, nil
		},
	}

	_, err := c.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testConfigName},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.registered {
		t.Error("expected registered=false after CM deletion")
	}
	if ps.CountRegistered() != 0 {
		t.Errorf("expected 0 registrations after CM deletion, got %d", ps.CountRegistered())
	}
}

// TestDisableSyncer_WithRegisteredEmitter unregisters and disables.
func TestDisableSyncer_WithRegisteredEmitter(t *testing.T) {
	ps := &generic.PeriodicSyncer{}
	emitter := specHubHA.NewHubHAEmitter(nil, nil, "active", "standby")
	emitter.SetEnabled(true)
	ps.Register(&generic.EmitterRegistration{Emitter: emitter})

	c := &hubHALifecycleController{
		periodicSyncer: ps,
		emitter:        emitter,
		registered:     true,
	}

	if err := c.disableSyncer(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.registered {
		t.Error("expected registered=false after disableSyncer")
	}
	if ps.CountRegistered() != 0 {
		t.Errorf("expected 0 registrations, got %d", ps.CountRegistered())
	}
}

// ----- additional coverage tests -----

// TestWithNoOpStartResourceSyncerFn verifies the option installs a fn that
// returns no GVKs and no error.
func TestWithNoOpStartResourceSyncerFn(t *testing.T) {
	c := &hubHALifecycleController{}
	WithNoOpStartResourceSyncerFn()(c)

	if c.startResourceSyncerFn == nil {
		t.Fatal("expected startResourceSyncerFn to be non-nil after applying option")
	}
	gvks, err := c.startResourceSyncerFn(nil)
	if err != nil {
		t.Errorf("no-op startFn returned unexpected error: %v", err)
	}
	if gvks != nil {
		t.Errorf("no-op startFn returned non-nil GVKs: %v", gvks)
	}
}

// TestReconcile_GetCMError_NonNotFound returns the error when the client
// returns a non-NotFound error fetching the ConfigMap.
func TestReconcile_GetCMError_NonNotFound(t *testing.T) {
	base := fake.NewClientBuilder().WithScheme(buildScheme(t)).Build()
	interceptedClient := interceptor.NewClient(base, interceptor.Funcs{
		Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
			return fmt.Errorf("connection refused")
		},
	})

	c := &hubHALifecycleController{
		client:         interceptedClient,
		periodicSyncer: &generic.PeriodicSyncer{},
		agentConfig:    newAgentConfig("", ""),
	}

	_, err := c.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testConfigName},
	})
	if err == nil {
		t.Error("expected error for non-NotFound client failure, got nil")
	}
}

// TestEnableSyncer_NilAgentConfig returns an error when agentConfig is nil
// and no emitter has been created yet.
func TestEnableSyncer_NilAgentConfig(t *testing.T) {
	c := &hubHALifecycleController{
		periodicSyncer: &generic.PeriodicSyncer{},
		startResourceSyncerFn: func(_ *specHubHA.HubHAEmitter) ([]schema.GroupVersionKind, error) {
			return nil, nil
		},
	}
	// agentConfig is nil and emitter is nil → enableSyncer must return error
	if err := c.enableSyncer(context.Background(), "standby-hub"); err == nil {
		t.Error("expected error when agentConfig is nil, got nil")
	}
}

// TestEnableSyncer_StartFnError propagates the startFn error.
func TestEnableSyncer_StartFnError(t *testing.T) {
	cm := agentCM(testNamespace, testConfigName, constants.GHHubRoleActive, "standby-hub")
	agentCfg := newAgentConfig("", "")

	fc := fake.NewClientBuilder().WithScheme(buildScheme(t)).WithObjects(cm).Build()
	ps := &generic.PeriodicSyncer{}
	c := &hubHALifecycleController{
		client:         fc,
		agentConfig:    agentCfg,
		periodicSyncer: ps,
		startResourceSyncerFn: func(_ *specHubHA.HubHAEmitter) ([]schema.GroupVersionKind, error) {
			return nil, fmt.Errorf("resource syncer registration failed")
		},
	}

	_, err := c.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testConfigName},
	})
	if err == nil {
		t.Error("expected error when startFn fails, got nil")
	}
}

// TestEnableSyncer_ExistingEmitter_UpdatesStandbyHub verifies that a second
// enableSyncer call updates the standby hub name on the existing emitter rather
// than creating a new one.
func TestEnableSyncer_ExistingEmitter_UpdatesStandbyHub(t *testing.T) {
	cm := agentCM(testNamespace, testConfigName, constants.GHHubRoleActive, "new-standby")
	agentCfg := newAgentConfig(constants.GHHubRoleActive, "old-standby")

	c, ps := stubController(t, cm, agentCfg)

	// Pre-populate with an existing emitter (simulates already-enabled state with a new standby hub).
	existingEmitter := specHubHA.NewHubHAEmitter(nil, nil, "hub1", "old-standby")
	existingEmitter.SetEnabled(true)
	c.emitter = existingEmitter
	c.resourceSyncerStarted = true
	// c.registered = false so enableSyncer still runs to register.

	_, err := c.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testConfigName},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The same emitter object should be reused (not replaced).
	if c.emitter != existingEmitter {
		t.Error("expected existing emitter to be reused, but a new one was created")
	}
	if ps.CountRegistered() != 1 {
		t.Errorf("expected 1 registration, got %d", ps.CountRegistered())
	}
}
