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

// go test /test/integration/manager/status -v -ginkgo.focus "ManagedClusterHandler"
var _ = Describe("ManagedClusterHandler", Ordered, func() {
	It("should be able to sync managed cluster event", func() {
		By("Create event")
		leafHubName := "hub1"
		version := eventversion.NewVersion()
		version.Incr()

		clusterID := "3f406177-34b2-4852-88dd-ff2809680335"
		cluster := &clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testManagedCluster",
				Namespace: "default",
				UID:       types.UID(clusterID),
			},
			Status: clusterv1.ManagedClusterStatus{
				ClusterClaims: []clusterv1.ManagedClusterClaim{
					{
						Name:  constants.ClusterIdClaimName,
						Value: clusterID,
					},
				},
			},
		}
		bundle := generic.GenericBundle[clusterv1.ManagedCluster]{}
		bundle.Create = []clusterv1.ManagedCluster{*cluster}
		evt := ToCloudEvent(leafHubName, string(enum.ManagedClusterType), version, bundle)

		By("Sync event with transport")
		err := producer.SendEvent(ctx, *evt)
		Expect(err).Should(Succeed())

		By("Check the leaf hubs table")
		Eventually(func() error {
			db := database.GetGorm()

			items := []models.ManagedCluster{}
			if err := db.Where("leaf_hub_name = ?", leafHubName).Find(&items).Error; err != nil {
				return err
			}

			for _, item := range items {
				fmt.Println("ManagedCluster", item.LeafHubName, item.ClusterID)
				if item.ClusterID == clusterID {
					return nil
				}
			}
			return fmt.Errorf("not found expected resource on the table")
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})
})
