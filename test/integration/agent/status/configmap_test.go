package status

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/syncers/configmap"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

// go test ./test/integration/agent/status -v -ginkgo.focus "Config Controller Integration"
var _ = Describe("Config Controller Integration", func() {
	var cm *corev1.ConfigMap

	BeforeEach(func() {
		// Initialize the ConfigMap in BeforeEach where agentConfig is available
		cm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: agentConfig.PodNamespace,
				Name:      constants.GHAgentConfigCMName,
			},
			Data: map[string]string{},
		}

		// create the configmap if not exists, if exists, skip create it
		Eventually(func() error {
			err := runtimeClient.Get(ctx, client.ObjectKeyFromObject(cm), cm)
			if err != nil {
				if errors.IsNotFound(err) {
					// ConfigMap doesn't exist, create it
					return runtimeClient.Create(ctx, &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: agentConfig.PodNamespace,
							Name:      constants.GHAgentConfigCMName,
						},
					})
				}
				// Other error, return it
				return err
			}
			// ConfigMap exists, nothing to do
			return nil
		}, 10*time.Second, time.Second).Should(Succeed())
	})

	Context("When ConfigMap is created or updated", func() {
		It("Should update sync intervals correctly", func() {
			By("Creating a ConfigMap with custom sync intervals")
			cm.Data = map[string]string{
				"resync.managedcluster":   "30m",
				"policy.localspec":        "3s",
				"resync.policy.localspec": "45m",

				"managedhub.info":        "2s",
				"resync.managedhub.info": "2h",

				"managedhub.heartbeat":        "2s",
				"resync.managedhub.heartbeat": "20m",

				"event.managedcluster":        "3s",
				"resync.event.managedcluster": "25m",
			}

			// Update the ConfigMap
			Expect(runtimeClient.Update(ctx, cm)).Should(Succeed())

			// Verify intervals were updated
			Eventually(func() error {
				if configmap.GetSyncInterval(enum.ManagedClusterType) != 5*time.Second {
					return fmt.Errorf("should get the default cluster interval 5s, but got %s", configmap.GetSyncInterval(enum.ManagedClusterType))
				}

				if configmap.GetResyncInterval(enum.ManagedClusterType) != 30*time.Minute {
					return fmt.Errorf("should get the default cluster resync interval 30m, but got %s",
						configmap.GetResyncInterval(enum.ManagedClusterType))
				}

				if configmap.GetSyncInterval(enum.LocalPolicySpecType) != 3*time.Second {
					return fmt.Errorf("the local policy spec interval should be %s", "3s")
				}

				if configmap.GetResyncInterval(enum.LocalPolicySpecType) != 45*time.Minute {
					return fmt.Errorf("the local policy spec resync interval should be %s", "45m")
				}

				if configmap.GetSyncInterval(enum.HubClusterInfoType) != 2*time.Second {
					return fmt.Errorf("the hub cluster info interval should be %s", "2s")
				}

				if configmap.GetResyncInterval(enum.HubClusterInfoType) != 2*time.Hour {
					return fmt.Errorf("the hub cluster info resync interval should be %s", "2h")
				}

				if configmap.GetSyncInterval(enum.HubClusterHeartbeatType) != 2*time.Second {
					return fmt.Errorf("the hub cluster heartbeat interval should be %s", "2s")
				}

				if configmap.GetResyncInterval(enum.HubClusterHeartbeatType) != 20*time.Minute {
					return fmt.Errorf("the hub cluster heartbeat resync interval should be %s", "20m")
				}

				if configmap.GetSyncInterval(enum.ManagedClusterEventType) != 3*time.Second {
					return fmt.Errorf("the managed cluster event interval should be %s", "3s")
				}

				if configmap.GetResyncInterval(enum.ManagedClusterEventType) != 25*time.Minute {
					return fmt.Errorf("the managed cluster event resync interval should be %s", "25m")
				}
				return nil
			}, 30*time.Second, time.Second).Should(BeNil())
		})

		It("Should handle invalid sync intervals gracefully", func() {
			By("Creating a ConfigMap with invalid sync intervals")
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: agentConfig.PodNamespace,
					Name:      constants.GHAgentConfigCMName,
				},
				Data: map[string]string{
					"managedcluster":        "invalid-duration",
					"resync.managedcluster": "also-invalid",
					"managedhub.info":       "3s", // valid one
				},
			}

			// Store original intervals
			originalSyncInterval := configmap.GetSyncInterval(enum.ManagedClusterType)
			originalResyncInterval := configmap.GetResyncInterval(enum.ManagedClusterType)

			// Update the ConfigMap
			Eventually(func() error {
				return runtimeClient.Update(ctx, configMap)
			}, 10*time.Second, time.Second).Should(Succeed())

			// Wait for reconciliation
			time.Sleep(2 * time.Second)

			// Verify invalid intervals were ignored (kept default values)
			Expect(configmap.GetSyncInterval(enum.ManagedClusterType)).To(Equal(originalSyncInterval))
			Expect(configmap.GetResyncInterval(enum.ManagedClusterType)).To(Equal(originalResyncInterval))

			// Verify valid interval was updated
			Expect(configmap.GetSyncInterval(enum.HubClusterInfoType)).To(Equal(3 * time.Second))
		})

		It("Should update agent configurations correctly", func() {
			By("Creating a ConfigMap with agent configurations")
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: agentConfig.PodNamespace,
					Name:      constants.GHAgentConfigCMName,
				},
				Data: map[string]string{
					"enableLocalPolicies": "true",
					"logLevel":            "debug",
				},
			}

			// Update the ConfigMap
			Eventually(func() error {
				return runtimeClient.Update(ctx, configMap)
			}, 10*time.Second, time.Second).Should(Succeed())

			// Wait for reconciliation
			time.Sleep(2 * time.Second)

			// Verify agent configs were updated
			Expect(configmap.GetEnableLocalPolicy()).To(Equal(configmap.AgentConfigValue("true")))
		})

		It("Should handle missing ConfigMap gracefully", func() {
			By("Deleting the ConfigMap")
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: agentConfig.PodNamespace,
					Name:      constants.GHAgentConfigCMName,
				},
			}

			// Delete the ConfigMap
			Eventually(func() error {
				return runtimeClient.Delete(ctx, configMap)
			}, 10*time.Second, time.Second).Should(Succeed())

			// Wait for reconciliation
			time.Sleep(2 * time.Second)

			// Create a new ConfigMap to trigger reconciliation
			newConfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: constants.GHAgentNamespace,
					Name:      constants.GHAgentConfigCMName,
				},
				Data: map[string]string{
					"managedhub.info": "1s",
				},
			}

			Eventually(func() error {
				return runtimeClient.Create(ctx, newConfigMap)
			}, 10*time.Second, time.Second).Should(Succeed())

			// Verify the controller handles the recreation properly
			Eventually(func() time.Duration {
				return configmap.GetSyncInterval(enum.HubClusterInfoType)
			}, 15*time.Second, time.Second).Should(Equal(1 * time.Second))
		})
	})

	Context("When ConfigMap has different data scenarios", func() {
		It("Should handle empty ConfigMap data", func() {
			By("Creating a ConfigMap with empty data")
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: agentConfig.PodNamespace,
					Name:      constants.GHAgentConfigCMName,
				},
				Data: map[string]string{},
			}

			// Store original intervals
			originalSyncInterval := configmap.GetSyncInterval(enum.ManagedClusterType)
			originalResyncInterval := configmap.GetResyncInterval(enum.ManagedClusterType)

			// Update the ConfigMap
			Eventually(func() error {
				return runtimeClient.Update(ctx, configMap)
			}, 10*time.Second, time.Second).Should(Succeed())

			// Wait for reconciliation
			time.Sleep(2 * time.Second)

			// Verify intervals remained unchanged
			Expect(configmap.GetSyncInterval(enum.ManagedClusterType)).To(Equal(originalSyncInterval))
			Expect(configmap.GetResyncInterval(enum.ManagedClusterType)).To(Equal(originalResyncInterval))
		})

		It("Should handle ConfigMap with mixed valid and invalid data", func() {
			By("Creating a ConfigMap with mixed data")
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: agentConfig.PodNamespace,
					Name:      constants.GHAgentConfigCMName,
				},
				Data: map[string]string{
					"managedcluster":         "4s",             // valid
					"resync.managedcluster":  "not-a-duration", // invalid
					"managedhub.info":        "3s",             // valid
					"resync.managedhub.info": "35m",            // valid
					"nonexistent-key":        "10s",            // not used
				},
			}

			// Store original intervals
			originalResyncInterval := configmap.GetResyncInterval(enum.ManagedClusterType)

			// Update the ConfigMap
			Eventually(func() error {
				return runtimeClient.Update(ctx, configMap)
			}, 10*time.Second, time.Second).Should(Succeed())

			// Wait for reconciliation
			time.Sleep(2 * time.Second)

			// Verify valid intervals were updated
			Expect(configmap.GetSyncInterval(enum.ManagedClusterType)).To(Equal(4 * time.Second))
			Expect(configmap.GetSyncInterval(enum.HubClusterInfoType)).To(Equal(3 * time.Second))
			Expect(configmap.GetResyncInterval(enum.HubClusterInfoType)).To(Equal(35 * time.Minute))

			// Verify invalid interval was ignored
			Expect(configmap.GetResyncInterval(enum.ManagedClusterType)).To(Equal(originalResyncInterval))
		})
	})
})
