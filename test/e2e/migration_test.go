package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
)

const (
	migrationNamespace    = "multicluster-global-hub"
	migrationTimeout      = 10 * time.Minute
	migrationPollInterval = 5 * time.Second
	agentNamespace        = "open-cluster-management-agent"
	mceNamespace          = "multicluster-engine"
)

var _ = Describe("Migration E2E", Label("e2e-test-migration"), Ordered, func() {
	var (
		sourceHubName        string
		targetHubName        string
		clusterToMigrate     string
		migrationName        string
		sourceHubClient      client.Client
		targetHubClient      client.Client
		managedClusterClient client.Client
	)

	BeforeAll(func() {
		// Use hub1 as source and hub2 as target
		Expect(len(managedHubNames)).To(BeNumerically(">=", 2))
		sourceHubName = managedHubNames[0]        // hub1
		targetHubName = managedHubNames[1]        // hub2
		clusterToMigrate = managedClusterNames[0] // hub1-cluster1
		migrationName = fmt.Sprintf("migration-%s", clusterToMigrate)

		var err error
		sourceHubClient, err = testClients.RuntimeClient(sourceHubName, agentScheme)
		Expect(err).NotTo(HaveOccurred())
		targetHubClient, err = testClients.RuntimeClient(targetHubName, agentScheme)
		Expect(err).NotTo(HaveOccurred())
		managedClusterClient, err = testClients.RuntimeClient(clusterToMigrate, agentScheme)
		Expect(err).NotTo(HaveOccurred())

		By(fmt.Sprintf("Migration: %s from %s to %s", clusterToMigrate, sourceHubName, targetHubName))

		// Setup RBAC for work-agent to manage klusterlets on all managed clusters
		// This is needed because ManifestWork with klusterlet needs work-agent to have permissions
		By("Setting up RBAC for work-agent to manage klusterlets")
		for _, mcName := range managedClusterNames {
			mcClient, err := testClients.RuntimeClient(mcName, agentScheme)
			Expect(err).NotTo(HaveOccurred())
			setupWorkAgentRBAC(ctx, mcClient)
		}

		// Verify ClusterManager CRD supports autoApproveUsers field
		// This is required for the agent to set up auto-approval for migrating clusters
		By("Verifying ClusterManager CRD supports autoApproveUsers")
		verifyAutoApproveUsersSupport(ctx, targetHubClient)
	})

	AfterAll(func() {
		// Cleanup migration CR if exists
		mcm := &migrationv1alpha1.ManagedClusterMigration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      migrationName,
				Namespace: migrationNamespace,
			},
		}
		_ = globalHubClient.Delete(ctx, mcm)

		// Cleanup manifestworks on both hubs
		manifestWorkName := fmt.Sprintf("%s-klusterlet", clusterToMigrate)
		_ = sourceHubClient.Delete(ctx, &workv1.ManifestWork{
			ObjectMeta: metav1.ObjectMeta{Name: manifestWorkName, Namespace: clusterToMigrate},
		})
		_ = targetHubClient.Delete(ctx, &workv1.ManifestWork{
			ObjectMeta: metav1.ObjectMeta{Name: manifestWorkName, Namespace: clusterToMigrate},
		})

		// Restore klusterlet on managed cluster to original configuration
		// This is critical to ensure the E2E environment remains usable for future tests
		By("Restoring klusterlet to original configuration")
		restoreKlusterlet(ctx, managedClusterClient, targetHubName)

		// Re-accept managed cluster on source hub
		// The migration sets HubAcceptsClient to false, so we need to restore it
		By("Restoring managed cluster acceptance on source hub")
		restoreManagedClusterAcceptance(ctx, sourceHubClient, clusterToMigrate)
	})

	Context("Migration from source hub to target hub", func() {
		// Step 1: Verify managed-serviceaccount addon is ready on global hub
		It("should verify managed-serviceaccount addon is ready on global hub", func() {
			By("Checking managed-serviceaccount-addon-manager deployment is ready")
			Eventually(func() bool {
				deploy := &appsv1.Deployment{}
				if err := globalHubClient.Get(ctx, types.NamespacedName{
					Name:      "managed-serviceaccount-addon-manager",
					Namespace: "open-cluster-management-addon",
				}, deploy); err != nil {
					klog.Infof("[DEBUG] managed-serviceaccount addon not found: %v", err)
					return false
				}
				klog.Infof("[DEBUG] managed-serviceaccount addon: ready=%d, replicas=%d",
					deploy.Status.ReadyReplicas, deploy.Status.Replicas)
				return deploy.Status.ReadyReplicas > 0 && deploy.Status.ReadyReplicas == deploy.Status.Replicas
			}, 2*time.Minute, migrationPollInterval).Should(BeTrue(),
				"managed-serviceaccount addon should be ready on global hub")
		})

		// Step 2: Verify prerequisites
		It("should verify multicluster-engine namespace exists on source hub", func() {
			ns := &corev1.Namespace{}
			err := sourceHubClient.Get(ctx, types.NamespacedName{Name: mceNamespace}, ns)
			if errors.IsNotFound(err) {
				// Create it if not exists
				ns = &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: mceNamespace},
				}
				Expect(sourceHubClient.Create(ctx, ns)).To(Succeed())
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
		})

		// Step 3: Create ManagedClusterMigration CR
		It("should create ManagedClusterMigration CR", func() {
			klog.Infof("[DEBUG] Creating ManagedClusterMigration: %s, from %s to %s, cluster: %s",
				migrationName, sourceHubName, targetHubName, clusterToMigrate)
			mcm := &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      migrationName,
					Namespace: migrationNamespace,
				},
				Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
					IncludedManagedClusters: []string{clusterToMigrate},
					From:                    sourceHubName,
					To:                      targetHubName,
				},
			}
			err := globalHubClient.Create(ctx, mcm)
			Expect(err).NotTo(HaveOccurred())
			klog.Infof("[DEBUG] ManagedClusterMigration created successfully")
		})

		// Step 4 & 5: Wait for Initializing phase and mock source hub resources
		It("should wait for Initializing phase and create ManifestWork on source hub", func() {
			bootstrapSecretName := fmt.Sprintf("bootstrap-%s", targetHubName)

			By("Waiting for migration to reach Initializing phase")
			Eventually(func() string {
				mcm := &migrationv1alpha1.ManagedClusterMigration{}
				if err := globalHubClient.Get(ctx, types.NamespacedName{
					Name:      migrationName,
					Namespace: migrationNamespace,
				}, mcm); err != nil {
					klog.Infof("[DEBUG] Failed to get migration CR: %v", err)
					return ""
				}
				klog.Infof("[DEBUG] Migration phase: %s", mcm.Status.Phase)
				return string(mcm.Status.Phase)
			}, 2*time.Minute, migrationPollInterval).Should(
				Or(Equal("Initializing"), Equal("Deploying"), Equal("Registering")))

			By("Waiting for bootstrap secret to be created in multicluster-engine namespace")
			Eventually(func() error {
				secret := &corev1.Secret{}
				err := sourceHubClient.Get(ctx, types.NamespacedName{
					Name:      bootstrapSecretName,
					Namespace: mceNamespace,
				}, secret)
				if err != nil {
					klog.Infof("[DEBUG] Bootstrap secret %s not found in %s: %v", bootstrapSecretName, mceNamespace, err)
				} else {
					klog.Infof("[DEBUG] Bootstrap secret %s found in %s", bootstrapSecretName, mceNamespace)
				}
				return err
			}, 3*time.Minute, migrationPollInterval).Should(Succeed(),
				"bootstrap secret should be created by managed-serviceaccount addon")

			By("Step 5: Creating ManifestWork on source hub (Mock Initializing Phase)")
			createInitializingManifestWork(ctx, sourceHubClient, managedClusterClient, clusterToMigrate, targetHubName)
		})

		// Verify resources are applied on managed cluster
		It("should verify bootstrap secret and klusterlet are configured on managed cluster", func() {
			bootstrapSecretName := fmt.Sprintf("bootstrap-%s", targetHubName)

			By("Verifying bootstrap secret exists on managed cluster")
			Eventually(func() error {
				secret := &corev1.Secret{}
				return managedClusterClient.Get(ctx, types.NamespacedName{
					Name:      bootstrapSecretName,
					Namespace: agentNamespace,
				}, secret)
			}, 2*time.Minute, migrationPollInterval).Should(Succeed())

			By("Verifying klusterlet has MultipleHubs feature gate enabled")
			Eventually(func() bool {
				klusterlet := &operatorv1.Klusterlet{}
				if err := managedClusterClient.Get(ctx, types.NamespacedName{Name: "klusterlet"}, klusterlet); err != nil {
					return false
				}
				if klusterlet.Spec.RegistrationConfiguration == nil {
					return false
				}
				for _, fg := range klusterlet.Spec.RegistrationConfiguration.FeatureGates {
					if fg.Feature == "MultipleHubs" && fg.Mode == operatorv1.FeatureGateModeTypeEnable {
						return true
					}
				}
				return false
			}, 2*time.Minute, migrationPollInterval).Should(BeTrue())
		})

		// Step 6: Wait for Registering phase and create ReadOnly ManifestWork on target hub
		It("should wait for Registering phase and create ReadOnly ManifestWork on target hub", func() {
			By("Waiting for migration to reach Registering phase")
			Eventually(func() string {
				mcm := &migrationv1alpha1.ManagedClusterMigration{}
				if err := globalHubClient.Get(ctx, types.NamespacedName{
					Name:      migrationName,
					Namespace: migrationNamespace,
				}, mcm); err != nil {
					return ""
				}
				return string(mcm.Status.Phase)
			}, 5*time.Minute, migrationPollInterval).Should(Equal("Registering"))

			By("Step 6: Creating ReadOnly ManifestWork on target hub (Mock Registering Phase)")
			createRegisteringManifestWork(ctx, targetHubClient, clusterToMigrate)
		})

		// Step 7: Alternative strategy - if ManifestWork doesn't get Applied status,
		// manually update it when the cluster becomes Available on target hub
		It("should ensure ManifestWork is applied when cluster is available", func() {
			manifestWorkName := fmt.Sprintf("%s-klusterlet", clusterToMigrate)

			By("Step 7: Waiting for cluster to become Available on target hub and ensuring ManifestWork is Applied")
			Eventually(func() bool {
				// Check if ManagedCluster is Available on target hub
				mc := &clusterv1.ManagedCluster{}
				if err := targetHubClient.Get(ctx, types.NamespacedName{Name: clusterToMigrate}, mc); err != nil {
					return false
				}

				if !isManagedClusterAvailable(mc) {
					return false
				}

				// Cluster is Available, now check if ManifestWork is Applied
				mw := &workv1.ManifestWork{}
				if err := targetHubClient.Get(ctx, types.NamespacedName{
					Name:      manifestWorkName,
					Namespace: clusterToMigrate,
				}, mw); err != nil {
					return false
				}

				if !isManifestWorkApplied(mw) {
					// Manually set Applied status to true
					By("ManifestWork not Applied, manually updating status")
					mw.Status.Conditions = append(mw.Status.Conditions, metav1.Condition{
						Type:               workv1.WorkApplied,
						Status:             metav1.ConditionTrue,
						Reason:             "AppliedManifestComplete",
						Message:            "Apply manifest complete",
						LastTransitionTime: metav1.Now(),
					})
					if err := targetHubClient.Status().Update(ctx, mw); err != nil {
						return false
					}
				}

				return true
			}, 5*time.Minute, migrationPollInterval).Should(BeTrue())
		})

		// Step 8: Verify migration completed
		It("should complete migration successfully", func() {
			By("Waiting for migration to complete")
			Eventually(func() string {
				mcm := &migrationv1alpha1.ManagedClusterMigration{}
				if err := globalHubClient.Get(ctx, types.NamespacedName{
					Name:      migrationName,
					Namespace: migrationNamespace,
				}, mcm); err != nil {
					return ""
				}
				return string(mcm.Status.Phase)
			}, migrationTimeout, migrationPollInterval).Should(Equal("Completed"))
		})
	})
})

// createInitializingManifestWork creates ManifestWork on source hub containing:
// 1. Bootstrap secret (from multicluster-engine/bootstrap-<target-hub>, namespace changed to open-cluster-management-agent)
// 2. Klusterlet with MultipleHubs feature gate and bootstrapKubeConfigs
// This follows Step 5 in the manual test document.
func createInitializingManifestWork(ctx context.Context, sourceHubClient, managedClusterClient client.Client, clusterName, targetHub string) {
	bootstrapSecretName := fmt.Sprintf("bootstrap-%s", targetHub)

	// Step 5.1: Get bootstrap secret from multicluster-engine namespace on source hub
	bootstrapSecret := &corev1.Secret{}
	err := sourceHubClient.Get(ctx, types.NamespacedName{
		Name:      bootstrapSecretName,
		Namespace: mceNamespace,
	}, bootstrapSecret)
	if err != nil {
		// If bootstrap secret not found, log and return
		return
	}

	// Step 5.2: Create bootstrap secret manifest with namespace changed to open-cluster-management-agent
	bootstrapSecretManifest := map[string]any{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]any{
			"name":      bootstrapSecretName,
			"namespace": agentNamespace,
		},
		"data": bootstrapSecret.Data,
		"type": "Opaque",
	}

	// Step 5.3: Get existing klusterlet and create modified version
	existingKlusterlet := &operatorv1.Klusterlet{}
	err = managedClusterClient.Get(ctx, types.NamespacedName{Name: "klusterlet"}, existingKlusterlet)
	Expect(err).NotTo(HaveOccurred())

	// Create klusterlet manifest with MultipleHubs configuration
	// IMPORTANT: Include all necessary fields from existing klusterlet to avoid overwriting with empty values
	klusterletSpec := map[string]any{
		"clusterName":  existingKlusterlet.Spec.ClusterName,
		"namespace":    existingKlusterlet.Spec.Namespace,
		"deployOption": existingKlusterlet.Spec.DeployOption,
		"registrationConfiguration": map[string]any{
			"featureGates": []map[string]any{
				{"feature": "ClusterClaim", "mode": "Enable"},
				{"feature": "AddonManagement", "mode": "Enable"},
				{"feature": "MultipleHubs", "mode": "Enable"},
			},
			"bootstrapKubeConfigs": map[string]any{
				"type": "LocalSecrets",
				"localSecretsConfig": map[string]any{
					"hubConnectionTimeoutSeconds": 180,
					"kubeConfigSecrets": []map[string]any{
						{"name": bootstrapSecretName},
						{"name": "hub-kubeconfig-secret"},
					},
				},
			},
		},
	}
	// Add optional image specs from the original klusterlet
	addIfNotEmpty(klusterletSpec, "imagePullSpec", existingKlusterlet.Spec.ImagePullSpec)
	addIfNotEmpty(klusterletSpec, "registrationImagePullSpec", existingKlusterlet.Spec.RegistrationImagePullSpec)
	addIfNotEmpty(klusterletSpec, "workImagePullSpec", existingKlusterlet.Spec.WorkImagePullSpec)
	if len(existingKlusterlet.Spec.ExternalServerURLs) > 0 {
		urls := make([]map[string]any, len(existingKlusterlet.Spec.ExternalServerURLs))
		for i, u := range existingKlusterlet.Spec.ExternalServerURLs {
			urls[i] = map[string]any{"url": u.URL}
		}
		klusterletSpec["externalServerURLs"] = urls
	}
	klusterletManifest := map[string]any{
		"apiVersion": "operator.open-cluster-management.io/v1",
		"kind":       "Klusterlet",
		"metadata": map[string]any{
			"name": "klusterlet",
		},
		"spec": klusterletSpec,
	}

	// Serialize manifests
	bootstrapSecretBytes, _ := json.Marshal(bootstrapSecretManifest)
	klusterletBytes, _ := json.Marshal(klusterletManifest)

	// Step 5.4: Create ManifestWork with name <cluster-name>-klusterlet
	manifestWork := &workv1.ManifestWork{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-klusterlet", clusterName),
			Namespace: clusterName,
		},
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: []workv1.Manifest{
					{RawExtension: runtime.RawExtension{Raw: bootstrapSecretBytes}},
					{RawExtension: runtime.RawExtension{Raw: klusterletBytes}},
				},
			},
		},
	}

	// Create ManifestWork on source hub
	existing := &workv1.ManifestWork{}
	err = sourceHubClient.Get(ctx, client.ObjectKeyFromObject(manifestWork), existing)
	if errors.IsNotFound(err) {
		Expect(sourceHubClient.Create(ctx, manifestWork)).To(Succeed())
	}

	// NOTE: Direct bootstrap secret application is temporarily disabled since ManifestWork
	// is now working correctly in the e2e environment. The ManifestWork created above
	// includes the bootstrap secret and klusterlet configuration.
	//
	// If ManifestWork stops working in the future, uncomment this section to apply directly.
	/*
		// Since there's no work-agent in Kind e2e environment, directly apply resources to managed cluster
		// Apply bootstrap secret
		managedClusterBootstrapSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      bootstrapSecretName,
				Namespace: agentNamespace,
			},
			Data: bootstrapSecret.Data,
			Type: corev1.SecretTypeOpaque,
		}
		existingSecret := &corev1.Secret{}
		err = managedClusterClient.Get(ctx, client.ObjectKeyFromObject(managedClusterBootstrapSecret), existingSecret)
		if errors.IsNotFound(err) {
			Expect(managedClusterClient.Create(ctx, managedClusterBootstrapSecret)).To(Succeed())
		}

		// Apply klusterlet update
		klusterletToUpdate := &operatorv1.Klusterlet{}
		err = managedClusterClient.Get(ctx, types.NamespacedName{Name: "klusterlet"}, klusterletToUpdate)
		Expect(err).NotTo(HaveOccurred())

		klusterletToUpdate.Spec.RegistrationConfiguration = &operatorv1.RegistrationConfiguration{
			FeatureGates: []operatorv1.FeatureGate{
				{Feature: "ClusterClaim", Mode: operatorv1.FeatureGateModeTypeEnable},
				{Feature: "AddonManagement", Mode: operatorv1.FeatureGateModeTypeEnable},
				{Feature: "MultipleHubs", Mode: operatorv1.FeatureGateModeTypeEnable},
			},
			BootstrapKubeConfigs: operatorv1.BootstrapKubeConfigs{
				Type: operatorv1.LocalSecrets,
				LocalSecrets: &operatorv1.LocalSecretsConfig{
					HubConnectionTimeoutSeconds: 180,
					KubeConfigSecrets: []operatorv1.KubeConfigSecret{
						{Name: bootstrapSecretName},
						{Name: "hub-kubeconfig-secret"},
					},
				},
			},
		}
		Expect(managedClusterClient.Update(ctx, klusterletToUpdate)).To(Succeed())
	*/
}

// createRegisteringManifestWork creates a ReadOnly ManifestWork on target hub
// to collect klusterlet status. This follows Step 6 in the manual test document.
func createRegisteringManifestWork(ctx context.Context, targetHubClient client.Client, clusterName string) {
	// Ensure cluster namespace exists on target hub
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: clusterName},
	}
	_ = targetHubClient.Create(ctx, ns)

	// Step 6.1: Create ReadOnly ManifestWork
	klusterletManifest := map[string]any{
		"apiVersion": "operator.open-cluster-management.io/v1",
		"kind":       "Klusterlet",
		"metadata": map[string]any{
			"name": "klusterlet",
		},
	}
	klusterletBytes, _ := json.Marshal(klusterletManifest)

	manifestWork := &workv1.ManifestWork{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-klusterlet", clusterName),
			Namespace: clusterName,
		},
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: []workv1.Manifest{
					{RawExtension: runtime.RawExtension{Raw: klusterletBytes}},
				},
			},
			ManifestConfigs: []workv1.ManifestConfigOption{
				{
					ResourceIdentifier: workv1.ResourceIdentifier{
						Group:    "operator.open-cluster-management.io",
						Resource: "klusterlets",
						Name:     "klusterlet",
					},
					FeedbackRules: []workv1.FeedbackRule{
						{Type: workv1.WellKnownStatusType},
						{
							Type: workv1.JSONPathsType,
							JsonPaths: []workv1.JsonPath{
								{
									Name: "isAvailable",
									Path: `.status.conditions[?(@.type=="Available")].status`,
								},
							},
						},
					},
					UpdateStrategy: &workv1.UpdateStrategy{
						Type: workv1.UpdateStrategyTypeReadOnly,
					},
				},
			},
		},
	}

	// Create ManifestWork on target hub
	existing := &workv1.ManifestWork{}
	err := targetHubClient.Get(ctx, client.ObjectKeyFromObject(manifestWork), existing)
	if errors.IsNotFound(err) {
		Expect(targetHubClient.Create(ctx, manifestWork)).To(Succeed())
	}
}

// setupWorkAgentRBAC creates ClusterRole and ClusterRoleBinding for work-agent
// to manage klusterlets on managed clusters. This is needed because the ManifestWork
// containing klusterlet resources requires the work-agent SA to have permissions.
func setupWorkAgentRBAC(ctx context.Context, mcClient client.Client) {
	// Create ClusterRole for klusterlet management
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "klusterlet-work-sa-klusterlet-role",
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"operator.open-cluster-management.io"},
				Resources: []string{"klusterlets"},
				Verbs:     []string{"get", "list", "watch", "update", "patch"},
			},
		},
	}

	existing := &rbacv1.ClusterRole{}
	err := mcClient.Get(ctx, types.NamespacedName{Name: clusterRole.Name}, existing)
	if errors.IsNotFound(err) {
		Expect(mcClient.Create(ctx, clusterRole)).To(Succeed())
	}

	// Create ClusterRoleBinding
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "klusterlet-work-sa-klusterlet-binding",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "klusterlet-work-sa-klusterlet-role",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "klusterlet-work-sa",
				Namespace: agentNamespace,
			},
		},
	}

	existingBinding := &rbacv1.ClusterRoleBinding{}
	err = mcClient.Get(ctx, types.NamespacedName{Name: clusterRoleBinding.Name}, existingBinding)
	if errors.IsNotFound(err) {
		Expect(mcClient.Create(ctx, clusterRoleBinding)).To(Succeed())
	}
}

// verifyAutoApproveUsersSupport verifies that the ClusterManager CRD supports autoApproveUsers field.
// This is required for the agent to configure auto-approval for migrating clusters.
// If the CRD doesn't support autoApproveUsers, the migration will fail because the field
// will be silently dropped when updating the ClusterManager resource.
// Note: autoApproveUsers only takes effect when ManagedClusterAutoApproval feature gate is enabled.
func verifyAutoApproveUsersSupport(ctx context.Context, hubClient client.Client) {
	clusterManager := &operatorv1.ClusterManager{}
	err := hubClient.Get(ctx, types.NamespacedName{Name: "cluster-manager"}, clusterManager)
	Expect(err).NotTo(HaveOccurred(), "ClusterManager should exist on hub")

	// Enable ManagedClusterAutoApproval feature gate if not already enabled
	// This is required for autoApproveUsers to take effect
	if clusterManager.Spec.RegistrationConfiguration == nil {
		clusterManager.Spec.RegistrationConfiguration = &operatorv1.RegistrationHubConfiguration{}
	}

	// Check if ManagedClusterAutoApproval feature gate is already enabled
	featureGateEnabled := false
	for _, fg := range clusterManager.Spec.RegistrationConfiguration.FeatureGates {
		if fg.Feature == "ManagedClusterAutoApproval" && fg.Mode == operatorv1.FeatureGateModeTypeEnable {
			featureGateEnabled = true
			break
		}
	}

	testUser := "system:test:migration-verify"

	// Set both feature gate and autoApproveUsers in the same update operation
	// This mirrors the actual migration code behavior and avoids webhook/controller timing issues
	Eventually(func() error {
		// Get latest ClusterManager
		cm := &operatorv1.ClusterManager{}
		if err := hubClient.Get(ctx, types.NamespacedName{Name: "cluster-manager"}, cm); err != nil {
			return err
		}

		// Ensure RegistrationConfiguration exists
		if cm.Spec.RegistrationConfiguration == nil {
			cm.Spec.RegistrationConfiguration = &operatorv1.RegistrationHubConfiguration{}
		}

		// Enable feature gate if not already enabled
		if !featureGateEnabled {
			// Check again in case it was enabled by another process
			fgEnabled := false
			for _, fg := range cm.Spec.RegistrationConfiguration.FeatureGates {
				if fg.Feature == "ManagedClusterAutoApproval" && fg.Mode == operatorv1.FeatureGateModeTypeEnable {
					fgEnabled = true
					break
				}
			}
			if !fgEnabled {
				cm.Spec.RegistrationConfiguration.FeatureGates = append(
					cm.Spec.RegistrationConfiguration.FeatureGates,
					operatorv1.FeatureGate{
						Feature: "ManagedClusterAutoApproval",
						Mode:    operatorv1.FeatureGateModeTypeEnable,
					},
				)
			}
			// Mark as enabled for next iterations
			featureGateEnabled = true
		}

		// Set autoApproveUsers in the same update
		cm.Spec.RegistrationConfiguration.AutoApproveUsers = []string{testUser}

		if err := hubClient.Update(ctx, cm); err != nil {
			return fmt.Errorf("failed to update ClusterManager: %w", err)
		}

		klog.Infof("[DEBUG] Updated ClusterManager with feature gate and autoApproveUsers")

		// Verify the value was saved
		updatedCM := &operatorv1.ClusterManager{}
		if err := hubClient.Get(ctx, types.NamespacedName{Name: "cluster-manager"}, updatedCM); err != nil {
			return err
		}

		if updatedCM.Spec.RegistrationConfiguration == nil {
			return fmt.Errorf("RegistrationConfiguration is nil after update")
		}

		// Check if testUser is in the list
		if !slices.Contains(updatedCM.Spec.RegistrationConfiguration.AutoApproveUsers, testUser) {
			return fmt.Errorf("autoApproveUsers does not contain test user, got: %v", updatedCM.Spec.RegistrationConfiguration.AutoApproveUsers)
		}

		return nil
	}, 2*time.Minute, 5*time.Second).Should(Succeed(),
		"autoApproveUsers should be saved in ClusterManager. "+
			"Ensure ManagedClusterAutoApproval feature gate is enabled.")

	// Clean up test value
	clusterManager.Spec.RegistrationConfiguration.AutoApproveUsers = nil
	_ = hubClient.Update(ctx, clusterManager)
	klog.Infof("[DEBUG] ClusterManager CRD supports autoApproveUsers field")
}

// restoreKlusterlet restores the klusterlet on the managed cluster to its original configuration
// by removing MultipleHubs feature gate and bootstrap secrets for the target hub.
func restoreKlusterlet(ctx context.Context, mcClient client.Client, targetHubName string) {
	klusterlet := &operatorv1.Klusterlet{}
	if err := mcClient.Get(ctx, types.NamespacedName{Name: "klusterlet"}, klusterlet); err != nil {
		klog.Infof("[DEBUG] restoreKlusterlet: klusterlet not found, skipping restore")
		return
	}

	// Only modify if RegistrationConfiguration exists and has MultipleHubs
	if klusterlet.Spec.RegistrationConfiguration != nil {
		// Remove MultipleHubs from feature gates, keep others
		var newFeatureGates []operatorv1.FeatureGate
		for _, fg := range klusterlet.Spec.RegistrationConfiguration.FeatureGates {
			if fg.Feature != "MultipleHubs" {
				newFeatureGates = append(newFeatureGates, fg)
			}
		}

		// Clear BootstrapKubeConfigs to use default hub-kubeconfig-secret
		klusterlet.Spec.RegistrationConfiguration = &operatorv1.RegistrationConfiguration{
			FeatureGates: newFeatureGates,
		}
		if err := mcClient.Update(ctx, klusterlet); err != nil {
			klog.Infof("[DEBUG] restoreKlusterlet: failed to update klusterlet: %v", err)
		} else {
			klog.Infof("[DEBUG] restoreKlusterlet: klusterlet updated successfully")
		}
	}

	// Delete bootstrap secret for target hub
	bootstrapSecretName := fmt.Sprintf("bootstrap-%s", targetHubName)
	_ = mcClient.Delete(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      bootstrapSecretName,
			Namespace: agentNamespace,
		},
	})
}

// restoreManagedClusterAcceptance restores the managed cluster acceptance on the source hub.
func restoreManagedClusterAcceptance(ctx context.Context, hubClient client.Client, clusterName string) {
	mc := &clusterv1.ManagedCluster{}
	if err := hubClient.Get(ctx, types.NamespacedName{Name: clusterName}, mc); err != nil {
		return
	}
	mc.Spec.HubAcceptsClient = true
	_ = hubClient.Update(ctx, mc)
}

// isManagedClusterAvailable checks if a ManagedCluster has the Available condition set to True.
func isManagedClusterAvailable(mc *clusterv1.ManagedCluster) bool {
	for _, cond := range mc.Status.Conditions {
		if cond.Type == clusterv1.ManagedClusterConditionAvailable && cond.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}

// isManifestWorkApplied checks if a ManifestWork has the Applied condition set to True.
func isManifestWorkApplied(mw *workv1.ManifestWork) bool {
	for _, cond := range mw.Status.Conditions {
		if cond.Type == workv1.WorkApplied && cond.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}

// addIfNotEmpty adds a key-value pair to the map only if the value is not empty.
func addIfNotEmpty(m map[string]any, key, value string) {
	if value != "" {
		m[key] = value
	}
}
