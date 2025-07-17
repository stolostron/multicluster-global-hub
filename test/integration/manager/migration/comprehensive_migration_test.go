package migration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/migration"
	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var _ = Describe("Comprehensive Migration Flow Integration Tests", Ordered, func() {
	var testCtx context.Context
	var testCtxCancel context.CancelFunc
	var testID string
	var fromHub, toHub string

	BeforeAll(func() {
		testCtx, testCtxCancel = context.WithCancel(ctx)
		testID = fmt.Sprintf("comprehensive-%d", time.Now().UnixNano())
		fromHub = fmt.Sprintf("hub1-%s", testID)
		toHub = fmt.Sprintf("hub2-%s", testID)

		// Set up test environment
		setupTestEnvironment(testCtx, fromHub, toHub, testID)
	})

	AfterAll(func() {
		cleanupTestEnvironment(testCtx, testID)
		testCtxCancel()
		time.Sleep(200 * time.Millisecond)
	})

	Describe("Complete Migration Lifecycle", func() {
		var m *migrationv1alpha1.ManagedClusterMigration
		var clusterName string

		BeforeEach(func() {
			clusterName = fmt.Sprintf("cluster1-%s", testID)
			migrationName := fmt.Sprintf("migration-%s", testID)
			m = createMigrationCR(testCtx, migrationName, []string{clusterName}, fromHub, toHub)
		})

		It("should go through the complete migration lifecycle successfully", func() {
			By("Phase 1: Pending -> Validating")
			assertMigrationPhase(testCtx, m, migrationv1alpha1.PhaseValidating)

			By("Phase 2: Validating -> Initializing")
			assertMigrationCondition(testCtx, m, migrationv1alpha1.ConditionTypeValidated, metav1.ConditionTrue, migration.ConditionReasonResourceValidated)
			assertMigrationPhase(testCtx, m, migrationv1alpha1.PhaseInitializing)

			By("Phase 3: Initializing -> Deploying")
			// Create token secret to enable initialization
			createMockTokenSecret(testCtx, m.Name, toHub)

			// Simulate hub confirmations for initialization
			simulateHubConfirmation(m.GetUID(), fromHub, migrationv1alpha1.PhaseInitializing)
			simulateHubConfirmation(m.GetUID(), toHub, migrationv1alpha1.PhaseInitializing)

			assertMigrationCondition(testCtx, m, migrationv1alpha1.ConditionTypeInitialized, metav1.ConditionTrue, migration.ConditionReasonResourceInitialized)
			assertMigrationPhase(testCtx, m, migrationv1alpha1.PhaseDeploying)

			By("Phase 4: Deploying -> Registering")
			// Simulate hub confirmations for deployment
			simulateHubConfirmation(m.GetUID(), fromHub, migrationv1alpha1.PhaseDeploying)
			simulateHubConfirmation(m.GetUID(), toHub, migrationv1alpha1.PhaseDeploying)

			assertMigrationCondition(testCtx, m, migrationv1alpha1.ConditionTypeDeployed, metav1.ConditionTrue, migration.ConditionReasonResourcesDeployed)
			assertMigrationPhase(testCtx, m, migrationv1alpha1.PhaseRegistering)

			By("Phase 5: Registering -> Cleaning")
			// Simulate hub confirmations for registration
			simulateHubConfirmation(m.GetUID(), fromHub, migrationv1alpha1.PhaseRegistering)
			simulateHubConfirmation(m.GetUID(), toHub, migrationv1alpha1.PhaseRegistering)

			assertMigrationCondition(testCtx, m, migrationv1alpha1.ConditionTypeRegistered, metav1.ConditionTrue, migration.ConditionReasonClusterRegistered)
			assertMigrationPhase(testCtx, m, migrationv1alpha1.PhaseCleaning)

			By("Phase 6: Cleaning -> Completed")
			// Simulate hub confirmations for cleanup
			simulateHubConfirmation(m.GetUID(), fromHub, migrationv1alpha1.PhaseCleaning)
			simulateHubConfirmation(m.GetUID(), toHub, migrationv1alpha1.PhaseCleaning)

			assertMigrationCondition(testCtx, m, migrationv1alpha1.ConditionTypeCleaned, metav1.ConditionTrue, migration.ConditionReasonResourceCleaned)
			assertMigrationPhase(testCtx, m, migrationv1alpha1.PhaseCompleted)
		})
	})

	Describe("Migration Error Handling", func() {
		It("should handle validation errors correctly", func() {
			By("Creating a migration with invalid configuration")
			invalidMigrationName := fmt.Sprintf("invalid-migration-%s", testID)
			invalidClusterName := fmt.Sprintf("nonexistent-cluster-%s", testID)

			// Create migration with non-existent cluster
			m := createMigrationCR(testCtx, invalidMigrationName, []string{invalidClusterName}, fromHub, toHub)

			By("Verifying migration fails during validation")
			assertMigrationCondition(testCtx, m, migrationv1alpha1.ConditionTypeValidated, metav1.ConditionFalse, migration.ConditionReasonClusterNotFound)
			assertMigrationPhase(testCtx, m, migrationv1alpha1.PhaseFailed)
		})

		It("should handle hub not found errors", func() {
			By("Creating a migration with non-existent source hub")
			invalidMigrationName := fmt.Sprintf("invalid-source-hub-%s", testID)
			clusterName := fmt.Sprintf("cluster1-%s", testID)
			nonExistentHub := fmt.Sprintf("nonexistent-hub-%s", testID)

			m := createMigrationCR(testCtx, invalidMigrationName, []string{clusterName}, nonExistentHub, toHub)

			By("Verifying migration fails due to hub not found")
			assertMigrationCondition(testCtx, m, migrationv1alpha1.ConditionTypeValidated, metav1.ConditionFalse, migration.ConditionReasonHubClusterNotFound)
			assertMigrationPhase(testCtx, m, migrationv1alpha1.PhaseFailed)
		})

		It("should handle initialization timeout", func() {
			By("Creating a migration that will timeout during initialization")
			timeoutMigrationName := fmt.Sprintf("timeout-migration-%s", testID)
			clusterName := fmt.Sprintf("cluster1-%s", testID)

			m := createMigrationCR(testCtx, timeoutMigrationName, []string{clusterName}, fromHub, toHub)

			By("Verifying migration reaches initializing phase")
			assertMigrationCondition(testCtx, m, migrationv1alpha1.ConditionTypeValidated, metav1.ConditionTrue, migration.ConditionReasonResourceValidated)
			assertMigrationPhase(testCtx, m, migrationv1alpha1.PhaseInitializing)

			By("Verifying migration stays in waiting state without hub confirmations")
			assertMigrationCondition(testCtx, m, migrationv1alpha1.ConditionTypeInitialized, metav1.ConditionFalse, migration.ConditionReasonWaiting)

			// Verify it doesn't progress without confirmations
			Consistently(func() string {
				Expect(mgr.GetClient().Get(testCtx, client.ObjectKeyFromObject(m), m)).To(Succeed())
				return m.Status.Phase
			}, 3*time.Second, 500*time.Millisecond).Should(Equal(migrationv1alpha1.PhaseInitializing))
		})
	})

	Describe("Migration with Resource Constraints", func() {
		It("should handle migrations with included resources", func() {
			By("Creating a migration with specific resource constraints")
			resourceMigrationName := fmt.Sprintf("resource-migration-%s", testID)
			clusterName := fmt.Sprintf("cluster1-%s", testID)

			m := &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceMigrationName,
					Namespace: utils.GetDefaultNamespace(),
				},
				Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
					IncludedManagedClusters: []string{clusterName},
					From:                    fromHub,
					To:                      toHub,
					IncludedResources: []string{
						"configmap/default/test-config",
						"secret/kube-system/test-secret",
					},
				},
			}

			Expect(mgr.GetClient().Create(testCtx, m)).To(Succeed())

			By("Verifying migration validates resources correctly")
			assertMigrationCondition(testCtx, m, migrationv1alpha1.ConditionTypeValidated, metav1.ConditionTrue, migration.ConditionReasonResourceValidated)
			assertMigrationPhase(testCtx, m, migrationv1alpha1.PhaseInitializing)
		})

		It("should reject migrations with invalid resource formats", func() {
			By("Creating a migration with invalid resource format")
			invalidResourceMigrationName := fmt.Sprintf("invalid-resource-migration-%s", testID)
			clusterName := fmt.Sprintf("cluster1-%s", testID)

			m := &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      invalidResourceMigrationName,
					Namespace: utils.GetDefaultNamespace(),
				},
				Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
					IncludedManagedClusters: []string{clusterName},
					From:                    fromHub,
					To:                      toHub,
					IncludedResources: []string{
						"invalid-format",
						"pod/default/unsupported-kind",
					},
				},
			}

			Expect(mgr.GetClient().Create(testCtx, m)).To(Succeed())

			By("Verifying migration fails resource validation")
			assertMigrationCondition(testCtx, m, migrationv1alpha1.ConditionTypeValidated, metav1.ConditionFalse, migration.ConditionReasonResourceInvalid)
			assertMigrationPhase(testCtx, m, migrationv1alpha1.PhaseFailed)
		})
	})

	Describe("Migration State Management", func() {
		It("should handle multiple concurrent migrations", func() {
			By("Creating multiple migrations")
			migration1Name := fmt.Sprintf("concurrent-migration1-%s", testID)
			migration2Name := fmt.Sprintf("concurrent-migration2-%s", testID)
			clusterName := fmt.Sprintf("cluster1-%s", testID)

			m1 := createMigrationCR(testCtx, migration1Name, []string{clusterName}, fromHub, toHub)
			m2 := createMigrationCR(testCtx, migration2Name, []string{clusterName}, fromHub, toHub)

			By("Verifying only one migration proceeds while others wait")
			// First migration should proceed
			assertMigrationCondition(testCtx, m1, migrationv1alpha1.ConditionTypeValidated, metav1.ConditionTrue, migration.ConditionReasonResourceValidated)

			// Second migration should wait in pending state
			assertMigrationPhase(testCtx, m2, migrationv1alpha1.PhasePending)

			By("Verifying second migration can proceed after first completes")
			// Complete first migration
			completeFullMigration(testCtx, m1)

			// Second migration should now proceed
			Eventually(func() string {
				Expect(mgr.GetClient().Get(testCtx, client.ObjectKeyFromObject(m2), m2)).To(Succeed())
				return m2.Status.Phase
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(migrationv1alpha1.PhaseValidating))
		})

		It("should handle migration deletion gracefully", func() {
			By("Creating a migration and then requesting deletion")
			deletionMigrationName := fmt.Sprintf("deletion-migration-%s", testID)
			clusterName := fmt.Sprintf("cluster1-%s", testID)

			m := createMigrationCR(testCtx, deletionMigrationName, []string{clusterName}, fromHub, toHub)

			By("Verifying migration starts normally")
			assertMigrationCondition(testCtx, m, migrationv1alpha1.ConditionTypeValidated, metav1.ConditionTrue, migration.ConditionReasonResourceValidated)

			By("Deleting the migration")
			Expect(mgr.GetClient().Delete(testCtx, m)).To(Succeed())

			By("Verifying migration is cleaned up properly")
			Eventually(func() bool {
				err := mgr.GetClient().Get(testCtx, client.ObjectKeyFromObject(m), m)
				return errors.IsNotFound(err)
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())
		})
	})
})

// Helper functions for comprehensive testing

func setupTestEnvironment(ctx context.Context, fromHub, toHub, testID string) {
	// Create test resources
	createTestResources(ctx, fromHub)
	createTestResources(ctx, toHub)

	// Create test hubs
	createTestHub(ctx, fromHub)
	createTestHub(ctx, toHub)

	// Create test managed cluster
	clusterName := fmt.Sprintf("cluster1-%s", testID)
	createTestManagedCluster(ctx, clusterName, fromHub)
}

func cleanupTestEnvironment(ctx context.Context, testID string) {
	// Clean up migration resources
	cleanupMigrationResources(ctx, testID)

	// Clean up database entries
	db.Exec("DELETE FROM status.leaf_hubs WHERE leaf_hub_name LIKE ?", fmt.Sprintf("%%-%s", testID))
	db.Exec("DELETE FROM status.managed_clusters WHERE leaf_hub_name LIKE ?", fmt.Sprintf("%%-%s", testID))

	// Clean up other resources
	cleanupAllTestResources(ctx, testID)
}

func cleanupAllTestResources(ctx context.Context, testID string) {
	// Clean up ManagedClusters
	mcList := &clusterv1.ManagedClusterList{}
	if err := mgr.GetClient().List(ctx, mcList); err == nil {
		for _, mc := range mcList.Items {
			if containsTestID(mc.Name, testID) {
				mgr.GetClient().Delete(ctx, &mc)
			}
		}
	}

	// Clean up Namespaces
	nsList := &corev1.NamespaceList{}
	if err := mgr.GetClient().List(ctx, nsList); err == nil {
		for _, ns := range nsList.Items {
			if containsTestID(ns.Name, testID) {
				mgr.GetClient().Delete(ctx, &ns)
			}
		}
	}

	// Clean up Secrets
	secretList := &corev1.SecretList{}
	if err := mgr.GetClient().List(ctx, secretList); err == nil {
		for _, secret := range secretList.Items {
			if containsTestID(secret.Name, testID) {
				mgr.GetClient().Delete(ctx, &secret)
			}
		}
	}

	// Clean up ManagedClusterAddOns
	addOnList := &addonapiv1alpha1.ManagedClusterAddOnList{}
	if err := mgr.GetClient().List(ctx, addOnList); err == nil {
		for _, addOn := range addOnList.Items {
			if containsTestID(addOn.Namespace, testID) {
				mgr.GetClient().Delete(ctx, &addOn)
			}
		}
	}
}

func containsTestID(name, testID string) bool {
	return len(name) > len(testID) && name[len(name)-len(testID):] == testID
}

func createMigrationCRWithResources(ctx context.Context, name string, clusters []string, from, to string, resources []string) *migrationv1alpha1.ManagedClusterMigration {
	m := &migrationv1alpha1.ManagedClusterMigration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: utils.GetDefaultNamespace(),
		},
		Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
			IncludedManagedClusters: clusters,
			From:                    from,
			To:                      to,
			IncludedResources:       resources,
		},
	}

	// Clean up any existing migration with the same name first
	existing := &migrationv1alpha1.ManagedClusterMigration{}
	err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(m), existing)
	if err == nil {
		// Remove finalizers and delete
		existing.Finalizers = nil
		mgr.GetClient().Update(ctx, existing)
		mgr.GetClient().Delete(ctx, existing)

		// Wait for deletion to complete
		Eventually(func() bool {
			err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(m), existing)
			return errors.IsNotFound(err)
		}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())
	}

	Expect(mgr.GetClient().Create(ctx, m)).To(Succeed())
	Expect(mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(m), m)).To(Succeed())
	return m
}

func createMockDatabase(ctx context.Context, testID string) {
	// Mock database entries for testing
	fromHubName := fmt.Sprintf("hub1-%s", testID)
	toHubName := fmt.Sprintf("hub2-%s", testID)
	clusterName := fmt.Sprintf("cluster1-%s", testID)

	// Create leaf hub entries
	db.Model(&models.LeafHub{}).Create(&models.LeafHub{
		LeafHubName: fromHubName,
		ClusterID:   uuid.New().String(),
		Payload:     []byte(`{}`),
	})

	db.Model(&models.LeafHub{}).Create(&models.LeafHub{
		LeafHubName: toHubName,
		ClusterID:   uuid.New().String(),
		Payload:     []byte(`{}`),
	})

	// Create managed cluster entry
	payload, _ := json.Marshal(map[string]interface{}{
		"metadata": map[string]interface{}{
			"name": clusterName,
		},
		"spec": map[string]interface{}{
			"managedClusterClientConfigs": []map[string]interface{}{
				{"url": "https://example.com"},
			},
		},
	})

	db.Model(&models.ManagedCluster{}).Create(&models.ManagedCluster{
		LeafHubName: fromHubName,
		ClusterID:   uuid.New().String(),
		Payload:     payload,
		Error:       database.ErrorNone,
	})
}
