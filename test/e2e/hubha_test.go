package tests

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	// With list-watch pattern + immediate send, changes are detected and sent immediately
	// Wait time accounts for: controller event processing + Kafka transport + standby agent apply
	hubHASyncWait = 10 * time.Second
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

		// Wait for local agent to be deployed
		By("Waiting for local agent deployment on global hub")
		Eventually(func() error {
			return checkDeployAvailable(globalHubClient, testOptions.GlobalHub.Namespace, "multicluster-global-hub-agent")
		}, 3*time.Minute, 5*time.Second).Should(Succeed())

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
			return getAgentHubRole(ctx, activeHubClient, "multicluster-global-hub-agent")
		}, 2*time.Minute, 5*time.Second).Should(Equal(constants.GHHubRoleActive))

		By("Waiting for active hub agent to receive standby hub configuration")
		Eventually(func() string {
			return getStandByHub(ctx, activeHubClient, "multicluster-global-hub-agent")
		}, 2*time.Minute, 5*time.Second).Should(Equal("local-cluster"))

		By("Waiting for local agent on global hub to receive role configuration")
		Eventually(func() string {
			return getAgentHubRole(ctx, globalHubClient, testOptions.GlobalHub.Namespace)
		}, 2*time.Minute, 5*time.Second).Should(Equal(constants.GHHubRoleStandby))
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

			// The test environment doesn't have local-cluster ManagedCluster available in isolation
			// Core functionality is tested in integration tests
			XIt("should set hubAcceptsClient=true on failover and false on recovery", func() {
				// This test simulates hub failure and recovery by manipulating agent availability
				// Timing considerations:
				// - Hub management checks heartbeats every 2 minutes (ProbeDuration)
				// - Hubs are considered inactive if heartbeat > 5 minutes old (ActiveTimeout)
				// - After setting timestamp, we may need to wait up to 2 minutes for next check cycle
				// - After status change, Kafka message + agent processing adds ~30s-1min
				// - Total: Allow 4 minutes per state change detection + 2 minutes for agent update

				// Set up hub roles for this test (may have been cleaned up by previous tests)
				By("Configuring hub1 as active hub")
				Eventually(func() error {
					cluster := &clusterv1.ManagedCluster{}
					if err := globalHubClient.Get(ctx, types.NamespacedName{Name: activeHubName}, cluster); err != nil {
						return err
					}
					if cluster.Labels == nil {
						cluster.Labels = make(map[string]string)
					}
					cluster.Labels[constants.GHHubRoleLabelKey] = constants.GHHubRoleActive
					return globalHubClient.Update(ctx, cluster)
				}, 1*time.Minute, 5*time.Second).Should(Succeed())
				klog.Infof("Configured %s as active hub", activeHubName)

				By("Configuring local-cluster as standby hub")
				Eventually(func() error {
					cluster := &clusterv1.ManagedCluster{}
					if err := globalHubClient.Get(ctx, types.NamespacedName{Name: "local-cluster"}, cluster); err != nil {
						return err
					}
					if cluster.Labels == nil {
						cluster.Labels = make(map[string]string)
					}
					cluster.Labels[constants.GHHubRoleLabelKey] = constants.GHHubRoleStandby
					return globalHubClient.Update(ctx, cluster)
				}, 1*time.Minute, 5*time.Second).Should(Succeed())
				klog.Infof("Configured local-cluster as standby hub")

				// Wait for agent to receive the standby hub configuration
				time.Sleep(10 * time.Second)

				// Use existing real managed cluster from the database (hub1-cluster1)
				// This cluster is already reported to the database by the agent
				realClusterName := activeHubName + "-cluster1"
				klog.Infof("Using existing managed cluster %s for failover test", realClusterName)

				By("Creating a copy of the real ManagedCluster on standby hub for testing")
				// Get the real cluster from active hub
				activeCluster := &clusterv1.ManagedCluster{}
				Expect(activeHubClient.Get(ctx, types.NamespacedName{Name: realClusterName}, activeCluster)).To(Succeed())

				// Delete if it already exists on standby (cleanup from previous test runs)
				existingCluster := &clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: realClusterName,
					},
				}
				_ = standbyHubClient.Delete(ctx, existingCluster)
				time.Sleep(2 * time.Second) // Brief wait for deletion to complete

				// Create a copy on standby hub (simulating what Hub HA sync would do)
				// Start with hubAcceptsClient=false (normal state)
				standbyCluster := &clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:   realClusterName,
						Labels: activeCluster.Labels,
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient:            false, // Normal state - active hub is healthy
						ManagedClusterClientConfigs: activeCluster.Spec.ManagedClusterClientConfigs,
					},
				}
				Expect(standbyHubClient.Create(ctx, standbyCluster)).To(Succeed())
				klog.Infof("Created ManagedCluster %s on standby hub with hubAcceptsClient=false", realClusterName)

				// Verify the cluster exists in the database (it should, as it's a real cluster)
				var count int64
				var err error
				err = db.Raw("SELECT COUNT(*) FROM status.managed_clusters WHERE cluster_name = ? AND leaf_hub_name = ? AND deleted_at IS NULL",
					realClusterName, activeHubName).Scan(&count).Error
				Expect(err).NotTo(HaveOccurred())
				Expect(count).To(BeNumerically(">", 0), "ManagedCluster should exist in database")
				klog.Infof("Verified ManagedCluster %s exists in database", realClusterName)

				By("Simulating active hub failure by stopping the agent")
				// Hub management checks every 2 minutes (ProbeDuration), considers hub inactive if heartbeat > 5 minutes old (ActiveTimeout)
				// To simulate failure, scale down the agent so it stops sending heartbeats, then set old heartbeat timestamp

				// Scale down agent deployment to stop heartbeats
				_, err = testClients.Kubectl(activeHubName, "scale", "deployment",
					"multicluster-global-hub-agent", "-n", "multicluster-global-hub-agent", "--replicas=0")
				Expect(err).NotTo(HaveOccurred())
				klog.Infof("Scaled down %s agent to simulate hub failure", activeHubName)

				// Wait a bit to ensure agent is fully stopped and no more heartbeats are sent
				time.Sleep(30 * time.Second)

				// Now set the heartbeat to 6 minutes ago (past the 5 minute ActiveTimeout threshold)
				// Agent is stopped so it won't overwrite this timestamp
				oldHeartbeat := models.LeafHubHeartbeat{
					Name:         activeHubName,
					LastUpdateAt: time.Now().Add(-6 * time.Minute),
					Status:       constants.HubStatusActive, // Will be changed to inactive by hub management
				}
				Expect(oldHeartbeat.UpInsertHeartBeat(db)).To(Succeed())
				klog.Infof("Set %s heartbeat to 6 minutes ago (agent is stopped)", activeHubName)

				By("Waiting for hub management to detect inactive status and trigger failover")
				// Hub management runs every 2 minutes. Worst case: just missed a cycle, wait 2min + processing time
				// Allow 4 minutes to be safe: 2min for next cycle + 2min buffer for processing + message delivery
				Eventually(func() error {
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
				// After hub management detects inactive, it sends Kafka message to standby agent
				// Agent processes message and updates ManagedCluster - allow 2 minutes for this
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
				}, 2*time.Minute, 5*time.Second).Should(Succeed())

				By("Simulating active hub recovery by restarting the agent")
				// Scale agent deployment back up - it will start sending heartbeats again
				_, err = testClients.Kubectl(activeHubName, "scale", "deployment",
					"multicluster-global-hub-agent", "-n", "multicluster-global-hub-agent", "--replicas=1")
				Expect(err).NotTo(HaveOccurred())
				klog.Infof("Scaled up %s agent to simulate hub recovery", activeHubName)

				// Wait for agent to start and send heartbeat (agent sends heartbeat every 1 minute on startup)
				time.Sleep(90 * time.Second)

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
				// After hub management detects active, it sends Kafka message to standby agent
				// Agent processes message and updates ManagedCluster back to false
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
				}, 2*time.Minute, 5*time.Second).Should(Succeed())

				// Clean up - delete the test cluster from standby hub
				Expect(standbyHubClient.Delete(ctx, standbyCluster)).To(Succeed())
				klog.Infof("Cleaned up test ManagedCluster %s from standby hub", realClusterName)
			})

			It("should maintain hubAcceptsClient=false when ManagedCluster is updated on active hub", func() {
				By("Creating ManagedCluster on active hub")
				managedCluster := &clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: testClusterName,
						Labels: map[string]string{
							"hive.openshift.io/secret-type": "kubeconfig",
							"env":                           "test",
						},
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient: true,
						ManagedClusterClientConfigs: []clusterv1.ClientConfig{
							{
								URL: "https://test-cluster-v1.example.com:6443",
							},
						},
					},
				}
				Expect(activeHubClient.Create(ctx, managedCluster)).To(Succeed())

				By("Waiting for initial sync to standby hub")
				Eventually(func() error {
					standbyCluster := &clusterv1.ManagedCluster{}
					if err := standbyHubClient.Get(ctx, types.NamespacedName{Name: testClusterName}, standbyCluster); err != nil {
						return err
					}
					if standbyCluster.Spec.HubAcceptsClient != false {
						return fmt.Errorf("initial sync: expected hubAcceptsClient=false")
					}
					return nil
				}, hubHASyncWait+30*time.Second, 5*time.Second).Should(Succeed())

				By("Updating ManagedCluster on active hub (changing URL and labels)")
				Eventually(func() error {
					activeCluster := &clusterv1.ManagedCluster{}
					if err := activeHubClient.Get(ctx, types.NamespacedName{Name: testClusterName}, activeCluster); err != nil {
						return err
					}
					activeCluster.Labels["env"] = "production"
					activeCluster.Spec.ManagedClusterClientConfigs[0].URL = "https://test-cluster-v2.example.com:6443"
					return activeHubClient.Update(ctx, activeCluster)
				}, 1*time.Minute, 5*time.Second).Should(Succeed())

				By("Verifying update is synced with hubAcceptsClient still false")
				Eventually(func() error {
					standbyCluster := &clusterv1.ManagedCluster{}
					if err := standbyHubClient.Get(ctx, types.NamespacedName{Name: testClusterName}, standbyCluster); err != nil {
						return err
					}

					// hubAcceptsClient should remain false
					if standbyCluster.Spec.HubAcceptsClient != false {
						return fmt.Errorf("after update: expected hubAcceptsClient=false, got %v",
							standbyCluster.Spec.HubAcceptsClient)
					}

					// Verify updates were applied
					if standbyCluster.Labels["env"] != "production" {
						return fmt.Errorf("label update not synced, expected production, got %s",
							standbyCluster.Labels["env"])
					}
					if standbyCluster.Spec.ManagedClusterClientConfigs[0].URL != "https://test-cluster-v2.example.com:6443" {
						return fmt.Errorf("URL update not synced")
					}

					klog.Infof("ManagedCluster %s update synced correctly with hubAcceptsClient=false maintained", testClusterName)
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
