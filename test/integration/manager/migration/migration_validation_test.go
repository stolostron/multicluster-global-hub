package migration_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var _ = Describe("ManagedClusterMigration Validation", func() {
	var (
		testCtx       context.Context
		fromHub       = "hub1"
		toHub         = "hub2"
		testName      string
		testNamespace string
	)

	BeforeEach(func() {
		testCtx = context.Background()
		testName = "test-migration-" + time.Now().Format("20060102-150405-000")
		testNamespace = utils.GetDefaultNamespace()
	})

	AfterEach(func() {
		if err := cleanupMigrationCR(context.Background(), testName, testNamespace); err != nil {
			fmt.Printf("failed to cleanup migration CR: %v\n", err)
		}
	})

	It("should reject migration with neither field specified", func() {
		migration := &migrationv1alpha1.ManagedClusterMigration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testName,
				Namespace: testNamespace,
			},
			Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
				From: fromHub,
				To:   toHub,
				// Neither IncludedManagedClusters nor IncludedManagedClustersPlacementRef is specified
			},
		}

		err := mgr.GetClient().Create(testCtx, migration)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("exactly one of includedManagedClustersPlacementRef or includedManagedClusters must be specified"))
	})

	It("should reject migration with both fields specified", func() {
		migration := &migrationv1alpha1.ManagedClusterMigration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testName,
				Namespace: testNamespace,
			},
			Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
				From:                                fromHub,
				To:                                  toHub,
				IncludedManagedClusters:             []string{"cluster1"},
				IncludedManagedClustersPlacementRef: "test-placement",
			},
		}

		err := mgr.GetClient().Create(testCtx, migration)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("exactly one of includedManagedClustersPlacementRef or includedManagedClusters must be specified"))
	})

	It("should accept migration with only includedManagedClusters", func() {
		migration := &migrationv1alpha1.ManagedClusterMigration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testName,
				Namespace: testNamespace,
			},
			Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
				From:                    fromHub,
				To:                      toHub,
				IncludedManagedClusters: []string{"cluster1"},
			},
		}

		err := mgr.GetClient().Create(testCtx, migration)
		Expect(err).NotTo(HaveOccurred())

		// Verify the migration was created successfully
		created := &migrationv1alpha1.ManagedClusterMigration{}
		Eventually(func() error {
			return mgr.GetClient().Get(testCtx,
				types.NamespacedName{Name: testName, Namespace: testNamespace},
				created)
		}, "10s", "100ms").Should(Succeed())

		Expect(created.Spec.IncludedManagedClusters).To(Equal([]string{"cluster1"}))
		Expect(created.Spec.IncludedManagedClustersPlacementRef).To(BeEmpty())
	})

	It("should accept migration with only includedManagedClustersPlacementRef", func() {
		migration := &migrationv1alpha1.ManagedClusterMigration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testName,
				Namespace: testNamespace,
			},
			Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
				From:                                fromHub,
				To:                                  toHub,
				IncludedManagedClustersPlacementRef: "test-placement",
			},
		}

		err := mgr.GetClient().Create(testCtx, migration)
		Expect(err).NotTo(HaveOccurred())

		// Verify the migration was created successfully
		created := &migrationv1alpha1.ManagedClusterMigration{}
		Eventually(func() error {
			return mgr.GetClient().Get(testCtx,
				types.NamespacedName{Name: testName, Namespace: testNamespace},
				created)
		}, "10s", "100ms").Should(Succeed())

		Expect(created.Spec.IncludedManagedClustersPlacementRef).To(Equal("test-placement"))
		Expect(created.Spec.IncludedManagedClusters).To(BeEmpty())
	})
})
