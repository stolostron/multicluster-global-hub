package tests

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

const (
	// Sync interval is 30s, so we need to wait a bit longer
	hubHASyncWait = 45 * time.Second
)

var _ = Describe("Hub HA Sync", Label("e2e-test-hubha"), Ordered, func() {
	var (
		activeHubName    string
		activeHubClient  client.Client
		standbyHubClient client.Client // This will be the global hub client (local agent)
		testNamespace    string
	)

	BeforeAll(func() {
		// Use hub1 as active and local agent on global hub as standby
		Expect(len(managedHubNames)).To(BeNumerically(">=", 1), "Hub HA tests require at least 1 managed hub")
		activeHubName = managedHubNames[0] // hub1
		testNamespace = "default"

		var err error
		activeHubClient, err = testClients.RuntimeClient(activeHubName, agentScheme)
		Expect(err).NotTo(HaveOccurred())

		// Standby hub is the global hub cluster itself (local agent)
		standbyHubClient = globalHubClient

		By(fmt.Sprintf("Configuring Hub HA: %s (active) -> global-hub local agent (standby)", activeHubName))

		// Enable local agent on global hub if not already enabled
		By("Enabling local agent on global hub as standby")
		Eventually(func() error {
			mgh := &globalhubv1alpha4.MulticlusterGlobalHub{}
			err := globalHubClient.Get(ctx, types.NamespacedName{
				Namespace: testOptions.GlobalHub.Namespace,
				Name:      "multiclusterglobalhub",
			}, mgh)
			if err != nil {
				return err
			}
			if !mgh.Spec.InstallAgentOnLocal {
				mgh.Spec.InstallAgentOnLocal = true
				return globalHubClient.Update(ctx, mgh)
			}
			return nil
		}, 3*time.Minute, 5*time.Second).Should(Succeed())

		// Wait for local agent to be deployed
		By("Waiting for local agent deployment on global hub")
		Eventually(func() error {
			return checkDeployAvailable(globalHubClient, testOptions.GlobalHub.Namespace, "multicluster-global-hub-agent")
		}, 5*time.Minute, 5*time.Second).Should(Succeed())

		// Check if hub roles are already configured (from previous test run or manual setup)
		currentActiveRole := getHubRoleLabel(ctx, globalHubClient, activeHubName)

		// Set active role on hub1
		if currentActiveRole != constants.GHHubRoleActive {
			By("Setting active hub role on hub1 managed cluster")
			Eventually(func() error {
				return setHubRole(ctx, globalHubClient, activeHubName, constants.GHHubRoleActive, "")
			}, 1*time.Minute, 5*time.Second).Should(Succeed())
		} else {
			By("Hub1 already has active role configured")
		}

		// Wait for agent ConfigMap to be updated with hub roles
		// The operator watches ManagedCluster label changes and propagates to agent ConfigMap
		// For local agent, the operator automatically configures it based on the hub topology
		// This typically takes 30-60 seconds: label change → addon annotation → ConfigMap update
		By("Waiting for active hub agent to receive role configuration")
		Eventually(func() string {
			return getAgentHubRole(ctx, activeHubClient, testOptions.GlobalHub.Namespace)
		}, 2*time.Minute, 5*time.Second).Should(Equal(constants.GHHubRoleActive))

		By("Waiting for local agent on global hub to receive standby role configuration")
		Eventually(func() string {
			return getAgentHubRole(ctx, globalHubClient, testOptions.GlobalHub.Namespace)
		}, 2*time.Minute, 5*time.Second).Should(Equal(constants.GHHubRoleStandby))
	})

	AfterAll(func() {
		// Note: We intentionally do NOT clean up hub roles here to allow for:
		// 1. Debugging - roles remain set for inspection after test failure
		// 2. Performance - subsequent test runs can reuse the configuration
		// 3. Stability - avoid unnecessary label churn
		//
		// To manually clean up hub role, run:
		// kubectl label managedcluster hub1 global-hub.open-cluster-management.io/hub-role-
		//
		// To disable local agent (standby):
		// kubectl patch mgh multiclusterglobalhub -n <namespace> --type merge -p '{"spec":{"installAgentOnLocal":false}}'
	})

	Context("Resource synchronization from active to standby hub", func() {
		var testSecretName string
		var testConfigMapName string

		BeforeEach(func() {
			testSecretName = fmt.Sprintf("hubha-test-secret-%d", time.Now().Unix())
			testConfigMapName = fmt.Sprintf("hubha-test-cm-%d", time.Now().Unix())
		})

		AfterEach(func() {
			// Clean up test resources from both hubs
			if testSecretName != "" {
				_ = activeHubClient.Delete(ctx, &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: testSecretName, Namespace: testNamespace},
				})
				_ = standbyHubClient.Delete(ctx, &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: testSecretName, Namespace: testNamespace},
				})
			}
			if testConfigMapName != "" {
				_ = activeHubClient.Delete(ctx, &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: testConfigMapName, Namespace: testNamespace},
				})
				_ = standbyHubClient.Delete(ctx, &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: testConfigMapName, Namespace: testNamespace},
				})
			}
		})

		It("should sync Secret with hive kubeconfig label from active to standby", func() {
			By("Creating Secret with hive kubeconfig label on active hub")
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testSecretName,
					Namespace: testNamespace,
					Labels: map[string]string{
						"hive.openshift.io/secret-type": "kubeconfig",
					},
				},
				Data: map[string][]byte{
					"kubeconfig": []byte("test-kubeconfig-data"),
				},
				Type: corev1.SecretTypeOpaque,
			}
			Expect(activeHubClient.Create(ctx, secret)).To(Succeed())
			klog.Infof("Created test secret %s on active hub", testSecretName)

			By("Verifying Secret is synced to standby hub")
			Eventually(func() error {
				standbySecret := &corev1.Secret{}
				if err := standbyHubClient.Get(ctx, types.NamespacedName{
					Name:      testSecretName,
					Namespace: testNamespace,
				}, standbySecret); err != nil {
					return fmt.Errorf("secret not found on standby hub: %w", err)
				}

				// Verify secret data
				if string(standbySecret.Data["kubeconfig"]) != "test-kubeconfig-data" {
					return fmt.Errorf("secret data mismatch on standby hub")
				}

				// Verify labels
				if standbySecret.Labels["hive.openshift.io/secret-type"] != "kubeconfig" {
					return fmt.Errorf("secret labels not synced correctly")
				}

				klog.Infof("Secret %s successfully synced to standby hub", testSecretName)
				return nil
			}, hubHASyncWait+30*time.Second, 5*time.Second).Should(Succeed())
		})

		It("should sync ConfigMap with hive kubeconfig label from active to standby", func() {
			By("Creating ConfigMap with hive kubeconfig label on active hub")
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testConfigMapName,
					Namespace: testNamespace,
					Labels: map[string]string{
						"hive.openshift.io/secret-type": "kubeconfig",
					},
				},
				Data: map[string]string{
					"config": "test-config-data",
				},
			}
			Expect(activeHubClient.Create(ctx, cm)).To(Succeed())
			klog.Infof("Created test ConfigMap %s on active hub", testConfigMapName)

			By("Verifying ConfigMap is synced to standby hub")
			Eventually(func() error {
				standbyCM := &corev1.ConfigMap{}
				if err := standbyHubClient.Get(ctx, types.NamespacedName{
					Name:      testConfigMapName,
					Namespace: testNamespace,
				}, standbyCM); err != nil {
					return fmt.Errorf("configmap not found on standby hub: %w", err)
				}

				// Verify data
				if standbyCM.Data["config"] != "test-config-data" {
					return fmt.Errorf("configmap data mismatch on standby hub")
				}

				klog.Infof("ConfigMap %s successfully synced to standby hub", testConfigMapName)
				return nil
			}, hubHASyncWait+30*time.Second, 5*time.Second).Should(Succeed())
		})

		It("should update synced Secret when modified on active hub", func() {
			By("Creating Secret on active hub")
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testSecretName,
					Namespace: testNamespace,
					Labels: map[string]string{
						"hive.openshift.io/secret-type": "kubeconfig",
					},
				},
				Data: map[string][]byte{
					"key": []byte("original-value"),
				},
				Type: corev1.SecretTypeOpaque,
			}
			Expect(activeHubClient.Create(ctx, secret)).To(Succeed())

			By("Waiting for initial sync to standby hub")
			Eventually(func() error {
				standbySecret := &corev1.Secret{}
				return standbyHubClient.Get(ctx, types.NamespacedName{
					Name:      testSecretName,
					Namespace: testNamespace,
				}, standbySecret)
			}, hubHASyncWait+30*time.Second, 5*time.Second).Should(Succeed())

			By("Updating Secret on active hub")
			Eventually(func() error {
				activeSecret := &corev1.Secret{}
				if err := activeHubClient.Get(ctx, types.NamespacedName{
					Name:      testSecretName,
					Namespace: testNamespace,
				}, activeSecret); err != nil {
					return err
				}
				activeSecret.Data["key"] = []byte("updated-value")
				return activeHubClient.Update(ctx, activeSecret)
			}, 1*time.Minute, 5*time.Second).Should(Succeed())

			By("Verifying Secret update is synced to standby hub")
			Eventually(func() string {
				standbySecret := &corev1.Secret{}
				if err := standbyHubClient.Get(ctx, types.NamespacedName{
					Name:      testSecretName,
					Namespace: testNamespace,
				}, standbySecret); err != nil {
					return ""
				}
				return string(standbySecret.Data["key"])
			}, hubHASyncWait+30*time.Second, 5*time.Second).Should(Equal("updated-value"))
		})

		It("should NOT sync Secret with velero exclude label", func() {
			excludedSecretName := fmt.Sprintf("excluded-secret-%d", time.Now().Unix())
			By("Creating Secret with velero exclude label on active hub")
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      excludedSecretName,
					Namespace: testNamespace,
					Labels: map[string]string{
						"hive.openshift.io/secret-type": "kubeconfig",
						"velero.io/exclude-from-backup": "true",
					},
				},
				Data: map[string][]byte{
					"key": []byte("should-not-sync"),
				},
				Type: corev1.SecretTypeOpaque,
			}
			Expect(activeHubClient.Create(ctx, secret)).To(Succeed())
			defer func() {
				_ = activeHubClient.Delete(ctx, secret)
			}()

			By("Verifying Secret is NOT synced to standby hub (should remain not found)")
			Consistently(func() bool {
				standbySecret := &corev1.Secret{}
				err := standbyHubClient.Get(ctx, types.NamespacedName{
					Name:      excludedSecretName,
					Namespace: testNamespace,
				}, standbySecret)
				// Should remain not found
				return err != nil
			}, hubHASyncWait, 5*time.Second).Should(BeTrue(), "Secret with velero exclude label should not be synced")
		})

		It("should NOT sync resources from non-included namespaces", func() {
			excludedNS := "kube-system"
			excludedSecretName := fmt.Sprintf("system-secret-%d", time.Now().Unix())

			By(fmt.Sprintf("Creating Secret in excluded namespace %s on active hub", excludedNS))
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      excludedSecretName,
					Namespace: excludedNS,
					Labels: map[string]string{
						"hive.openshift.io/secret-type": "kubeconfig",
					},
				},
				Data: map[string][]byte{
					"key": []byte("should-not-sync"),
				},
				Type: corev1.SecretTypeOpaque,
			}
			Expect(activeHubClient.Create(ctx, secret)).To(Succeed())
			defer func() {
				_ = activeHubClient.Delete(ctx, secret)
			}()

			By("Verifying Secret from kube-system is NOT synced to standby hub")
			Consistently(func() bool {
				standbySecret := &corev1.Secret{}
				err := standbyHubClient.Get(ctx, types.NamespacedName{
					Name:      excludedSecretName,
					Namespace: excludedNS,
				}, standbySecret)
				return err != nil
			}, hubHASyncWait, 5*time.Second).Should(BeTrue(), "Secrets from kube-system should not be synced")
		})
	})
})

// getHubRoleLabel retrieves the hub role label from a managed cluster
func getHubRoleLabel(ctx context.Context, c client.Client, clusterName string) string {
	cluster := &clusterv1.ManagedCluster{}
	if err := c.Get(ctx, types.NamespacedName{Name: clusterName}, cluster); err != nil {
		return ""
	}
	return cluster.Labels[constants.GHHubRoleLabelKey]
}

// setHubRole sets the hub role label on a managed cluster
func setHubRole(ctx context.Context, c client.Client, clusterName, role, _ string) error {
	cluster := &clusterv1.ManagedCluster{}
	if err := c.Get(ctx, types.NamespacedName{Name: clusterName}, cluster); err != nil {
		return err
	}

	if cluster.Labels == nil {
		cluster.Labels = make(map[string]string)
	}

	cluster.Labels[constants.GHHubRoleLabelKey] = role

	return c.Update(ctx, cluster)
}

// getAgentHubRole retrieves the hub role from agent ConfigMap
func getAgentHubRole(ctx context.Context, c client.Client, namespace string) string {
	cm := &corev1.ConfigMap{}
	if err := c.Get(ctx, types.NamespacedName{
		Name:      "multicluster-global-hub-agent-config",
		Namespace: namespace,
	}, cm); err != nil {
		return ""
	}
	return cm.Data["hubRole"]
}
