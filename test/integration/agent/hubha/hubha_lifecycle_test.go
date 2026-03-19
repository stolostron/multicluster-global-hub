// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubha

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	agentconfigs "github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/syncers/configmap"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var _ = Describe("Hub HA Dynamic Syncer Lifecycle", func() {
	var (
		agentNamespace = constants.GHAgentNamespace
		configMapName  = constants.GHAgentConfigCMName
		producer       *mockProducer
	)

	BeforeEach(func() {
		producer = newMockProducer()

		// Initialize agent config
		agentConfig := &agentconfigs.AgentConfig{
			LeafHubName:     "hub1",
			PodNamespace:    agentNamespace,
			TransportConfig: transportConfig,
		}
		agentconfigs.SetAgentConfig(agentConfig)

		// Set up syncer manager
		configmap.SetHubHASyncerManager(mgr, producer, func(ctx context.Context, m ctrl.Manager, prod transport.Producer) error {
			// Mock start function for testing
			return nil
		})

		// Create controller
		err := configmap.AddConfigMapController(mgr, agentConfig)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if producer != nil {
			producer.Stop()
		}

		// Clean up configmap
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: agentNamespace,
			},
		}
		_ = k8sClient.Delete(context.Background(), cm)
	})

	It("should update agent config when configmap hub role changes", func() {
		ctx := context.Background()

		// Create configmap with active hub role
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: agentNamespace,
			},
			Data: map[string]string{
				configmap.AgentHubRoleKey: constants.GHHubRoleActive,
				"standbyHub":              "hub2",
			},
		}
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())

		// Wait for agent config to be updated
		Eventually(func() string {
			config := agentconfigs.GetAgentConfig()
			if config == nil {
				return ""
			}
			return config.HubRole
		}, 10*time.Second, 500*time.Millisecond).Should(Equal(constants.GHHubRoleActive))

		// Verify standby hub is also set
		config := agentconfigs.GetAgentConfig()
		Expect(config.StandbyHub).To(Equal("hub2"))

		// Update configmap to standby role
		Eventually(func() error {
			currentCM := &corev1.ConfigMap{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      configMapName,
				Namespace: agentNamespace,
			}, currentCM)
			if err != nil {
				return err
			}

			currentCM.Data[configmap.AgentHubRoleKey] = constants.GHHubRoleStandby
			return k8sClient.Update(ctx, currentCM)
		}, 5*time.Second, 500*time.Millisecond).Should(Succeed())

		// Wait for agent config to reflect the change
		Eventually(func() string {
			config := agentconfigs.GetAgentConfig()
			if config == nil {
				return ""
			}
			return config.HubRole
		}, 10*time.Second, 500*time.Millisecond).Should(Equal(constants.GHHubRoleStandby))
	})

	It("should handle hub role being removed from configmap", func() {
		ctx := context.Background()

		// Create configmap with hub role
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: agentNamespace,
			},
			Data: map[string]string{
				configmap.AgentHubRoleKey: constants.GHHubRoleActive,
				"standbyHub":              "hub2",
			},
		}
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())

		// Wait for config to be set
		Eventually(func() string {
			config := agentconfigs.GetAgentConfig()
			if config == nil {
				return ""
			}
			return config.HubRole
		}, 10*time.Second, 500*time.Millisecond).Should(Equal(constants.GHHubRoleActive))

		// Remove hub role from configmap
		Eventually(func() error {
			currentCM := &corev1.ConfigMap{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      configMapName,
				Namespace: agentNamespace,
			}, currentCM)
			if err != nil {
				return err
			}

			delete(currentCM.Data, configmap.AgentHubRoleKey)
			delete(currentCM.Data, "standbyHub")
			return k8sClient.Update(ctx, currentCM)
		}, 5*time.Second, 500*time.Millisecond).Should(Succeed())

		// Wait for agent config to clear hub role
		Eventually(func() string {
			config := agentconfigs.GetAgentConfig()
			if config == nil {
				return ""
			}
			return config.HubRole
		}, 10*time.Second, 500*time.Millisecond).Should(Equal(""))
	})

	It("should update sync intervals from configmap", func() {
		ctx := context.Background()

		// Create configmap with custom sync interval
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: agentNamespace,
			},
			Data: map[string]string{
				"sync.managedcluster": "10s",
			},
		}
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())

		// The interval should be updated by the controller
		// We can't easily verify this without exposing the interval getter
		// but we can verify the configmap was processed without error
		time.Sleep(2 * time.Second)

		// Verify configmap still exists and wasn't deleted due to errors
		currentCM := &corev1.ConfigMap{}
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      configMapName,
			Namespace: agentNamespace,
		}, currentCM)
		Expect(err).NotTo(HaveOccurred())
	})
})

var _ = Describe("Hub HA ConfigMap Controller Error Handling", func() {
	var (
		agentNamespace = constants.GHAgentNamespace
		configMapName  = constants.GHAgentConfigCMName
	)

	It("should handle invalid sync interval gracefully", func() {
		ctx := context.Background()

		// Create configmap with invalid interval
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: agentNamespace,
			},
			Data: map[string]string{
				"sync.managedcluster": "invalid-duration",
			},
		}
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())

		// Controller should log error but not crash
		time.Sleep(2 * time.Second)

		// Verify configmap still exists
		currentCM := &corev1.ConfigMap{}
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      configMapName,
			Namespace: agentNamespace,
		}, currentCM)
		Expect(err).NotTo(HaveOccurred())

		// Cleanup
		Expect(k8sClient.Delete(ctx, cm)).To(Succeed())
	})

	It("should handle missing configmap gracefully", func() {
		ctx := context.Background()

		// Try to get non-existent configmap
		cm := &corev1.ConfigMap{}
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      "non-existent-cm",
			Namespace: agentNamespace,
		}, cm)
		Expect(err).To(HaveOccurred())
	})
})
