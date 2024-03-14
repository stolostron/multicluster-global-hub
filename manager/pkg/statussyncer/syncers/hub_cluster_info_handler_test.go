package dbsyncer_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/cluster"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

// go test ./manager/pkg/statussyncer/syncers -v -ginkgo.focus "HubClusterInfoHandler"
var _ = Describe("HubClusterInfoHandler", Ordered, func() {
	const (
		leafHubName = "hub1"
		routeHost   = "console-openshift-console.apps.test-cluster"
	)

	It("should handle the hub cluster info event", func() {
		By("Create hubClusterInfo event")
		data := cluster.HubClusterInfo{
			ConsoleURL: routeHost,
			ClusterId:  "00000000-0000-0000-0000-000000000001",
		}

		version := eventversion.NewVersion()
		version.Incr()
		evt := ToCloudEvent(leafHubName, string(enum.HubClusterInfoType), version, data)

		By("Sync event with transport")
		err := producer.SendEvent(ctx, *evt)
		Expect(err).Should(Succeed())

		By("Check the leaf hubs table")
		Eventually(func() error {
			db := database.GetGorm()
			leafHubs := []models.LeafHub{}
			if err := db.Find(&leafHubs).Error; err != nil {
				return err
			}

			count := 0
			for _, hub := range leafHubs {
				fmt.Println(hub.LeafHubName, hub.ClusterID, hub.Payload)
				count++
			}
			if count > 0 {
				return nil
			}
			return fmt.Errorf("not found expected resource on the table")
		}, 30*time.Second, 100*time.Microsecond).ShouldNot(HaveOccurred())
	})
})
