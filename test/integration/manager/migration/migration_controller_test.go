package migration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
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

var _ = Describe("ManagedCluster Migration Controller", Ordered, func() {
	var testCtx context.Context
	var testCtxCancel context.CancelFunc
	var testID string
	var fromHub, toHub string
	var m *migrationv1alpha1.ManagedClusterMigration

	BeforeAll(func() {
		testCtx, testCtxCancel = context.WithCancel(ctx)

		// Generate unique test identifiers to avoid conflicts
		testID = fmt.Sprintf("basic-%d", time.Now().UnixNano())
		fromHub = fmt.Sprintf("hub1-%s", testID)
		toHub = fmt.Sprintf("hub2-%s", testID)

		// Create namespaces and other required resources for hubs
		createTestResources(testCtx, fromHub)
		createTestResources(testCtx, toHub)
		createTestHub(testCtx, fromHub)
		createTestHub(testCtx, toHub)
		createTestManagedCluster(testCtx, fmt.Sprintf("cluster1-%s", testID), fromHub)
		m = createMigrationCR(testCtx, fmt.Sprintf("migration-test-%s", testID), []string{fmt.Sprintf("cluster1-%s", testID)}, fromHub, toHub)
		time.Sleep(100 * time.Millisecond)
	})

	AfterAll(func() {
		// Clean up created resources in reverse order
		cleanupMigrationResources(testCtx, testID)
		deleteTestResources(testCtx, fromHub)
		deleteTestResources(testCtx, toHub)
		testCtxCancel()

		// Add a small delay to ensure cleanup completes
		time.Sleep(100 * time.Millisecond)
	})

	Describe("Basic Migration Flow", func() {
		It("should be validated successfully", func() {
			assertMigrationConditionHelper(testCtx, m, migrationv1alpha1.ConditionTypeValidated, metav1.ConditionTrue, migration.ConditionReasonResourceValidated)
		})

		It("should remain in Initializing phase with 'Waiting' reason if hubs have not confirmed", func() {
			assertMigrationConditionHelper(testCtx, m, migrationv1alpha1.ConditionTypeInitialized, metav1.ConditionFalse, migration.ConditionReasonWaiting)
			Expect(m.Status.Phase).ShouldNot(Equal(migrationv1alpha1.PhaseFailed))
		})

		It("should update status to Initialized once both hubs confirm", func() {
			Eventually(func(g Gomega) {
				g.Expect(mgr.GetClient().Get(testCtx, client.ObjectKeyFromObject(m), m)).To(Succeed())
				g.Expect(m.GetUID()).NotTo(BeEmpty())
			}).Should(Succeed())

			createMockTokenSecret(testCtx, m.Name, toHub)

			simulateHubConfirmation(m.GetUID(), fromHub, migrationv1alpha1.PhaseInitializing)
			simulateHubConfirmation(m.GetUID(), toHub, migrationv1alpha1.PhaseInitializing)
			assertMigrationCondition(testCtx, m, migrationv1alpha1.ConditionTypeInitialized, metav1.ConditionTrue, migration.ConditionReasonResourceInitialized)
		})

		It("should remain in Deploying phase with 'Waiting' reason if hubs have not confirmed", func() {
			assertMigrationCondition(testCtx, m, migrationv1alpha1.ConditionTypeDeployed, metav1.ConditionFalse, migration.ConditionReasonWaiting)
		})

		It("should update status to Deployed once both hubs confirm", func() {
			Eventually(func(g Gomega) {
				g.Expect(mgr.GetClient().Get(testCtx, client.ObjectKeyFromObject(m), m)).To(Succeed())
				g.Expect(m.GetUID()).NotTo(BeEmpty())
			}).Should(Succeed())

			simulateHubConfirmation(m.GetUID(), fromHub, migrationv1alpha1.PhaseDeploying)
			simulateHubConfirmation(m.GetUID(), toHub, migrationv1alpha1.PhaseDeploying)
			assertMigrationCondition(testCtx, m, migrationv1alpha1.ConditionTypeDeployed, metav1.ConditionTrue, migration.ConditionReasonResourcesDeployed)
		})

		It("should complete registration phase", func() {
			Eventually(func(g Gomega) {
				g.Expect(mgr.GetClient().Get(testCtx, client.ObjectKeyFromObject(m), m)).To(Succeed())
				g.Expect(m.Status.Phase).To(Equal(migrationv1alpha1.PhaseRegistering))
			}).Should(Succeed())

			simulateHubConfirmation(m.GetUID(), fromHub, migrationv1alpha1.PhaseRegistering)
			simulateHubConfirmation(m.GetUID(), toHub, migrationv1alpha1.PhaseRegistering)
			assertMigrationCondition(testCtx, m, migrationv1alpha1.ConditionTypeRegistered, metav1.ConditionTrue, migration.ConditionReasonClusterRegistered)
		})

		It("should complete cleaning phase", func() {
			Eventually(func(g Gomega) {
				g.Expect(mgr.GetClient().Get(testCtx, client.ObjectKeyFromObject(m), m)).To(Succeed())
				g.Expect(m.Status.Phase).To(Equal(migrationv1alpha1.PhaseCleaning))
			}).Should(Succeed())

			simulateHubConfirmation(m.GetUID(), fromHub, migrationv1alpha1.PhaseCleaning)
			simulateHubConfirmation(m.GetUID(), toHub, migrationv1alpha1.PhaseCleaning)
			assertMigrationCondition(testCtx, m, migrationv1alpha1.ConditionTypeCleaned, metav1.ConditionTrue, migration.ConditionReasonResourceCleaned)
		})

		It("should reach completed state", func() {
			Eventually(func(g Gomega) {
				g.Expect(mgr.GetClient().Get(testCtx, client.ObjectKeyFromObject(m), m)).To(Succeed())
				g.Expect(m.Status.Phase).To(Equal(migrationv1alpha1.PhaseCompleted))
			}).Should(Succeed())
		})
	})

	Describe("Migration State Persistence", func() {
		It("should maintain migration state across restarts", func() {
			// Get current migration state
			currentMigration := &migrationv1alpha1.ManagedClusterMigration{}
			Expect(mgr.GetClient().Get(testCtx, client.ObjectKeyFromObject(m), currentMigration)).To(Succeed())

			// Simulate restart by checking if migration status is preserved
			Eventually(func(g Gomega) {
				retrievedMigration := &migrationv1alpha1.ManagedClusterMigration{}
				g.Expect(mgr.GetClient().Get(testCtx, client.ObjectKeyFromObject(m), retrievedMigration)).To(Succeed())
				g.Expect(retrievedMigration.Status.Phase).To(Equal(currentMigration.Status.Phase))
				g.Expect(len(retrievedMigration.Status.Conditions)).To(Equal(len(currentMigration.Status.Conditions)))
			}).Should(Succeed())
		})

		It("should handle migration event status correctly", func() {
			// Verify migration event status functions work properly
			uid := string(m.GetUID())

			// Test started/finished states
			By("Setting and checking started state")
			migration.SetStarted(uid, fromHub, migrationv1alpha1.PhaseInitializing)
			Expect(migration.GetStarted(uid, fromHub, migrationv1alpha1.PhaseInitializing)).To(BeTrue())

			By("Setting and checking finished state")
			migration.SetFinished(uid, fromHub, migrationv1alpha1.PhaseInitializing)
			Expect(migration.GetFinished(uid, fromHub, migrationv1alpha1.PhaseInitializing)).To(BeTrue())

			By("Setting and checking error state")
			testErrorMsg := "test error message"
			migration.SetErrorMessage(uid, fromHub, migrationv1alpha1.PhaseInitializing, testErrorMsg)
			Expect(migration.GetErrorMessage(uid, fromHub, migrationv1alpha1.PhaseInitializing)).To(Equal(testErrorMsg))
		})
	})
})

// Helper functions to reduce boilerplate

func createMigrationCR(ctx context.Context, name string, clusters []string, from, to string) *migrationv1alpha1.ManagedClusterMigration {
	m := &migrationv1alpha1.ManagedClusterMigration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: utils.GetDefaultNamespace(),
		},
		Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
			IncludedManagedClusters: clusters,
			From:                    from,
			To:                      to,
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

func createTestResources(ctx context.Context, hubName string) {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: hubName}}
	err := mgr.GetClient().Create(ctx, ns)
	if !errors.IsAlreadyExists(err) {
		Expect(err).NotTo(HaveOccurred())
	}
	// Create a mock ManagedClusterAddOn for the hub
	createMockManagedClusterAddOn(ctx, hubName)
}

func deleteTestResources(ctx context.Context, hubName string) {
	// Clean up database entries first
	db.Exec("DELETE FROM status.leaf_hubs WHERE leaf_hub_name = ?", hubName)
	db.Exec("DELETE FROM status.managed_clusters WHERE leaf_hub_name = ?", hubName)

	// Clean up ManagedCluster
	mc := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: hubName}}
	err := mgr.GetClient().Delete(ctx, mc)
	if err != nil && !errors.IsNotFound(err) {
		// Log error but don't fail the test
		fmt.Printf("Warning: Failed to delete ManagedCluster %s: %v\n", hubName, err)
	}

	// Clean up namespace
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: hubName}}
	err = mgr.GetClient().Delete(ctx, ns)
	if err != nil && !errors.IsNotFound(err) {
		// Log error but don't fail the test
		fmt.Printf("Warning: Failed to delete namespace %s: %v\n", hubName, err)
		return
	}

	// Don't wait for namespace deletion to avoid hanging tests
	// Just try to delete and move on
	time.Sleep(100 * time.Millisecond)
}

func createTestHub(ctx context.Context, hubName string) {
	// Create in K8s
	mc := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: hubName},
		Spec: clusterv1.ManagedClusterSpec{
			ManagedClusterClientConfigs: []clusterv1.ClientConfig{{URL: "https://example.com"}},
		},
	}
	err := mgr.GetClient().Create(ctx, mc)
	if !errors.IsAlreadyExists(err) {
		Expect(err).To(Succeed())
	}

	// Create in DB
	err = db.Model(&models.LeafHub{}).Create(&models.LeafHub{
		LeafHubName: hubName,
		ClusterID:   uuid.New().String(),
		Payload:     []byte(`{}`),
	}).Error
	Expect(err).To(Succeed())
}

func createTestManagedCluster(ctx context.Context, clusterName, hubName string) {
	// Create in K8s
	mc := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: clusterName},
		Spec: clusterv1.ManagedClusterSpec{
			ManagedClusterClientConfigs: []clusterv1.ClientConfig{{URL: "https://example.com"}},
		},
	}
	err := mgr.GetClient().Create(ctx, mc)
	if !errors.IsAlreadyExists(err) {
		Expect(err).To(Succeed())
	}

	// Create in DB - ensure payload has metadata.name for cluster_name generation
	payload, err := json.Marshal(map[string]interface{}{
		"metadata": map[string]interface{}{
			"name": clusterName,
		},
		"spec": mc.Spec,
	})
	Expect(err).To(Succeed())

	err = db.Model(&models.ManagedCluster{}).Create(&models.ManagedCluster{
		LeafHubName: hubName,
		ClusterID:   uuid.New().String(),
		Payload:     payload,
		Error:       database.ErrorNone,
	}).Error
	Expect(err).To(BeNil())
}

func assertMigrationCondition(ctx context.Context, m *migrationv1alpha1.ManagedClusterMigration,
	conditionType string, status metav1.ConditionStatus, reason string,
) {
	Eventually(func(g Gomega) {
		key := types.NamespacedName{Name: m.Name, Namespace: m.Namespace}
		g.Expect(mgr.GetClient().Get(ctx, key, m)).To(Succeed())

		cond := meta.FindStatusCondition(m.Status.Conditions, conditionType)
		g.Expect(cond).NotTo(BeNil(), fmt.Sprintf("Condition %s not found", conditionType))
		g.Expect(cond.Status).To(Equal(status), fmt.Sprintf("Expected status %s but got %s", status, cond.Status))
		g.Expect(cond.Reason).To(Equal(reason), fmt.Sprintf("Expected reason %s but got %s", reason, cond.Reason))
	}, 10*time.Second, 100*time.Millisecond).Should(Succeed())
}

func simulateHubConfirmation(uid types.UID, hubName string, phase string) {
	migration.SetFinished(string(uid), hubName, phase)
}

func cleanupMigrationResources(ctx context.Context, testID string) {
	// Clean up any migration CRs that might be left over
	migrationList := &migrationv1alpha1.ManagedClusterMigrationList{}
	err := mgr.GetClient().List(ctx, migrationList)
	if err == nil {
		for _, migrationCR := range migrationList.Items {
			if strings.Contains(migrationCR.Name, testID) {
				// Remove finalizers to allow deletion
				toDelete := migrationCR.DeepCopy()
				toDelete.Finalizers = nil
				mgr.GetClient().Update(ctx, toDelete)
				mgr.GetClient().Delete(ctx, toDelete)
			}
		}
	}

	// Clean up any mock token secrets
	secretList := &corev1.SecretList{}
	err = mgr.GetClient().List(ctx, secretList)
	if err == nil {
		for _, secret := range secretList.Items {
			if strings.Contains(secret.Name, testID) {
				mgr.GetClient().Delete(ctx, &secret)
			}
		}
	}

	// Clean up ManagedClusterAddOn
	addOnList := &addonapiv1alpha1.ManagedClusterAddOnList{}
	err = mgr.GetClient().List(ctx, addOnList)
	if err == nil {
		for _, addOn := range addOnList.Items {
			if strings.Contains(addOn.Namespace, testID) {
				mgr.GetClient().Delete(ctx, &addOn)
			}
		}
	}
}

func createMockManagedClusterAddOn(ctx context.Context, hubName string) {
	addOn := &addonapiv1alpha1.ManagedClusterAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "managed-serviceaccount",
			Namespace: hubName,
		},
	}
	err := mgr.GetClient().Create(ctx, addOn)
	if !errors.IsAlreadyExists(err) {
		Expect(err).NotTo(HaveOccurred())
	}

	// Update the status of the addon to simulate it being ready
	Eventually(func(g Gomega) {
		createdAddOn := &addonapiv1alpha1.ManagedClusterAddOn{}
		g.Expect(mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(addOn), createdAddOn)).To(Succeed())
		createdAddOn.Status = addonapiv1alpha1.ManagedClusterAddOnStatus{
			Namespace: "open-cluster-management-agent-addon",
		}
		g.Expect(mgr.GetClient().Status().Update(ctx, createdAddOn)).To(Succeed())
	}, 10*time.Second, 100*time.Millisecond).Should(Succeed())
}

// Mock the ManagedServiceAccount behavior by creating the token secret directly
func createMockTokenSecret(ctx context.Context, migrationName, hubNamespace string) {
	// This simulates what ManagedServiceAccount would create
	tokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      migrationName,
			Namespace: hubNamespace,
			Labels: map[string]string{
				"authentication.open-cluster-management.io/managed-by": "managed-serviceaccount",
			},
			Annotations: map[string]string{
				"kubernetes.io/service-account.name": "default",
			},
		},
		Type: corev1.SecretTypeServiceAccountToken,
		Data: map[string][]byte{
			"token":  []byte("mock-token-data"),
			"ca.crt": []byte("mock-ca-cert"),
		},
	}

	err := mgr.GetClient().Create(ctx, tokenSecret)
	// Ignore already exists error
	if err != nil && !errors.IsAlreadyExists(err) {
		Expect(err).To(Succeed())
	}
}
