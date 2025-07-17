package migration_test

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/migration"
	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var _ = Describe("Migration Error Handling and Edge Cases", func() {
	var testCtx context.Context
	var testCtxCancel context.CancelFunc
	var testID string
	var fromHub, toHub string

	BeforeEach(func() {
		testCtx, testCtxCancel = context.WithCancel(ctx)
		testID = fmt.Sprintf("error-%d", time.Now().UnixNano())
		fromHub = fmt.Sprintf("hub1-%s", testID)
		toHub = fmt.Sprintf("hub2-%s", testID)

		// Set up basic test environment
		createTestResources(testCtx, fromHub)
		createTestResources(testCtx, toHub)
		createTestHub(testCtx, fromHub)
		createTestHub(testCtx, toHub)
		createTestManagedCluster(testCtx, fmt.Sprintf("cluster1-%s", testID), fromHub)
	})

	AfterEach(func() {
		cleanupMigrationResources(testCtx, testID)
		deleteTestResources(testCtx, fromHub)
		deleteTestResources(testCtx, toHub)
		testCtxCancel()
		time.Sleep(100 * time.Millisecond)
	})

	Describe("Validation Phase Error Handling", func() {
		It("should handle missing source hub gracefully", func() {
			By("Creating migration with non-existent source hub")
			nonExistentHub := fmt.Sprintf("nonexistent-hub-%s", testID)
			m := createMigrationCR(testCtx, fmt.Sprintf("missing-source-%s", testID),
				[]string{fmt.Sprintf("cluster1-%s", testID)}, nonExistentHub, toHub)

			By("Verifying migration fails with appropriate error")
			assertMigrationCondition(testCtx, m, migrationv1alpha1.ConditionTypeValidated,
				metav1.ConditionFalse, migration.ConditionReasonHubClusterNotFound)
			assertMigrationPhase(testCtx, m, migrationv1alpha1.PhaseFailed)
		})

		It("should handle missing target hub gracefully", func() {
			By("Creating migration with non-existent target hub")
			nonExistentHub := fmt.Sprintf("nonexistent-hub-%s", testID)
			m := createMigrationCR(testCtx, fmt.Sprintf("missing-target-%s", testID),
				[]string{fmt.Sprintf("cluster1-%s", testID)}, fromHub, nonExistentHub)

			By("Verifying migration fails with appropriate error")
			assertMigrationCondition(testCtx, m, migrationv1alpha1.ConditionTypeValidated,
				metav1.ConditionFalse, migration.ConditionReasonHubClusterNotFound)
			assertMigrationPhase(testCtx, m, migrationv1alpha1.PhaseFailed)
		})

		It("should handle missing managed cluster gracefully", func() {
			By("Creating migration with non-existent managed cluster")
			nonExistentCluster := fmt.Sprintf("nonexistent-cluster-%s", testID)
			m := createMigrationCR(testCtx, fmt.Sprintf("missing-cluster-%s", testID),
				[]string{nonExistentCluster}, fromHub, toHub)

			By("Verifying migration fails with appropriate error")
			assertMigrationCondition(testCtx, m, migrationv1alpha1.ConditionTypeValidated,
				metav1.ConditionFalse, migration.ConditionReasonClusterNotFound)
			assertMigrationPhase(testCtx, m, migrationv1alpha1.PhaseFailed)
		})

		It("should handle cluster already in target hub", func() {
			By("Creating cluster in target hub")
			conflictCluster := fmt.Sprintf("conflict-cluster-%s", testID)
			createTestManagedCluster(testCtx, conflictCluster, toHub)

			By("Creating migration for cluster already in target hub")
			m := createMigrationCR(testCtx, fmt.Sprintf("conflict-migration-%s", testID),
				[]string{conflictCluster}, fromHub, toHub)

			By("Verifying migration fails with conflict error")
			assertMigrationCondition(testCtx, m, migrationv1alpha1.ConditionTypeValidated,
				metav1.ConditionFalse, migration.ConditionReasonClusterConflict)
			assertMigrationPhase(testCtx, m, migrationv1alpha1.PhaseFailed)
		})

		It("should handle invalid resource formats", func() {
			By("Creating migration with invalid resource format")
			m := &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("invalid-resources-%s", testID),
					Namespace: utils.GetDefaultNamespace(),
				},
				Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
					IncludedManagedClusters: []string{fmt.Sprintf("cluster1-%s", testID)},
					From:                    fromHub,
					To:                      toHub,
					IncludedResources: []string{
						"invalid-format",
						"configmap/default", // Missing name
						"unsupported/default/test",
					},
				},
			}

			Expect(mgr.GetClient().Create(testCtx, m)).To(Succeed())

			By("Verifying migration fails with resource validation error")
			assertMigrationCondition(testCtx, m, migrationv1alpha1.ConditionTypeValidated,
				metav1.ConditionFalse, migration.ConditionReasonResourceInvalid)
			assertMigrationPhase(testCtx, m, migrationv1alpha1.PhaseFailed)
		})
	})

	Describe("Phase Transition Error Handling", func() {
		It("should handle hub errors during initialization", func() {
			By("Creating successful migration")
			m := createMigrationCR(testCtx, fmt.Sprintf("init-error-%s", testID),
				[]string{fmt.Sprintf("cluster1-%s", testID)}, fromHub, toHub)

			By("Verifying migration reaches initialization phase")
			assertMigrationCondition(testCtx, m, migrationv1alpha1.ConditionTypeValidated,
				metav1.ConditionTrue, migration.ConditionReasonResourceValidated)
			assertMigrationPhase(testCtx, m, migrationv1alpha1.PhaseInitializing)

			By("Simulating hub error during initialization")
			createMockTokenSecret(testCtx, m.Name, toHub)
			simulateHubError(m.GetUID(), fromHub, migrationv1alpha1.PhaseInitializing, "initialization failed")

			By("Verifying migration handles error appropriately")
			assertMigrationCondition(testCtx, m, migrationv1alpha1.ConditionTypeInitialized,
				metav1.ConditionFalse, migration.ConditionReasonError)
			assertMigrationPhase(testCtx, m, migrationv1alpha1.PhaseCleaning)
		})

		It("should handle hub errors during deployment", func() {
			By("Creating and progressing migration to deployment phase")
			m := createMigrationCR(testCtx, fmt.Sprintf("deploy-error-%s", testID),
				[]string{fmt.Sprintf("cluster1-%s", testID)}, fromHub, toHub)

			// Progress to deployment phase
			progressToPhase(testCtx, m, migrationv1alpha1.PhaseDeploying)

			By("Simulating hub error during deployment")
			simulateHubError(m.GetUID(), toHub, migrationv1alpha1.PhaseDeploying, "deployment failed")

			By("Verifying migration handles error appropriately")
			assertMigrationCondition(testCtx, m, migrationv1alpha1.ConditionTypeDeployed,
				metav1.ConditionFalse, migration.ConditionReasonError)
			assertMigrationPhase(testCtx, m, migrationv1alpha1.PhaseCleaning)
		})

		It("should handle hub errors during registration", func() {
			By("Creating and progressing migration to registration phase")
			m := createMigrationCR(testCtx, fmt.Sprintf("register-error-%s", testID),
				[]string{fmt.Sprintf("cluster1-%s", testID)}, fromHub, toHub)

			// Progress to registration phase
			progressToPhase(testCtx, m, migrationv1alpha1.PhaseRegistering)

			By("Simulating hub error during registration")
			simulateHubError(m.GetUID(), fromHub, migrationv1alpha1.PhaseRegistering, "registration failed")

			By("Verifying migration handles error appropriately")
			assertMigrationCondition(testCtx, m, migrationv1alpha1.ConditionTypeRegistered,
				metav1.ConditionFalse, migration.ConditionReasonError)
			assertMigrationPhase(testCtx, m, migrationv1alpha1.PhaseCleaning)
		})

		It("should handle cleanup errors gracefully", func() {
			By("Creating and progressing migration to cleanup phase")
			m := createMigrationCR(testCtx, fmt.Sprintf("cleanup-error-%s", testID),
				[]string{fmt.Sprintf("cluster1-%s", testID)}, fromHub, toHub)

			// Progress to cleanup phase
			progressToPhase(testCtx, m, migrationv1alpha1.PhaseCleaning)

			By("Simulating hub error during cleanup")
			simulateHubError(m.GetUID(), toHub, migrationv1alpha1.PhaseCleaning, "cleanup failed")

			By("Verifying migration handles cleanup error appropriately")
			assertMigrationCondition(testCtx, m, migrationv1alpha1.ConditionTypeCleaned,
				metav1.ConditionFalse, migration.ConditionReasonError)
			assertMigrationPhase(testCtx, m, migrationv1alpha1.PhaseFailed)
		})
	})

	Describe("Concurrency and Race Conditions", func() {
		It("should handle concurrent migrations properly", func() {
			By("Creating multiple migrations simultaneously")
			migrations := []*migrationv1alpha1.ManagedClusterMigration{}
			for i := 0; i < 3; i++ {
				m := createMigrationCR(testCtx, fmt.Sprintf("concurrent-%d-%s", i, testID),
					[]string{fmt.Sprintf("cluster1-%s", testID)}, fromHub, toHub)
				migrations = append(migrations, m)
			}

			By("Verifying only one migration progresses while others wait")
			var activeMigration *migrationv1alpha1.ManagedClusterMigration
			var waitingMigrations []*migrationv1alpha1.ManagedClusterMigration

			for _, m := range migrations {
				Eventually(func() string {
					Expect(mgr.GetClient().Get(testCtx, client.ObjectKeyFromObject(m), m)).To(Succeed())
					return m.Status.Phase
				}, 10*time.Second, 500*time.Millisecond).Should(Or(
					Equal(migrationv1alpha1.PhaseValidating),
					Equal(migrationv1alpha1.PhasePending),
				))

				if m.Status.Phase == migrationv1alpha1.PhaseValidating {
					activeMigration = m
				} else {
					waitingMigrations = append(waitingMigrations, m)
				}
			}

			By("Verifying exactly one migration is active")
			Expect(activeMigration).NotTo(BeNil())
			Expect(len(waitingMigrations)).To(Equal(2))

			By("Completing active migration and verifying next one starts")
			completeFullMigration(testCtx, activeMigration)

			Eventually(func() bool {
				for _, m := range waitingMigrations {
					Expect(mgr.GetClient().Get(testCtx, client.ObjectKeyFromObject(m), m)).To(Succeed())
					if m.Status.Phase == migrationv1alpha1.PhaseValidating {
						return true
					}
				}
				return false
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())
		})
	})

	Describe("Migration State Recovery", func() {
		It("should recover from interrupted migrations", func() {
			By("Creating migration and progressing to mid-state")
			m := createMigrationCR(testCtx, fmt.Sprintf("recovery-%s", testID),
				[]string{fmt.Sprintf("cluster1-%s", testID)}, fromHub, toHub)

			// Progress to initialization phase
			progressToPhase(testCtx, m, migrationv1alpha1.PhaseInitializing)

			By("Simulating partial hub confirmations")
			simulateHubConfirmation(m.GetUID(), fromHub, migrationv1alpha1.PhaseInitializing)
			// Don't confirm toHub to simulate interruption

			By("Verifying migration waits for remaining confirmations")
			Consistently(func() string {
				Expect(mgr.GetClient().Get(testCtx, client.ObjectKeyFromObject(m), m)).To(Succeed())
				return m.Status.Phase
			}, 3*time.Second, 500*time.Millisecond).Should(Equal(migrationv1alpha1.PhaseInitializing))

			By("Completing remaining confirmations")
			simulateHubConfirmation(m.GetUID(), toHub, migrationv1alpha1.PhaseInitializing)

			By("Verifying migration continues normally")
			assertMigrationCondition(testCtx, m, migrationv1alpha1.ConditionTypeInitialized,
				metav1.ConditionTrue, migration.ConditionReasonResourceInitialized)
			assertMigrationPhase(testCtx, m, migrationv1alpha1.PhaseDeploying)
		})

		It("should handle migration event status edge cases", func() {
			By("Testing event status functions with edge cases")
			uid := fmt.Sprintf("edge-case-uid-%s", testID)

			// Test with empty strings
			migration.SetErrorMessage(uid, "", migrationv1alpha1.PhaseInitializing, "")
			Expect(migration.GetErrorMessage(uid, "", migrationv1alpha1.PhaseInitializing)).To(Equal(""))

			// Test with special characters
			specialHub := "hub-with-special-chars_123.test"
			migration.SetStarted(uid, specialHub, migrationv1alpha1.PhaseInitializing)
			Expect(migration.GetStarted(uid, specialHub, migrationv1alpha1.PhaseInitializing)).To(BeTrue())

			// Test with long strings
			longErrorMsg := strings.Repeat("error ", 100)
			migration.SetErrorMessage(uid, fromHub, migrationv1alpha1.PhaseInitializing, longErrorMsg)
			Expect(migration.GetErrorMessage(uid, fromHub, migrationv1alpha1.PhaseInitializing)).To(Equal(longErrorMsg))
		})
	})

	Describe("Resource Validation Edge Cases", func() {
		It("should handle edge cases in resource validation", func() {
			testCases := []struct {
				name       string
				resources  []string
				shouldFail bool
			}{
				{
					name:       "empty resource list",
					resources:  []string{},
					shouldFail: false,
				},
				{
					name:       "resource with dots in name",
					resources:  []string{"configmap/default/kube-root-ca.crt"},
					shouldFail: false,
				},
				{
					name:       "resource with dashes and underscores",
					resources:  []string{"secret/kube-system/test-secret_123"},
					shouldFail: false,
				},
				{
					name:       "resource with invalid characters",
					resources:  []string{"configmap/default/invalid@name"},
					shouldFail: true,
				},
				{
					name:       "resource with wildcard",
					resources:  []string{"secret/default/*"},
					shouldFail: true,
				},
				{
					name:       "resource with capital letters in kind",
					resources:  []string{"ConfigMap/default/test"},
					shouldFail: false, // Should be normalized to lowercase
				},
			}

			for _, tc := range testCases {
				By(fmt.Sprintf("Testing %s", tc.name))
				m := &migrationv1alpha1.ManagedClusterMigration{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("resource-test-%s-%s", strings.ReplaceAll(tc.name, " ", "-"), testID),
						Namespace: utils.GetDefaultNamespace(),
					},
					Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
						IncludedManagedClusters: []string{fmt.Sprintf("cluster1-%s", testID)},
						From:                    fromHub,
						To:                      toHub,
						IncludedResources:       tc.resources,
					},
				}

				Expect(mgr.GetClient().Create(testCtx, m)).To(Succeed())

				if tc.shouldFail {
					assertMigrationCondition(testCtx, m, migrationv1alpha1.ConditionTypeValidated,
						metav1.ConditionFalse, migration.ConditionReasonResourceInvalid)
				} else {
					assertMigrationCondition(testCtx, m, migrationv1alpha1.ConditionTypeValidated,
						metav1.ConditionTrue, migration.ConditionReasonResourceValidated)
				}
			}
		})
	})
})

// Helper functions for error handling tests - functions are shared in migration_test_helpers.go
