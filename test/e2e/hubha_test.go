package tests

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

const (
	// Wait time accounts for: controller event processing + Kafka transport + standby agent apply.
	// CI environments can be slow under load, so use a generous baseline.
	hubHASyncWait = 30 * time.Second
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
		}, 1*time.Minute, 5*time.Second).Should(Succeed())

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

		By("Waiting for active hub agent to receive role configuration")
		Eventually(func() string {
			return getAgentHubRole(ctx, activeHubClient, "multicluster-global-hub-agent")
		}, 3*time.Minute, 10*time.Second).Should(Equal(constants.GHHubRoleActive))

		By("Waiting for active hub agent to receive prefixed standby hub configuration")
		Eventually(func() string {
			return getStandByHub(ctx, activeHubClient, "multicluster-global-hub-agent")
		}, 3*time.Minute, 10*time.Second).Should(Equal("global-hub/local-cluster"),
			"active hub agent must receive the prefixed standbyHub value for global-hub local standby routing")

		By("Waiting for local agent on global hub to receive role configuration")
		Eventually(func() string {
			return getAgentHubRole(ctx, globalHubClient, testOptions.GlobalHub.Namespace)
		}, 3*time.Minute, 10*time.Second).Should(Equal(constants.GHHubRoleStandby))
	})

	AfterAll(func() {
		By("Cleaning up hub roles")
		// Remove hub role label from active hub
		Eventually(func() error {
			cluster := &clusterv1.ManagedCluster{}
			if err := globalHubClient.Get(ctx, types.NamespacedName{Name: activeHubName}, cluster); err != nil {
				return err
			}
			if cluster.Labels != nil {
				delete(cluster.Labels, constants.GHHubRoleLabelKey)
			}
			return globalHubClient.Update(ctx, cluster)
		}, 1*time.Minute, 5*time.Second).Should(Succeed())
		klog.Infof("Removed hub role label from %s", activeHubName)

		By("Disabling local agent on global hub")
		Eventually(func() error {
			mgh := &globalhubv1alpha4.MulticlusterGlobalHub{}
			err := globalHubClient.Get(ctx, types.NamespacedName{
				Namespace: testOptions.GlobalHub.Namespace,
				Name:      "multiclusterglobalhub",
			}, mgh)
			if err != nil {
				return err
			}
			if mgh.Spec.InstallAgentOnLocal {
				mgh.Spec.InstallAgentOnLocal = false
				return globalHubClient.Update(ctx, mgh)
			}
			return nil
		}, 1*time.Minute, 5*time.Second).Should(Succeed())
		klog.Infof("Disabled local agent on global hub")

		By("Waiting for local agent to be removed")
		Eventually(func() bool {
			deploy := &appsv1.Deployment{}
			err := globalHubClient.Get(ctx, types.NamespacedName{
				Name:      "multicluster-global-hub-agent",
				Namespace: testOptions.GlobalHub.Namespace,
			}, deploy)
			// Should be not found after cleanup
			return err != nil
		}, 2*time.Minute, 5*time.Second).Should(BeTrue())
		klog.Infof("Local agent deployment removed from global hub")
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

		It("should sync Policy from active to standby hub", func() {
			testPolicyName := fmt.Sprintf("hubha-test-policy-%d", time.Now().Unix())
			By("Creating Policy on active hub")
			policy := &policyv1.Policy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testPolicyName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						"policy.open-cluster-management.io/categories": "CM Configuration Management",
						"policy.open-cluster-management.io/standards":  "NIST SP 800-53",
					},
				},
				Spec: policyv1.PolicySpec{
					Disabled:          false,
					RemediationAction: policyv1.Inform,
					PolicyTemplates: []*policyv1.PolicyTemplate{
						{
							ObjectDefinition: runtime.RawExtension{
								Raw: []byte(`{
									"apiVersion": "policy.open-cluster-management.io/v1",
									"kind": "ConfigurationPolicy",
									"metadata": {
										"name": "test-config-policy"
									},
									"spec": {
										"remediationAction": "inform",
										"severity": "low",
										"object-templates": [{
											"complianceType": "musthave",
											"objectDefinition": {
												"apiVersion": "v1",
												"kind": "Namespace",
												"metadata": {
													"name": "test-namespace"
												}
											}
										}]
									}
								}`),
							},
						},
					},
				},
			}
			Expect(activeHubClient.Create(ctx, policy)).To(Succeed())
			klog.Infof("Created test policy %s on active hub", testPolicyName)

			defer func() {
				_ = activeHubClient.Delete(ctx, policy)
				_ = standbyHubClient.Delete(ctx, &policyv1.Policy{
					ObjectMeta: metav1.ObjectMeta{Name: testPolicyName, Namespace: testNamespace},
				})
			}()

			By("Verifying Policy is synced to standby hub")
			Eventually(func() error {
				standbyPolicy := &policyv1.Policy{}
				if err := standbyHubClient.Get(ctx, types.NamespacedName{
					Name:      testPolicyName,
					Namespace: testNamespace,
				}, standbyPolicy); err != nil {
					return fmt.Errorf("policy not found on standby hub: %w", err)
				}

				// Verify policy spec
				if standbyPolicy.Spec.RemediationAction != policyv1.Inform {
					return fmt.Errorf("policy remediation action mismatch, expected %s, got %s",
						policyv1.Inform, standbyPolicy.Spec.RemediationAction)
				}
				if standbyPolicy.Spec.Disabled {
					return fmt.Errorf("policy should not be disabled")
				}

				// Verify policy templates
				if len(standbyPolicy.Spec.PolicyTemplates) != 1 {
					return fmt.Errorf("expected 1 policy template, got %d", len(standbyPolicy.Spec.PolicyTemplates))
				}

				// Verify annotations
				if standbyPolicy.Annotations["policy.open-cluster-management.io/categories"] != "CM Configuration Management" {
					return fmt.Errorf("policy annotations not synced correctly")
				}

				klog.Infof("Policy %s successfully synced to standby hub", testPolicyName)
				return nil
			}, hubHASyncWait+30*time.Second, 5*time.Second).Should(Succeed())
		})

		It("should update synced Policy when modified on active hub", func() {
			testPolicyName := fmt.Sprintf("hubha-test-policy-update-%d", time.Now().Unix())
			By("Creating Policy on active hub")
			policy := &policyv1.Policy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testPolicyName,
					Namespace: testNamespace,
				},
				Spec: policyv1.PolicySpec{
					Disabled:          false,
					RemediationAction: policyv1.Inform,
					PolicyTemplates: []*policyv1.PolicyTemplate{
						{
							ObjectDefinition: runtime.RawExtension{
								Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"test"}}`),
							},
						},
					},
				},
			}
			Expect(activeHubClient.Create(ctx, policy)).To(Succeed())

			defer func() {
				_ = activeHubClient.Delete(ctx, policy)
				_ = standbyHubClient.Delete(ctx, &policyv1.Policy{
					ObjectMeta: metav1.ObjectMeta{Name: testPolicyName, Namespace: testNamespace},
				})
			}()

			By("Waiting for initial sync to standby hub")
			Eventually(func() error {
				standbyPolicy := &policyv1.Policy{}
				return standbyHubClient.Get(ctx, types.NamespacedName{
					Name:      testPolicyName,
					Namespace: testNamespace,
				}, standbyPolicy)
			}, hubHASyncWait+30*time.Second, 5*time.Second).Should(Succeed())

			By("Updating Policy on active hub")
			Eventually(func() error {
				activePolicy := &policyv1.Policy{}
				if err := activeHubClient.Get(ctx, types.NamespacedName{
					Name:      testPolicyName,
					Namespace: testNamespace,
				}, activePolicy); err != nil {
					return err
				}
				activePolicy.Spec.RemediationAction = policyv1.Enforce
				activePolicy.Spec.Disabled = true
				return activeHubClient.Update(ctx, activePolicy)
			}, 1*time.Minute, 5*time.Second).Should(Succeed())

			By("Verifying Policy update is synced to standby hub")
			Eventually(func() error {
				standbyPolicy := &policyv1.Policy{}
				if err := standbyHubClient.Get(ctx, types.NamespacedName{
					Name:      testPolicyName,
					Namespace: testNamespace,
				}, standbyPolicy); err != nil {
					return err
				}
				if standbyPolicy.Spec.RemediationAction != policyv1.Enforce {
					return fmt.Errorf("remediation action not updated, expected %s, got %s",
						policyv1.Enforce, standbyPolicy.Spec.RemediationAction)
				}
				if !standbyPolicy.Spec.Disabled {
					return fmt.Errorf("policy should be disabled")
				}
				return nil
			}, hubHASyncWait+30*time.Second, 5*time.Second).Should(Succeed())
		})

		It("should sync PlacementBinding from active to standby hub", func() {
			testPlacementBindingName := fmt.Sprintf("hubha-test-pb-%d", time.Now().Unix())
			testPolicyName := fmt.Sprintf("hubha-test-policy-pb-%d", time.Now().Unix())

			By("Creating Policy on active hub")
			policy := &policyv1.Policy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testPolicyName,
					Namespace: testNamespace,
				},
				Spec: policyv1.PolicySpec{
					Disabled:          false,
					RemediationAction: policyv1.Inform,
					PolicyTemplates: []*policyv1.PolicyTemplate{
						{
							ObjectDefinition: runtime.RawExtension{
								Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"test"}}`),
							},
						},
					},
				},
			}
			Expect(activeHubClient.Create(ctx, policy)).To(Succeed())

			By("Creating PlacementBinding on active hub")
			placementBinding := &policyv1.PlacementBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testPlacementBindingName,
					Namespace: testNamespace,
				},
				PlacementRef: policyv1.PlacementSubject{
					APIGroup: "cluster.open-cluster-management.io",
					Kind:     "Placement",
					Name:     "test-placement",
				},
				Subjects: []policyv1.Subject{
					{
						APIGroup: "policy.open-cluster-management.io",
						Kind:     "Policy",
						Name:     testPolicyName,
					},
				},
			}
			Expect(activeHubClient.Create(ctx, placementBinding)).To(Succeed())
			klog.Infof("Created test PlacementBinding %s on active hub", testPlacementBindingName)

			defer func() {
				_ = activeHubClient.Delete(ctx, placementBinding)
				_ = activeHubClient.Delete(ctx, policy)
				_ = standbyHubClient.Delete(ctx, &policyv1.PlacementBinding{
					ObjectMeta: metav1.ObjectMeta{Name: testPlacementBindingName, Namespace: testNamespace},
				})
				_ = standbyHubClient.Delete(ctx, &policyv1.Policy{
					ObjectMeta: metav1.ObjectMeta{Name: testPolicyName, Namespace: testNamespace},
				})
			}()

			By("Verifying PlacementBinding is synced to standby hub")
			Eventually(func() error {
				standbyPB := &policyv1.PlacementBinding{}
				if err := standbyHubClient.Get(ctx, types.NamespacedName{
					Name:      testPlacementBindingName,
					Namespace: testNamespace,
				}, standbyPB); err != nil {
					return fmt.Errorf("placementbinding not found on standby hub: %w", err)
				}

				// Verify PlacementBinding spec
				if standbyPB.PlacementRef.Name != "test-placement" {
					return fmt.Errorf("placement reference mismatch")
				}
				if len(standbyPB.Subjects) != 1 {
					return fmt.Errorf("expected 1 subject, got %d", len(standbyPB.Subjects))
				}
				if standbyPB.Subjects[0].Name != testPolicyName {
					return fmt.Errorf("subject policy name mismatch, expected %s, got %s",
						testPolicyName, standbyPB.Subjects[0].Name)
				}

				klog.Infof("PlacementBinding %s successfully synced to standby hub", testPlacementBindingName)
				return nil
			}, hubHASyncWait+30*time.Second, 5*time.Second).Should(Succeed())
		})

		It("should delete Secret from standby when deleted on active hub", func() {
			deleteTestSecretName := fmt.Sprintf("delete-test-secret-%d", time.Now().Unix())

			By("Creating Secret on active hub")
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      deleteTestSecretName,
					Namespace: testNamespace,
					Labels: map[string]string{
						"hive.openshift.io/secret-type": "kubeconfig",
					},
				},
				Data: map[string][]byte{
					"kubeconfig": []byte("test-delete-data"),
				},
				Type: corev1.SecretTypeOpaque,
			}
			Expect(activeHubClient.Create(ctx, secret)).To(Succeed())
			klog.Infof("Created test secret %s on active hub", deleteTestSecretName)

			By("Verifying Secret is synced to standby hub")
			Eventually(func() error {
				standbySecret := &corev1.Secret{}
				if err := standbyHubClient.Get(ctx, types.NamespacedName{
					Name:      deleteTestSecretName,
					Namespace: testNamespace,
				}, standbySecret); err != nil {
					return fmt.Errorf("secret not found on standby hub: %w", err)
				}
				klog.Infof("Secret %s successfully synced to standby hub", deleteTestSecretName)
				return nil
			}, hubHASyncWait+30*time.Second, 5*time.Second).Should(Succeed())

			By("Deleting Secret from active hub")
			Expect(activeHubClient.Delete(ctx, secret)).To(Succeed())
			klog.Infof("Deleted secret %s from active hub", deleteTestSecretName)

			By("Verifying Secret is deleted from standby hub")
			Eventually(func() bool {
				standbySecret := &corev1.Secret{}
				err := standbyHubClient.Get(ctx, types.NamespacedName{
					Name:      deleteTestSecretName,
					Namespace: testNamespace,
				}, standbySecret)
				// Should be not found after deletion
				if err != nil {
					klog.Infof("Secret %s successfully deleted from standby hub", deleteTestSecretName)
					return true
				}
				return false
			}, hubHASyncWait+30*time.Second, 5*time.Second).Should(BeTrue(), "Secret should be deleted from standby hub")
		})

		It("should delete ConfigMap from standby when deleted on active hub", func() {
			deleteTestCMName := fmt.Sprintf("delete-test-cm-%d", time.Now().Unix())

			By("Creating ConfigMap on active hub")
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      deleteTestCMName,
					Namespace: testNamespace,
					Labels: map[string]string{
						"hive.openshift.io/secret-type": "kubeconfig",
					},
				},
				Data: map[string]string{
					"config": "test-delete-data",
				},
			}
			Expect(activeHubClient.Create(ctx, cm)).To(Succeed())
			klog.Infof("Created test ConfigMap %s on active hub", deleteTestCMName)

			By("Verifying ConfigMap is synced to standby hub")
			Eventually(func() error {
				standbyCM := &corev1.ConfigMap{}
				if err := standbyHubClient.Get(ctx, types.NamespacedName{
					Name:      deleteTestCMName,
					Namespace: testNamespace,
				}, standbyCM); err != nil {
					return fmt.Errorf("configmap not found on standby hub: %w", err)
				}
				klog.Infof("ConfigMap %s successfully synced to standby hub", deleteTestCMName)
				return nil
			}, hubHASyncWait+30*time.Second, 5*time.Second).Should(Succeed())

			By("Deleting ConfigMap from active hub")
			Expect(activeHubClient.Delete(ctx, cm)).To(Succeed())
			klog.Infof("Deleted ConfigMap %s from active hub", deleteTestCMName)

			By("Verifying ConfigMap is deleted from standby hub")
			Eventually(func() bool {
				standbyCM := &corev1.ConfigMap{}
				err := standbyHubClient.Get(ctx, types.NamespacedName{
					Name:      deleteTestCMName,
					Namespace: testNamespace,
				}, standbyCM)
				// Should be not found after deletion
				if err != nil {
					klog.Infof("ConfigMap %s successfully deleted from standby hub", deleteTestCMName)
					return true
				}
				return false
			}, hubHASyncWait+30*time.Second, 5*time.Second).Should(BeTrue(), "ConfigMap should be deleted from standby hub")
		})

		It("should delete Policy from standby when deleted on active hub", func() {
			deleteTestPolicyName := fmt.Sprintf("delete-test-policy-%d", time.Now().Unix())

			By("Creating Policy on active hub")
			policy := &policyv1.Policy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      deleteTestPolicyName,
					Namespace: testNamespace,
				},
				Spec: policyv1.PolicySpec{
					RemediationAction: "inform",
					Disabled:          false,
					PolicyTemplates: []*policyv1.PolicyTemplate{
						{
							ObjectDefinition: runtime.RawExtension{
								Raw: []byte(`{
									"apiVersion": "policy.open-cluster-management.io/v1",
									"kind": "ConfigurationPolicy",
									"metadata": {
										"name": "delete-test-config-policy"
									},
									"spec": {
										"severity": "low"
									}
								}`),
							},
						},
					},
				},
			}
			Expect(activeHubClient.Create(ctx, policy)).To(Succeed())
			klog.Infof("Created test Policy %s on active hub", deleteTestPolicyName)

			By("Verifying Policy is synced to standby hub")
			Eventually(func() error {
				standbyPolicy := &policyv1.Policy{}
				if err := standbyHubClient.Get(ctx, types.NamespacedName{
					Name:      deleteTestPolicyName,
					Namespace: testNamespace,
				}, standbyPolicy); err != nil {
					return fmt.Errorf("policy not found on standby hub: %w", err)
				}
				klog.Infof("Policy %s successfully synced to standby hub", deleteTestPolicyName)
				return nil
			}, hubHASyncWait+30*time.Second, 5*time.Second).Should(Succeed())

			By("Deleting Policy from active hub")
			Expect(activeHubClient.Delete(ctx, policy)).To(Succeed())
			klog.Infof("Deleted Policy %s from active hub", deleteTestPolicyName)

			By("Verifying Policy is deleted from standby hub")
			Eventually(func() bool {
				standbyPolicy := &policyv1.Policy{}
				err := standbyHubClient.Get(ctx, types.NamespacedName{
					Name:      deleteTestPolicyName,
					Namespace: testNamespace,
				}, standbyPolicy)
				// Should be not found after deletion
				if err != nil {
					klog.Infof("Policy %s successfully deleted from standby hub", deleteTestPolicyName)
					return true
				}
				return false
			}, hubHASyncWait+30*time.Second, 5*time.Second).Should(BeTrue(), "Policy should be deleted from standby hub")
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

		Context("ManagedCluster hubAcceptsClient failover", func() {
			var testClusterName string

			BeforeEach(func() {
				testClusterName = fmt.Sprintf("hubha-test-cluster-%d", time.Now().Unix())
			})

			AfterEach(func() {
				// Clean up test ManagedCluster from both hubs
				if testClusterName != "" {
					_ = activeHubClient.Delete(ctx, &clusterv1.ManagedCluster{
						ObjectMeta: metav1.ObjectMeta{Name: testClusterName},
					})
					_ = standbyHubClient.Delete(ctx, &clusterv1.ManagedCluster{
						ObjectMeta: metav1.ObjectMeta{Name: testClusterName},
					})
				}
			})

			It("should sync ManagedCluster with hubAcceptsClient=false from active to standby", func() {
				By("Creating ManagedCluster on active hub with hubAcceptsClient=true")
				managedCluster := &clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: testClusterName,
						Labels: map[string]string{
							"hive.openshift.io/secret-type": "kubeconfig",
						},
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient: true, // Set to true on active hub
						ManagedClusterClientConfigs: []clusterv1.ClientConfig{
							{
								URL: "https://test-cluster.example.com:6443",
							},
						},
					},
				}
				Expect(activeHubClient.Create(ctx, managedCluster)).To(Succeed())
				klog.Infof("Created test ManagedCluster %s on active hub with hubAcceptsClient=true", testClusterName)

				By("Verifying ManagedCluster is synced to standby hub with hubAcceptsClient=false")
				Eventually(func() error {
					standbyCluster := &clusterv1.ManagedCluster{}
					if err := standbyHubClient.Get(ctx, types.NamespacedName{
						Name: testClusterName,
					}, standbyCluster); err != nil {
						return fmt.Errorf("managedcluster not found on standby hub: %w", err)
					}

					// Critical check: hubAcceptsClient should be false on standby
					// This ensures standby hub doesn't accept connections in normal state
					if standbyCluster.Spec.HubAcceptsClient != false {
						return fmt.Errorf("expected hubAcceptsClient=false on standby hub, got %v",
							standbyCluster.Spec.HubAcceptsClient)
					}

					// Verify other spec fields are synced
					if len(standbyCluster.Spec.ManagedClusterClientConfigs) == 0 {
						return fmt.Errorf("managedcluster client configs not synced")
					}
					if standbyCluster.Spec.ManagedClusterClientConfigs[0].URL != "https://test-cluster.example.com:6443" {
						return fmt.Errorf("managedcluster URL not synced correctly")
					}

					klog.Infof("ManagedCluster %s synced to standby with hubAcceptsClient=false (correct)", testClusterName)
					return nil
				}, hubHASyncWait+30*time.Second, 5*time.Second).Should(Succeed())
			})

			It("should set hubAcceptsClient=true on failover and false on recovery", func() {
				// This test simulates hub failure and recovery by manipulating agent availability
				// Timing considerations:
				// - Hub management checks heartbeats every 2 minutes (ProbeDuration)
				// - Hubs are considered inactive if heartbeat > 5 minutes old (ActiveTimeout)
				// - After setting timestamp, we may need to wait up to 2 minutes for next check cycle
				// - After status change, Kafka message + agent processing adds ~30s-1min
				// - Total: Allow 4 minutes per state change detection + 2 minutes for agent update

				// Ensure hub roles are configured (BeforeAll sets them, re-apply if cleaned up)
				By("Ensuring hub1 has active role")
				Eventually(func() error {
					return setHubRole(ctx, globalHubClient, activeHubName, constants.GHHubRoleActive, "")
				}, 1*time.Minute, 5*time.Second).Should(Succeed())
				klog.Infof("Ensured %s has active hub role", activeHubName)

				By("Waiting for agents to receive role configuration")
				Eventually(func() string {
					return getAgentHubRole(ctx, activeHubClient, "multicluster-global-hub-agent")
				}, 3*time.Minute, 10*time.Second).Should(Equal(constants.GHHubRoleActive))
				Eventually(func() string {
					return getAgentHubRole(ctx, globalHubClient, testOptions.GlobalHub.Namespace)
				}, 3*time.Minute, 10*time.Second).Should(Equal(constants.GHHubRoleStandby))

				realClusterName := activeHubName + "-cluster1"
				klog.Infof("Using existing managed cluster %s for failover test", realClusterName)

				By("Getting the real ManagedCluster from active hub")
				activeCluster := &clusterv1.ManagedCluster{}
				Expect(activeHubClient.Get(ctx, types.NamespacedName{Name: realClusterName}, activeCluster)).To(Succeed())

				By("Ensuring managed cluster exists in database for hub management failover")
				var count int64
				var err error
				err = db.Raw("SELECT COUNT(*) FROM status.managed_clusters WHERE cluster_name = ? AND leaf_hub_name = ? AND deleted_at IS NULL",
					realClusterName, activeHubName).Scan(&count).Error
				Expect(err).NotTo(HaveOccurred())
				if count == 0 {
					// The cluster may be absent because the agent syncer filters out clusters
					// without id.k8s.io ClusterClaim, or because a prior migration test triggered
					// a soft-delete. Insert a minimal row so hub management includes it in
					// failover status updates (cluster_name is generated from payload metadata).
					minPayload := fmt.Sprintf(`{"metadata":{"name":"%s"}}`, realClusterName)
					insertErr := db.Exec(
						`INSERT INTO status.managed_clusters (leaf_hub_name, cluster_id, payload, error) VALUES (?, ?, ?::jsonb, 'none')`,
						activeHubName, uuid.New().String(), minPayload,
					).Error
					Expect(insertErr).NotTo(HaveOccurred())
					DeferCleanup(func() {
						db.Exec("DELETE FROM status.managed_clusters WHERE cluster_name = ? AND leaf_hub_name = ?",
							realClusterName, activeHubName)
					})
					klog.Infof("Inserted ManagedCluster %s into database for failover test", realClusterName)
				}
				klog.Infof("Verified ManagedCluster %s exists in database", realClusterName)

				By("Creating a copy of the real ManagedCluster on standby hub for testing")
				existingCluster := &clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: realClusterName,
					},
				}
				_ = standbyHubClient.Delete(ctx, existingCluster)
				Eventually(func() error {
					return standbyHubClient.Get(ctx, types.NamespacedName{Name: realClusterName}, existingCluster)
				}, 30*time.Second, 1*time.Second).ShouldNot(Succeed())

				standbyCluster := &clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:   realClusterName,
						Labels: activeCluster.Labels,
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient:            false,
						ManagedClusterClientConfigs: activeCluster.Spec.ManagedClusterClientConfigs,
					},
				}
				Expect(standbyHubClient.Create(ctx, standbyCluster)).To(Succeed())
				DeferCleanup(func() {
					_ = standbyHubClient.Delete(ctx, standbyCluster)
				})
				klog.Infof("Created ManagedCluster %s on standby hub with hubAcceptsClient=false", realClusterName)

				By("Simulating active hub failure by stopping the agent")
				// Hub management checks every 2 minutes (ProbeDuration), considers hub inactive if heartbeat > 5 minutes old (ActiveTimeout)
				// To simulate failure, scale down the agent so it stops sending heartbeats, then set old heartbeat timestamp

				// Scale down agent deployment to stop heartbeats
				_, err = testClients.Kubectl(activeHubName, "scale", "deployment",
					"multicluster-global-hub-agent", "-n", "multicluster-global-hub-agent", "--replicas=0")
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(func() {
					_, _ = testClients.Kubectl(activeHubName, "scale", "deployment",
						"multicluster-global-hub-agent", "-n", "multicluster-global-hub-agent", "--replicas=1")
				})
				klog.Infof("Scaled down %s agent to simulate hub failure", activeHubName)

				Eventually(func() error {
					deploy := &appsv1.Deployment{}
					if err := activeHubClient.Get(ctx, types.NamespacedName{
						Name: "multicluster-global-hub-agent", Namespace: "multicluster-global-hub-agent",
					}, deploy); err != nil {
						return err
					}
					if deploy.Status.ReadyReplicas != 0 {
						return fmt.Errorf("agent still has %d ready replicas", deploy.Status.ReadyReplicas)
					}
					return nil
				}, 1*time.Minute, 5*time.Second).Should(Succeed())

				By("Waiting for hub management to detect inactive status and trigger failover")
				Eventually(func() error {
					staleHeartbeat := models.LeafHubHeartbeat{
						Name:         activeHubName,
						LastUpdateAt: time.Now().Add(-6 * time.Minute),
						Status:       constants.HubStatusActive,
					}
					if err := staleHeartbeat.UpInsertHeartBeat(db); err != nil {
						return fmt.Errorf("failed to set stale heartbeat: %w", err)
					}
					var heartbeat models.LeafHubHeartbeat
					if err := db.Where("leaf_hub_name = ?", activeHubName).First(&heartbeat).Error; err != nil {
						return err
					}
					if heartbeat.Status != constants.HubStatusInactive {
						return fmt.Errorf("hub status should be inactive, got %s (waiting for hub management cycle)", heartbeat.Status)
					}
					klog.Infof("Hub %s detected as inactive in database", activeHubName)
					return nil
				}, 4*time.Minute, 5*time.Second).Should(Succeed())

				By("Verifying hubAcceptsClient=true on standby (failover triggered)")
				Eventually(func() error {
					updatedCluster := &clusterv1.ManagedCluster{}
					if err := standbyHubClient.Get(ctx, types.NamespacedName{Name: realClusterName}, updatedCluster); err != nil {
						return err
					}
					if updatedCluster.Spec.HubAcceptsClient != true {
						return fmt.Errorf("expected hubAcceptsClient=true during failover, got %v (waiting for agent to process message)",
							updatedCluster.Spec.HubAcceptsClient)
					}
					klog.Infof("ManagedCluster %s has hubAcceptsClient=true (failover successful)", realClusterName)
					return nil
				}, 4*time.Minute, 5*time.Second).Should(Succeed())

				By("Simulating active hub recovery by restarting the agent")
				// Scale agent deployment back up - it will start sending heartbeats again
				_, err = testClients.Kubectl(activeHubName, "scale", "deployment",
					"multicluster-global-hub-agent", "-n", "multicluster-global-hub-agent", "--replicas=1")
				Expect(err).NotTo(HaveOccurred())
				klog.Infof("Scaled up %s agent to simulate hub recovery", activeHubName)

				Eventually(func() error {
					deploy := &appsv1.Deployment{}
					if err := activeHubClient.Get(ctx, types.NamespacedName{
						Name: "multicluster-global-hub-agent", Namespace: "multicluster-global-hub-agent",
					}, deploy); err != nil {
						return err
					}
					if deploy.Status.ReadyReplicas < 1 {
						return fmt.Errorf("agent has %d ready replicas, waiting for 1", deploy.Status.ReadyReplicas)
					}
					return nil
				}, 2*time.Minute, 5*time.Second).Should(Succeed())

				By("Waiting for hub management to detect active status recovery")
				// Same timing: hub management cycle (2min) + processing buffer
				Eventually(func() error {
					var heartbeat models.LeafHubHeartbeat
					if err := db.Where("leaf_hub_name = ?", activeHubName).First(&heartbeat).Error; err != nil {
						return err
					}
					if heartbeat.Status != constants.HubStatusActive {
						return fmt.Errorf("hub status should be active after recovery, got %s (waiting for hub management cycle)", heartbeat.Status)
					}
					klog.Infof("Hub %s detected as active in database (recovered)", activeHubName)
					return nil
				}, 4*time.Minute, 5*time.Second).Should(Succeed())

				By("Verifying hubAcceptsClient=false on standby (back to normal state)")
				Eventually(func() error {
					recoveredCluster := &clusterv1.ManagedCluster{}
					if err := standbyHubClient.Get(ctx, types.NamespacedName{Name: realClusterName}, recoveredCluster); err != nil {
						return err
					}
					if recoveredCluster.Spec.HubAcceptsClient != false {
						return fmt.Errorf("expected hubAcceptsClient=false after recovery, got %v (waiting for agent to process message)",
							recoveredCluster.Spec.HubAcceptsClient)
					}
					klog.Infof("ManagedCluster %s has hubAcceptsClient=false (back to normal)", realClusterName)
					return nil
				}, 4*time.Minute, 5*time.Second).Should(Succeed())
			})
		})

		Context("Post-failover workload validation", Ordered, func() {
			var realClusterName string

			BeforeAll(func() {
				realClusterName = activeHubName + "-cluster1"

				By("Ensuring hub roles are configured for failover test")
				Eventually(func() error {
					return setHubRole(ctx, globalHubClient, activeHubName, constants.GHHubRoleActive, "")
				}, 1*time.Minute, 5*time.Second).Should(Succeed())

				By("Waiting for agent to receive role configuration")
				Eventually(func() string {
					return getAgentHubRole(ctx, activeHubClient, "multicluster-global-hub-agent")
				}, 3*time.Minute, 10*time.Second).Should(Equal(constants.GHHubRoleActive),
					"active hub agent must receive the active role before failover is simulated")

				Eventually(func() string {
					return getAgentHubRole(ctx, globalHubClient, testOptions.GlobalHub.Namespace)
				}, 3*time.Minute, 10*time.Second).Should(Equal(constants.GHHubRoleStandby),
					"standby agent must receive the standby role before failover is simulated")

				By("Ensuring managed cluster exists in database for failover")
				var count int64
				err := db.Raw("SELECT COUNT(*) FROM status.managed_clusters WHERE cluster_name = ? AND leaf_hub_name = ? AND deleted_at IS NULL",
					realClusterName, activeHubName).Scan(&count).Error
				Expect(err).NotTo(HaveOccurred())
				if count == 0 {
					minPayload := fmt.Sprintf(`{"metadata":{"name":"%s"}}`, realClusterName)
					insertErr := db.Exec(
						`INSERT INTO status.managed_clusters (leaf_hub_name, cluster_id, payload, error) VALUES (?, ?, ?::jsonb, 'none')`,
						activeHubName, uuid.New().String(), minPayload,
					).Error
					Expect(insertErr).NotTo(HaveOccurred())
					DeferCleanup(func() {
						db.Exec("DELETE FROM status.managed_clusters WHERE cluster_name = ? AND leaf_hub_name = ?",
							realClusterName, activeHubName)
					})
					klog.Infof("Inserted ManagedCluster %s into database for failover tests", realClusterName)
				}
			})

			It("should preserve Policies on standby hub through failover and recovery", func() {
				failoverPolicyName := fmt.Sprintf("failover-policy-%d", time.Now().Unix())

				By("Creating Policy on active hub before failover")
				policy := &policyv1.Policy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      failoverPolicyName,
						Namespace: testNamespace,
						Annotations: map[string]string{
							"policy.open-cluster-management.io/categories": "CM Configuration Management",
							"policy.open-cluster-management.io/standards":  "NIST SP 800-53",
						},
					},
					Spec: policyv1.PolicySpec{
						Disabled:          false,
						RemediationAction: policyv1.Inform,
						PolicyTemplates: []*policyv1.PolicyTemplate{
							{
								ObjectDefinition: runtime.RawExtension{
									Raw: []byte(`{
										"apiVersion": "policy.open-cluster-management.io/v1",
										"kind": "ConfigurationPolicy",
										"metadata": { "name": "failover-config-policy" },
										"spec": {
											"remediationAction": "inform",
											"severity": "high",
											"object-templates": [{
												"complianceType": "musthave",
												"objectDefinition": {
													"apiVersion": "v1",
													"kind": "Namespace",
													"metadata": { "name": "failover-test-ns" }
												}
											}]
										}
									}`),
								},
							},
						},
					},
				}
				Expect(activeHubClient.Create(ctx, policy)).To(Succeed())
				klog.Infof("Created failover test Policy %s on active hub", failoverPolicyName)

				defer func() {
					_ = activeHubClient.Delete(ctx, policy)
					_ = standbyHubClient.Delete(ctx, &policyv1.Policy{
						ObjectMeta: metav1.ObjectMeta{Name: failoverPolicyName, Namespace: testNamespace},
					})
				}()

				By("Verifying Policy is synced to standby hub before failover")
				Eventually(func() error {
					standbyPolicy := &policyv1.Policy{}
					return standbyHubClient.Get(ctx, types.NamespacedName{
						Name: failoverPolicyName, Namespace: testNamespace,
					}, standbyPolicy)
				}, hubHASyncWait+30*time.Second, 5*time.Second).Should(Succeed())

				By("Creating ManagedCluster on standby for failover test")
				activeCluster := &clusterv1.ManagedCluster{}
				Expect(activeHubClient.Get(ctx, types.NamespacedName{Name: realClusterName}, activeCluster)).To(Succeed())

				_ = standbyHubClient.Delete(ctx, &clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{Name: realClusterName},
				})
				Eventually(func() bool {
					err := standbyHubClient.Get(ctx, types.NamespacedName{Name: realClusterName},
						&clusterv1.ManagedCluster{})
					return err != nil
				}, 30*time.Second, 2*time.Second).Should(BeTrue(), "ManagedCluster deletion should complete")

				standbyCluster := &clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:   realClusterName,
						Labels: activeCluster.Labels,
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient:            false,
						ManagedClusterClientConfigs: activeCluster.Spec.ManagedClusterClientConfigs,
					},
				}
				Expect(standbyHubClient.Create(ctx, standbyCluster)).To(Succeed())
				DeferCleanup(func() {
					_ = standbyHubClient.Delete(ctx, standbyCluster)
				})

				By("Simulating active hub failure")
				_, err := testClients.Kubectl(activeHubName, "scale", "deployment",
					"multicluster-global-hub-agent", "-n", "multicluster-global-hub-agent", "--replicas=0")
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(func() {
					_, _ = testClients.Kubectl(activeHubName, "scale", "deployment",
						"multicluster-global-hub-agent", "-n", "multicluster-global-hub-agent", "--replicas=1")
				})
				Eventually(func() int32 {
					deploy := &appsv1.Deployment{}
					if err := activeHubClient.Get(ctx, types.NamespacedName{
						Name:      "multicluster-global-hub-agent",
						Namespace: "multicluster-global-hub-agent",
					}, deploy); err != nil {
						return -1
					}
					return deploy.Status.ReadyReplicas
				}, 1*time.Minute, 5*time.Second).Should(Equal(int32(0)),
					"Agent deployment should have 0 ready replicas")

				By("Waiting for failover detection")
				Eventually(func() error {
					staleHeartbeat := models.LeafHubHeartbeat{
						Name:         activeHubName,
						LastUpdateAt: time.Now().Add(-6 * time.Minute),
						Status:       constants.HubStatusActive,
					}
					if err := staleHeartbeat.UpInsertHeartBeat(db); err != nil {
						return fmt.Errorf("failed to set stale heartbeat: %w", err)
					}
					var heartbeat models.LeafHubHeartbeat
					if err := db.Where("leaf_hub_name = ?", activeHubName).First(&heartbeat).Error; err != nil {
						return err
					}
					if heartbeat.Status != constants.HubStatusInactive {
						return fmt.Errorf("hub status should be inactive, got %s", heartbeat.Status)
					}
					return nil
				}, 4*time.Minute, 5*time.Second).Should(Succeed())

				By("Verifying hubAcceptsClient=true on standby (failover triggered)")
				Eventually(func() error {
					updatedCluster := &clusterv1.ManagedCluster{}
					if err := standbyHubClient.Get(ctx, types.NamespacedName{Name: realClusterName}, updatedCluster); err != nil {
						return err
					}
					if !updatedCluster.Spec.HubAcceptsClient {
						return fmt.Errorf("expected hubAcceptsClient=true during failover")
					}
					return nil
				}, 4*time.Minute, 5*time.Second).Should(Succeed())

				By("Verifying Policy still exists and is intact on standby hub post-failover")
				standbyPolicy := &policyv1.Policy{}
				err = standbyHubClient.Get(ctx, types.NamespacedName{
					Name: failoverPolicyName, Namespace: testNamespace,
				}, standbyPolicy)
				Expect(err).NotTo(HaveOccurred(), "Policy should survive failover on standby hub")
				Expect(standbyPolicy.Spec.RemediationAction).To(Equal(policyv1.Inform),
					"Policy spec should be preserved post-failover")
				Expect(standbyPolicy.Spec.Disabled).To(BeFalse(),
					"Policy should remain enabled post-failover")
				Expect(standbyPolicy.Annotations["policy.open-cluster-management.io/standards"]).To(
					Equal("NIST SP 800-53"), "Policy annotations should be preserved",
				)
				klog.Infof("Policy %s verified intact post-failover", failoverPolicyName)

				By("Simulating active hub recovery")
				_, err = testClients.Kubectl(activeHubName, "scale", "deployment",
					"multicluster-global-hub-agent", "-n", "multicluster-global-hub-agent", "--replicas=1")
				Expect(err).NotTo(HaveOccurred())
				Eventually(func() int32 {
					deploy := &appsv1.Deployment{}
					if err := activeHubClient.Get(ctx, types.NamespacedName{
						Name:      "multicluster-global-hub-agent",
						Namespace: "multicluster-global-hub-agent",
					}, deploy); err != nil {
						return 0
					}
					return deploy.Status.ReadyReplicas
				}, 2*time.Minute, 5*time.Second).Should(BeNumerically(">=", int32(1)),
					"Agent deployment should have at least 1 ready replica")

				By("Waiting for recovery detection")
				Eventually(func() error {
					var heartbeat models.LeafHubHeartbeat
					if err := db.Where("leaf_hub_name = ?", activeHubName).First(&heartbeat).Error; err != nil {
						return err
					}
					if heartbeat.Status != constants.HubStatusActive {
						return fmt.Errorf("hub status should be active, got %s", heartbeat.Status)
					}
					return nil
				}, 4*time.Minute, 5*time.Second).Should(Succeed())

				By("Verifying hubAcceptsClient=false on standby (recovery)")
				Eventually(func() error {
					recoveredCluster := &clusterv1.ManagedCluster{}
					if err := standbyHubClient.Get(ctx, types.NamespacedName{Name: realClusterName}, recoveredCluster); err != nil {
						return err
					}
					if recoveredCluster.Spec.HubAcceptsClient {
						return fmt.Errorf("expected hubAcceptsClient=false after recovery")
					}
					return nil
				}, 4*time.Minute, 5*time.Second).Should(Succeed())

				By("Verifying Policy still exists post-recovery")
				err = standbyHubClient.Get(ctx, types.NamespacedName{
					Name: failoverPolicyName, Namespace: testNamespace,
				}, standbyPolicy)
				Expect(err).NotTo(HaveOccurred(), "Policy should survive recovery on standby hub")
				klog.Infof("Policy %s verified intact post-recovery", failoverPolicyName)
			})

			// Sync-verification tests: these validate the active→standby data path that
			// enables workload availability post-failover. The full failover cycle is
			// exercised by the Policy test above; these verify additional resource types
			// are synced correctly, which is the prerequisite for post-failover continuity.
			It("should sync PlacementBinding from active to standby hub", func() {
				failoverPBName := fmt.Sprintf("failover-pb-%d", time.Now().Unix())
				failoverPolicyName := fmt.Sprintf("failover-pb-policy-%d", time.Now().Unix())

				By("Creating Policy and PlacementBinding on active hub")
				policy := &policyv1.Policy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      failoverPolicyName,
						Namespace: testNamespace,
					},
					Spec: policyv1.PolicySpec{
						Disabled:          false,
						RemediationAction: policyv1.Enforce,
						PolicyTemplates: []*policyv1.PolicyTemplate{
							{
								ObjectDefinition: runtime.RawExtension{
									Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"test"}}`),
								},
							},
						},
					},
				}
				Expect(activeHubClient.Create(ctx, policy)).To(Succeed())

				pb := &policyv1.PlacementBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      failoverPBName,
						Namespace: testNamespace,
					},
					PlacementRef: policyv1.PlacementSubject{
						APIGroup: "cluster.open-cluster-management.io",
						Kind:     "Placement",
						Name:     "failover-placement",
					},
					Subjects: []policyv1.Subject{
						{
							APIGroup: "policy.open-cluster-management.io",
							Kind:     "Policy",
							Name:     failoverPolicyName,
						},
					},
				}
				Expect(activeHubClient.Create(ctx, pb)).To(Succeed())
				klog.Infof("Created PlacementBinding %s on active hub", failoverPBName)

				defer func() {
					_ = activeHubClient.Delete(ctx, pb)
					_ = activeHubClient.Delete(ctx, policy)
					_ = standbyHubClient.Delete(ctx, &policyv1.PlacementBinding{
						ObjectMeta: metav1.ObjectMeta{Name: failoverPBName, Namespace: testNamespace},
					})
					_ = standbyHubClient.Delete(ctx, &policyv1.Policy{
						ObjectMeta: metav1.ObjectMeta{Name: failoverPolicyName, Namespace: testNamespace},
					})
				}()

				By("Verifying PlacementBinding is synced to standby")
				Eventually(func() error {
					standbyPB := &policyv1.PlacementBinding{}
					return standbyHubClient.Get(ctx, types.NamespacedName{
						Name: failoverPBName, Namespace: testNamespace,
					}, standbyPB)
				}, hubHASyncWait+30*time.Second, 5*time.Second).Should(Succeed())

				By("Verifying PlacementBinding spec is preserved on standby")
				standbyPB := &policyv1.PlacementBinding{}
				err := standbyHubClient.Get(ctx, types.NamespacedName{
					Name: failoverPBName, Namespace: testNamespace,
				}, standbyPB)
				Expect(err).NotTo(HaveOccurred(), "PlacementBinding should exist on standby hub")
				Expect(standbyPB.PlacementRef.Name).To(Equal("failover-placement"),
					"PlacementRef name should be preserved on standby")
				Expect(standbyPB.Subjects).To(HaveLen(1),
					"PlacementBinding should have exactly one subject")
				Expect(standbyPB.Subjects[0].Name).To(Equal(failoverPolicyName),
					"PlacementBinding subject should reference the correct policy")
				klog.Infof("PlacementBinding %s verified on standby", failoverPBName)
			})

			It("should sync Argo Application from active to standby hub", func() {
				argoAppName := fmt.Sprintf("failover-argoapp-%d", time.Now().Unix())

				By("Creating Argo Application on active hub (unstructured)")
				argoApp := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "argoproj.io/v1alpha1",
						"kind":       "Application",
						"metadata": map[string]interface{}{
							"name":      argoAppName,
							"namespace": testNamespace,
						},
						"spec": map[string]interface{}{
							"project": "default",
							"source": map[string]interface{}{
								"repoURL":        "https://github.com/example/app.git",
								"targetRevision": "HEAD",
								"path":           "manifests",
							},
							"destination": map[string]interface{}{
								"server":    "https://kubernetes.default.svc",
								"namespace": "default",
							},
						},
					},
				}

				err := activeHubClient.Create(ctx, argoApp)
				if err != nil {
					if meta.IsNoMatchError(err) {
						klog.Infof("Argo Application CRD not available on active hub: %v", err)
						Skip("Argo Application CRD not installed on active hub")
					}
					Expect(err).NotTo(HaveOccurred(), "unexpected error creating Argo Application")
				}
				klog.Infof("Created Argo Application %s on active hub", argoAppName)

				defer func() {
					_ = activeHubClient.Delete(ctx, argoApp)
					standbyArgo := &unstructured.Unstructured{}
					standbyArgo.SetAPIVersion("argoproj.io/v1alpha1")
					standbyArgo.SetKind("Application")
					standbyArgo.SetName(argoAppName)
					standbyArgo.SetNamespace(testNamespace)
					_ = standbyHubClient.Delete(ctx, standbyArgo)
				}()

				By("Verifying Argo Application is synced to standby hub")
				Eventually(func() error {
					standbyArgo := &unstructured.Unstructured{}
					standbyArgo.SetAPIVersion("argoproj.io/v1alpha1")
					standbyArgo.SetKind("Application")
					if err := standbyHubClient.Get(ctx, types.NamespacedName{
						Name: argoAppName, Namespace: testNamespace,
					}, standbyArgo); err != nil {
						return err
					}

					spec, ok := standbyArgo.Object["spec"].(map[string]interface{})
					if !ok {
						return fmt.Errorf("argo app spec not found")
					}
					project, _ := spec["project"].(string)
					if project != "default" {
						return fmt.Errorf("expected project=default, got %s", project)
					}

					source, ok := spec["source"].(map[string]interface{})
					if !ok {
						return fmt.Errorf("argo app source not found")
					}
					repoURL, _ := source["repoURL"].(string)
					if repoURL != "https://github.com/example/app.git" {
						return fmt.Errorf("expected repoURL match, got %s", repoURL)
					}

					klog.Infof("Argo Application %s verified on standby hub", argoAppName)
					return nil
				}, hubHASyncWait+30*time.Second, 5*time.Second).Should(Succeed())
			})

			It("should sync ClusterInstance from active to standby hub", func() {
				ciName := fmt.Sprintf("failover-ci-%d", time.Now().Unix())

				By("Creating ClusterInstance on active hub (unstructured)")
				ci := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "siteconfig.open-cluster-management.io/v1alpha1",
						"kind":       "ClusterInstance",
						"metadata": map[string]interface{}{
							"name":      ciName,
							"namespace": testNamespace,
						},
						"spec": map[string]interface{}{
							"clusterName":    ciName,
							"baseDomain":     "example.com",
							"clusterNetwork": []interface{}{},
							"machineNetwork": []interface{}{},
						},
					},
				}

				err := activeHubClient.Create(ctx, ci)
				if err != nil {
					if meta.IsNoMatchError(err) {
						klog.Infof("ClusterInstance CRD not available on active hub: %v", err)
						Skip("ClusterInstance CRD not installed on active hub")
					}
					Expect(err).NotTo(HaveOccurred(), "unexpected error creating ClusterInstance")
				}
				klog.Infof("Created ClusterInstance %s on active hub", ciName)

				defer func() {
					_ = activeHubClient.Delete(ctx, ci)
					standbyCI := &unstructured.Unstructured{}
					standbyCI.SetAPIVersion("siteconfig.open-cluster-management.io/v1alpha1")
					standbyCI.SetKind("ClusterInstance")
					standbyCI.SetName(ciName)
					standbyCI.SetNamespace(testNamespace)
					_ = standbyHubClient.Delete(ctx, standbyCI)
				}()

				By("Verifying ClusterInstance is synced to standby hub")
				Eventually(func() error {
					standbyCI := &unstructured.Unstructured{}
					standbyCI.SetAPIVersion("siteconfig.open-cluster-management.io/v1alpha1")
					standbyCI.SetKind("ClusterInstance")
					if err := standbyHubClient.Get(ctx, types.NamespacedName{
						Name: ciName, Namespace: testNamespace,
					}, standbyCI); err != nil {
						return err
					}

					spec, ok := standbyCI.Object["spec"].(map[string]interface{})
					if !ok {
						return fmt.Errorf("clusterinstance spec not found")
					}
					clusterName, _ := spec["clusterName"].(string)
					if clusterName != ciName {
						return fmt.Errorf("expected clusterName=%s, got %s", ciName, clusterName)
					}
					baseDomain, _ := spec["baseDomain"].(string)
					if baseDomain != "example.com" {
						return fmt.Errorf("expected baseDomain=example.com, got %s", baseDomain)
					}

					klog.Infof("ClusterInstance %s verified on standby hub", ciName)
					return nil
				}, hubHASyncWait+30*time.Second, 5*time.Second).Should(Succeed())
			})
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

// getStandByHub retrieves the standby hub from agent ConfigMap
func getStandByHub(ctx context.Context, c client.Client, namespace string) string {
	cm := &corev1.ConfigMap{}
	if err := c.Get(ctx, types.NamespacedName{
		Name:      "multicluster-global-hub-agent-config",
		Namespace: namespace,
	}, cm); err != nil {
		return ""
	}
	return cm.Data["standbyHub"]
}
