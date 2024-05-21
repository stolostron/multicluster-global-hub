package dbsyncer_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

// go test ./manager/pkg/statussyncer/syncers -v -ginkgo.focus "ManagedClusterEventHandler"
var _ = Describe("ManagedClusterEventHandler", Ordered, func() {
	It("should be able to sync replicate policy event", func() {
		By("Create hubClusterInfo event")

		leafHubName := "hub1"
		version := eventversion.NewVersion()
		version.Incr()

		data := event.ManagedClusterEventBundle{}
		data = append(data, models.ManagedClusterEvent{
			EventNamespace:      "managed-cluster1",
			EventName:           "managed-cluster1.17cd5c3642c43a8a",
			ClusterID:           "",
			LeafHubName:         "hub1",
			ClusterName:         "managed-cluster1",
			Message:             "The managed cluster (managed-cluster1) cannot connect to the hub cluster.",
			Reason:              "AvailableUnknown",
			ReportingController: "registration-controller",
			ReportingInstance:   "registration-controller-cluster-manager-registration-controller-6794cf54d9-j7lgm",
			EventType:           "Warning",
			CreatedAt:           time.Now(),
		})

		evt := ToCloudEvent(leafHubName, string(enum.ManagedClusterEventType), version, data)

		By("Sync event with transport")
		err := producer.SendEvent(ctx, *evt)
		Expect(err).Should(Succeed())

		By("Check the leaf hubs table")
		Eventually(func() error {
			db := database.GetGorm()
			items := []models.ManagedClusterEvent{}
			if err := db.Find(&items).Error; err != nil {
				return err
			}

			count := 0
			for _, item := range items {
				fmt.Println(">> ", item.LeafHubName, item.ClusterName, item.EventName, item.Message, item.CreatedAt)
				count++
			}
			if count > 0 {
				return nil
			}
			return fmt.Errorf("not found expected resource on the table")
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})
})
