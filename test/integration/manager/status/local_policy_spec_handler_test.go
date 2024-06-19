package status

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

// go test /test/integration/manager/status -v -ginkgo.focus "LocalPolicySpecHandler"
var _ = Describe("LocalPolicySpecHandler", Ordered, func() {
	It("should be able to sync local policy event", func() {
		By("Create event")
		leafHubName := "hub1"
		version := eventversion.NewVersion()
		version.Incr()

		data := generic.GenericObjectBundle{}
		policy := &policiesv1.Policy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testLocalPolicy",
				Namespace: "default",
				UID:       types.UID(uuid.New().String()),
			},
			Spec: policiesv1.PolicySpec{},
		}
		data = append(data, policy)
		evt := ToCloudEvent(leafHubName, string(enum.LocalPolicySpecType), version, data)

		By("Sync event with transport")
		err := producer.SendEvent(ctx, *evt)
		Expect(err).Should(Succeed())

		By("Check the leaf hubs table")
		Eventually(func() error {
			db := database.GetGorm()

			items := []models.LocalSpecPolicy{}
			if err := db.Find(&items).Error; err != nil {
				return err
			}

			count := 0
			for _, item := range items {
				fmt.Println("PolicySpec", item.LeafHubName, item.PolicyID, item.Payload)
				count++
			}
			if count > 0 {
				return nil
			}
			return fmt.Errorf("not found expected resource on the table")
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})
})
