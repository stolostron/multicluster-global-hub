package status

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

// go test ./test/integration/manager/status -v -ginkgo.focus "ManagedClusterHandler"
var _ = Describe("ManagedClusterHandler", Ordered, func() {
	var version *eventversion.Version
	var clusterID1 string
	var clusterID2 string
	BeforeEach(func() {
		if version == nil {
			version = eventversion.NewVersion()
		}
		clusterID1 = "3f406177-34b2-4852-88dd-ff2809680331"
		clusterID2 = "3f406177-34b2-4852-88dd-ff2809680332"
	})

	It("should be able to sync managed cluster event", func() {
		By("Create event")
		leafHubName := "hub1"
		version.Incr()

		bundle := generic.GenericBundle[clusterv1.ManagedCluster]{}
		bundle.Create = []clusterv1.ManagedCluster{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cluster1",
					Namespace: "cluster1",
					UID:       types.UID(clusterID1),
				},
				Status: clusterv1.ManagedClusterStatus{
					ClusterClaims: []clusterv1.ManagedClusterClaim{
						{
							Name:  constants.ClusterIdClaimName,
							Value: clusterID1,
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cluster2",
					Namespace: "cluster2",
					UID:       types.UID(clusterID2),
				},
				Status: clusterv1.ManagedClusterStatus{
					ClusterClaims: []clusterv1.ManagedClusterClaim{
						{
							Name:  constants.ClusterIdClaimName,
							Value: clusterID2,
						},
					},
				},
			},
		}
		evt := ToCloudEvent(leafHubName, string(enum.ManagedClusterType), version, bundle)

		By("Sync event with transport")
		err := producer.SendEvent(ctx, *evt)
		version.Next()
		Expect(err).Should(Succeed())

		By("Check the leaf hubs table")
		Eventually(func() error {
			db := database.GetGorm()

			items := []models.ManagedCluster{}
			if err := db.Where("leaf_hub_name = ?", leafHubName).Find(&items).Error; err != nil {
				return err
			}

			count := 0
			for _, item := range items {
				fmt.Println("ManagedCluster Creating", item.LeafHubName, item.ClusterID)
				if item.ClusterID == clusterID1 || item.ClusterID == clusterID2 {
					count += 1
				}

			}
			if count != 2 {
				return fmt.Errorf("not found expected resource on the table")
			}
			return nil
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})

	It("should be able to resync the managed clusters", func() {
		By("Create event")
		leafHubName := "hub1"
		version.Incr()

		bundle := generic.GenericBundle[clusterv1.ManagedCluster]{}
		bundle.ResyncMetadata = []generic.ObjectMetadata{
			{
				Namespace: "cluster2",
				Name:      "cluster2",
				ID:        clusterID2,
			},
		}
		evt := ToCloudEvent(leafHubName, string(enum.ManagedClusterType), version, bundle)

		By("Sync event with transport")
		err := producer.SendEvent(ctx, *evt)
		version.Next()
		Expect(err).Should(Succeed())

		By("Check the leaf hubs table")
		Eventually(func() error {
			db := database.GetGorm()

			items := []models.ManagedCluster{}
			if err := db.Where("leaf_hub_name = ?", leafHubName).Find(&items).Error; err != nil {
				return err
			}

			for _, item := range items {
				fmt.Println("ManagedCluster Resync", item.LeafHubName, item.ClusterID)
				if item.ClusterID == clusterID1 {
					return fmt.Errorf("should delete the cluster %s", clusterID1)
				}
			}
			return nil
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})

	It("should be able to delete managed clusters", func() {
		By("Create event")
		leafHubName := "hub1"
		version.Incr()

		bundle := generic.GenericBundle[clusterv1.ManagedCluster]{}
		bundle.Delete = []generic.ObjectMetadata{
			{
				Namespace: "cluster2",
				Name:      "cluster2",
				ID:        clusterID2,
			},
		}
		evt := ToCloudEvent(leafHubName, string(enum.ManagedClusterType), version, bundle)

		By("Sync event with transport")
		err := producer.SendEvent(ctx, *evt)
		version.Next()
		Expect(err).Should(Succeed())

		By("Check the leaf hubs table")
		Eventually(func() error {
			db := database.GetGorm()

			items := []models.ManagedCluster{}
			if err := db.Where("leaf_hub_name = ?", leafHubName).Find(&items).Error; err != nil {
				return fmt.Errorf("failed to get the managed clusters: %v", err)
			}

			fmt.Println("ManagedCluster Delete", items)
			for _, item := range items {
				if item.ClusterID == clusterID2 {
					return fmt.Errorf("should delete the cluster %s", clusterID2)
				}
			}
			return nil
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})
})
