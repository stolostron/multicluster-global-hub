// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package lifecycle

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	agentconfigs "github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/syncers/configmap"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

type mockProducer struct {
	mu      sync.RWMutex
	events  chan cloudevents.Event
	closed  bool
	dropped atomic.Int64
}

func newMockProducer() *mockProducer {
	return &mockProducer{
		events: make(chan cloudevents.Event, 1000),
	}
}

func (m *mockProducer) SendEvent(ctx context.Context, evt cloudevents.Event) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed {
		return nil
	}

	select {
	case m.events <- evt:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		m.dropped.Add(1)
		GinkgoWriter.Printf("WARNING: Event dropped! Total dropped: %d\n", m.dropped.Load())
		return nil
	}
}

func (m *mockProducer) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}
	m.closed = true
	close(m.events)
}

func (m *mockProducer) Reconnect(config *transport.TransportInternalConfig, clusterName string) error {
	return nil
}

func drainProducerEvents(producer *mockProducer) {
	for {
		select {
		case <-producer.events:
		default:
			return
		}
	}
}

func upsertAgentConfigCM(ctx context.Context, hubRole, standbyHub string) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.GHAgentConfigCMName,
			Namespace: constants.GHAgentNamespace,
		},
	}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      constants.GHAgentConfigCMName,
		Namespace: constants.GHAgentNamespace,
	}, cm)
	if err != nil {
		cm.Data = map[string]string{}
		Expect(k8sClient.Create(ctx, cm)).To(Succeed(), "failed to create agent ConfigMap %s/%s", constants.GHAgentNamespace, constants.GHAgentConfigCMName)
	}

	Eventually(func() error {
		current := &corev1.ConfigMap{}
		if getErr := k8sClient.Get(ctx, types.NamespacedName{
			Name:      constants.GHAgentConfigCMName,
			Namespace: constants.GHAgentNamespace,
		}, current); getErr != nil {
			return getErr
		}
		if current.Data == nil {
			current.Data = map[string]string{}
		}
		if hubRole == "" {
			delete(current.Data, configmap.AgentHubRoleKey)
		} else {
			current.Data[configmap.AgentHubRoleKey] = hubRole
		}
		if standbyHub == "" {
			delete(current.Data, "standbyHub")
		} else {
			current.Data["standbyHub"] = standbyHub
		}
		return k8sClient.Update(ctx, current)
	}, 5*time.Second, 200*time.Millisecond).Should(Succeed(), "failed to upsert agent ConfigMap with hubRole=%q standbyHub=%q", hubRole, standbyHub)
}

func waitForAgentHubRole(hubRole, standbyHub string) {
	Eventually(func(g Gomega) {
		config := agentconfigs.GetAgentConfig()
		g.Expect(config).NotTo(BeNil())
		g.Expect(config.GetHubRole()).To(Equal(hubRole))
		g.Expect(config.GetStandbyHub()).To(Equal(standbyHub))
	}, 10*time.Second, 200*time.Millisecond).Should(Succeed(), "agent config hubRole/standbyHub did not reach %q/%q", hubRole, standbyHub)
}

func deleteAgentConfigCM(ctx context.Context) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.GHAgentConfigCMName,
			Namespace: constants.GHAgentNamespace,
		},
	}
	Expect(k8sClient.Delete(ctx, cm)).To(Succeed(), "failed to delete agent ConfigMap %s/%s", constants.GHAgentNamespace, constants.GHAgentConfigCMName)
	Eventually(func() bool {
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      constants.GHAgentConfigCMName,
			Namespace: constants.GHAgentNamespace,
		}, &corev1.ConfigMap{})
		return err != nil
	}, 10*time.Second, 200*time.Millisecond).Should(BeTrue(), "agent ConfigMap should be deleted")
}

func assertNoHubHAEvents(producer *mockProducer, reason string) {
	Consistently(func() bool {
		select {
		case <-producer.events:
			return false
		default:
			return true
		}
	}, 3*time.Second, 200*time.Millisecond).Should(BeTrue(), reason)
}

func assertSyncerDisabledFor(ctx context.Context, producer *mockProducer, cmName, reason string) {
	drainProducerEvents(producer)
	createSyncableConfigMap(ctx, cmName)
	assertNoHubHAEvents(producer, reason)
}

func createSyncableConfigMap(ctx context.Context, name string) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			Labels: map[string]string{
				"hive.openshift.io/secret-type": "kubeconfig",
			},
		},
		Data: map[string]string{
			"key": "value",
		},
	}
	Expect(k8sClient.Create(ctx, cm)).To(Succeed(), "failed to create syncable ConfigMap %s", name)
}

func waitForHubHAEvent(producer *mockProducer, name string) cloudevents.Event {
	var evt cloudevents.Event
	Eventually(func() bool {
		select {
		case received := <-producer.events:
			Expect(received.Type()).To(Equal(constants.HubHAResourcesMsgKey))
			Expect(received.Source()).To(Equal("hub1"))

			bundle := generic.NewGenericBundle[*unstructured.Unstructured]()
			Expect(received.DataAs(bundle)).To(Succeed())
			for _, obj := range bundle.Update {
				if obj.GetKind() == "ConfigMap" && obj.GetName() == name {
					evt = received
					return true
				}
			}
			return false
		case <-time.After(100 * time.Millisecond):
			return false
		}
	}, 10*time.Second, 100*time.Millisecond).Should(BeTrue(), "expected Hub HA event for ConfigMap %s", name)
	return evt
}

var _ = Describe("Hub HA Lifecycle E2E Integration", Ordered, func() {
	var (
		agentNamespace = constants.GHAgentNamespace
		configMapName  = constants.GHAgentConfigCMName
	)

	AfterAll(func() {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: agentNamespace,
			},
		}
		_ = k8sClient.Delete(context.Background(), cm)
	})

	It("should start active syncer and send events when ConfigMap sets hubRole=active and standbyHub", func() {
		testCtx := context.Background()

		upsertAgentConfigCM(testCtx, constants.GHHubRoleActive, "hub2")
		waitForAgentHubRole(constants.GHHubRoleActive, "hub2")
		drainProducerEvents(suiteProducer)

		createSyncableConfigMap(testCtx, "lifecycle-e2e-active-cm")
		waitForHubHAEvent(suiteProducer, "lifecycle-e2e-active-cm")
	})

	It("should stop sending events when hubRole changes to standby", func() {
		testCtx := context.Background()

		upsertAgentConfigCM(testCtx, constants.GHHubRoleStandby, "hub2")
		waitForAgentHubRole(constants.GHHubRoleStandby, "hub2")
		assertSyncerDisabledFor(
			testCtx,
			suiteProducer,
			"lifecycle-e2e-standby-cm",
			"active syncer should not emit events in standby role",
		)
	})

	It("should route events to the updated standby hub without restarting the agent", func() {
		testCtx := context.Background()

		upsertAgentConfigCM(testCtx, constants.GHHubRoleActive, "hub2")
		waitForAgentHubRole(constants.GHHubRoleActive, "hub2")
		drainProducerEvents(suiteProducer)

		upsertAgentConfigCM(testCtx, constants.GHHubRoleActive, "hub3")
		waitForAgentHubRole(constants.GHHubRoleActive, "hub3")
		drainProducerEvents(suiteProducer)

		createSyncableConfigMap(testCtx, "lifecycle-e2e-new-standby-cm")
		evt := waitForHubHAEvent(suiteProducer, "lifecycle-e2e-new-standby-cm")
		Expect(evt.Subject()).To(Equal("hub3"), "emitter should route events to updated standby hub")
	})

	It("should stop sending events when the agent ConfigMap is deleted", func() {
		testCtx := context.Background()

		// Prior spec leaves active/hub3; confirm syncer is still emitting before delete.
		drainProducerEvents(suiteProducer)
		createSyncableConfigMap(testCtx, "lifecycle-e2e-pre-delete-cm")
		waitForHubHAEvent(suiteProducer, "lifecycle-e2e-pre-delete-cm")

		deleteAgentConfigCM(testCtx)
		waitForAgentHubRole("", "")
		assertSyncerDisabledFor(
			testCtx,
			suiteProducer,
			"lifecycle-e2e-after-delete-cm",
			"active syncer should not emit events after agent ConfigMap deletion",
		)
	})
})
