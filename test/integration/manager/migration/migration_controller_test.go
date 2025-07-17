package migration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/migration"
	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const (
	FROM_HUB = "hub1"
	TO_HUB   = "hub2"
)

var _ = Describe("ManagedCluster Migration Controller", func() {
	var testCtx context.Context
	var testCtxCancel context.CancelFunc

	BeforeEach(func() {
		testCtx, testCtxCancel = context.WithCancel(ctx)

		// Create namespaces and other required resources for hubs
		createTestResources(testCtx, FROM_HUB)
		createTestResources(testCtx, TO_HUB)
	})

	AfterEach(func() {
		// Clean up created resources
		deleteTestResources(testCtx, FROM_HUB)
		deleteTestResources(testCtx, TO_HUB)
		testCtxCancel()
	})

	Context("When validating the migration spec", func() {
		It("should fail if the 'from' hub is not found in the database", func() {
			m := createMigrationCR(testCtx, "validation-fail-hub", []string{"cluster1"}, FROM_HUB, TO_HUB)
			assertMigrationCondition(testCtx, m, migrationv1alpha1.ConditionTypeValidated, metav1.ConditionFalse, migration.ConditionReasonHubClusterNotFound)
		})

		It("should fail if a managed cluster is not found in the database", func() {
			createTestHub(testCtx, FROM_HUB)
			createTestHub(testCtx, TO_HUB)
			m := createMigrationCR(testCtx, "validation-fail-cluster", []string{"nonexistent-cluster"}, FROM_HUB, TO_HUB)
			assertMigrationCondition(testCtx, m, migrationv1alpha1.ConditionTypeValidated, metav1.ConditionFalse, migration.ConditionReasonClusterNotFound)
		})

		It("should succeed when all resources exist", func() {
			createTestHub(testCtx, FROM_HUB)
			createTestHub(testCtx, TO_HUB)
			createTestManagedCluster(testCtx, "cluster1", FROM_HUB)
			m := createMigrationCR(testCtx, "validation-success", []string{"cluster1"}, FROM_HUB, TO_HUB)
			assertMigrationCondition(testCtx, m, migrationv1alpha1.ConditionTypeValidated, metav1.ConditionTrue, migration.ConditionReasonResourceValidated)
		})
	})

	Context("When initializing the migration", func() {
		var m *migrationv1alpha1.ManagedClusterMigration

		BeforeEach(func() {
			createTestHub(testCtx, FROM_HUB)
			createTestHub(testCtx, TO_HUB)
			createTestManagedCluster(testCtx, "cluster1", FROM_HUB)
			m = createMigrationCR(testCtx, "initialization-test", []string{"cluster1"}, FROM_HUB, TO_HUB)
			// Ensure validation passes first
			assertMigrationCondition(testCtx, m, migrationv1alpha1.ConditionTypeValidated, metav1.ConditionTrue, migration.ConditionReasonResourceValidated)
		})

		It("should remain in Initializing phase with 'Waiting' reason if hubs have not confirmed", func() {
			assertMigrationCondition(testCtx, m, migrationv1alpha1.ConditionTypeInitialized, metav1.ConditionFalse, migration.ConditionReasonWaiting)
			Consistently(func() string {
				Expect(mgr.GetClient().Get(testCtx, client.ObjectKeyFromObject(m), m)).To(Succeed())
				return m.Status.Phase
			}, 5*time.Second, 1*time.Second).ShouldNot(Equal(migrationv1alpha1.PhaseFailed))
		})

		It("should update status to Initialized once both hubs confirm", func() {
			Eventually(func(g Gomega) {
				g.Expect(mgr.GetClient().Get(testCtx, client.ObjectKeyFromObject(m), m)).To(Succeed())
				g.Expect(m.GetUID()).NotTo(BeEmpty())
			}).Should(Succeed())

			simulateHubConfirmation(m.GetUID(), FROM_HUB, migrationv1alpha1.PhaseInitializing)
			simulateHubConfirmation(m.GetUID(), TO_HUB, migrationv1alpha1.PhaseInitializing)
			assertMigrationCondition(testCtx, m, migrationv1alpha1.ConditionTypeInitialized, metav1.ConditionTrue, migration.ConditionReasonResourceInitialized)
		})
	})

	Context("When deploying the resources", func() {
		var m *migrationv1alpha1.ManagedClusterMigration

		BeforeEach(func() {
			createTestHub(testCtx, FROM_HUB)
			createTestHub(testCtx, TO_HUB)
			createTestManagedCluster(testCtx, "cluster1", FROM_HUB)
			m = createMigrationCR(testCtx, "deploying-test", []string{"cluster1"}, FROM_HUB, TO_HUB)
			// Fast-forward to deploying phase
			assertMigrationCondition(testCtx, m, migrationv1alpha1.ConditionTypeValidated, metav1.ConditionTrue, migration.ConditionReasonResourceValidated)

			Eventually(func(g Gomega) {
				g.Expect(mgr.GetClient().Get(testCtx, client.ObjectKeyFromObject(m), m)).To(Succeed())
				g.Expect(m.GetUID()).NotTo(BeEmpty())
			}).Should(Succeed())

			simulateHubConfirmation(m.GetUID(), FROM_HUB, migrationv1alpha1.PhaseInitializing)
			simulateHubConfirmation(m.GetUID(), TO_HUB, migrationv1alpha1.PhaseInitializing)
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

			simulateHubConfirmation(m.GetUID(), FROM_HUB, migrationv1alpha1.PhaseDeploying)
			simulateHubConfirmation(m.GetUID(), TO_HUB, migrationv1alpha1.PhaseDeploying)
			assertMigrationCondition(testCtx, m, migrationv1alpha1.ConditionTypeDeployed, metav1.ConditionTrue, migration.ConditionReasonResourcesDeployed)
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
	Expect(mgr.GetClient().Create(ctx, m)).To(Succeed())
	return m
}

func createTestResources(ctx context.Context, hubName string) {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: hubName}}
	err := mgr.GetClient().Create(ctx, ns)
	if !errors.IsAlreadyExists(err) {
		Expect(err).NotTo(HaveOccurred())
	}
}

func deleteTestResources(ctx context.Context, hubName string) {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: hubName}}
	Expect(mgr.GetClient().Delete(ctx, ns)).To(Succeed())
	// Clean up database entries
	Expect(db.Exec("DELETE FROM spec.leaf_hubs WHERE leaf_hub_name = ?", hubName).Error).To(Succeed())
	Expect(db.Exec("DELETE FROM spec.managed_clusters WHERE leaf_hub_name = ?", hubName).Error).To(Succeed())
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

	// Create in DB
	payload, err := json.Marshal(mc)
	Expect(err).To(Succeed())
	err = db.Model(&models.ManagedCluster{}).Create(&models.ManagedCluster{
		LeafHubName: hubName,
		Payload:     payload,
		Error:       database.ErrorNone,
	}).Error
	Expect(err).To(Succeed())
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
	}, 30*time.Second, 100*time.Millisecond).Should(Succeed())
}

func simulateHubConfirmation(uid types.UID, hubName string, phase string) {
	migration.SetFinished(string(uid), hubName, phase)
}
