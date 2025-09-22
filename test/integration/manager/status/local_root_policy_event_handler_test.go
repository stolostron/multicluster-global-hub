package status

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

// go test /test/integration/manager/status -v -ginkgo.focus "LocalEventPolicyHandler"
var _ = Describe("LocalEventPolicyHandler", Ordered, func() {
	It("should be able to sync root policy event", func() {
		By("Create hubClusterInfo event")

		leafHubName := "hub1"
		version := eventversion.NewVersion()
		version.Incr()

		data := event.RootPolicyEventBundle{}
		data = append(data, &event.RootPolicyEvent{
			BaseEvent: event.BaseEvent{
				EventName:      "policy-limitrange.17b8363660d39188",
				EventNamespace: "local-policy-namespace",
				Message:        "Policy local-policy-namespace/policy-limitrange was propagated to cluster kind-hub2-cluster1/kind-hub2-cluster1",
				Reason:         "PolicyPropagation",
				Source: v1.EventSource{
					Component: "policy-propagator",
				},
				CreatedAt: time.Now(),
			},
			PolicyID:   "13b2e003-2bdf-4c82-9bdf-f1aa7ccf608d",
			Compliance: "NonCompliant",
		})

		evt := ToCloudEvent(leafHubName, string(enum.LocalRootPolicyEventType), version, data)

		By("Sync event with transport")
		err := producer.SendEvent(ctx, *evt)
		Expect(err).Should(Succeed())

		By("Check the leaf hubs table")
		Eventually(func() error {
			db := database.GetGorm()
			items := []models.LocalRootPolicyEvent{}
			if err := db.Find(&items).Error; err != nil {
				return err
			}

			count := 0
			for _, item := range items {
				fmt.Println(item.LeafHubName, item.EventName, item.Message)
				count++
			}
			if count > 0 {
				return nil
			}
			return fmt.Errorf("not found expected resource on the table")
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})
})
