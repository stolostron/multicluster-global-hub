package dbsyncer_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

// go test ./manager/pkg/statussyncer/syncers -v -ginkgo.focus "LocalPlacementRuleHandler"
var _ = Describe("LocalPlacementRuleHandler", Ordered, func() {

	It("should be able to sync local placement rule event", func() {

		By("Create event")
		leafHubName := "hub1"
		version := metadata.NewBundleVersion()
		version.Incr()

		data := generic.GenericObjectData{}
		data = append(data, &placementrulev1.PlacementRule{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-placementrule-1",
				Namespace: "default",
				UID:       "f47ac10b-58cc-4372-a567-0e02b2c3d479",
			},
			Spec: placementrulev1.PlacementRuleSpec{
				SchedulerName: constants.GlobalHubSchedulerName,
			},
		})

		evt := ToCloudEvent(leafHubName, string(enum.LocalPlacementRuleSpecType), version, data)

		By("Sync event with transport")
		err := producer.SendEvent(ctx, *evt)
		Expect(err).Should(Succeed())

		By("Check the leaf hubs table")
		Eventually(func() error {
			db := database.GetGorm()

			items := []models.LocalSpecPlacementRule{}
			if err := db.Find(&items).Error; err != nil {
				return err
			}

			count := 0
			for _, item := range items {
				fmt.Println(item.LeafHubName, item.ID, item.Payload)
				count++
			}
			if count > 0 {
				return nil
			}
			return fmt.Errorf("not found expected resource on the table")
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})
})
