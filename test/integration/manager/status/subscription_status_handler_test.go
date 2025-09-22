package status

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	appsv1alpha1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1alpha1"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

// go test ./test/integration/manager/status -v -ginkgo.focus "SubscriptionStatusHandler"
var _ = Describe("SubscriptionStatusHandler", Ordered, func() {
	It("should be able to sync app status event", func() {
		By("Create event")
		leafHubName := "hub1"
		version := eventversion.NewVersion()
		version.Incr()

		data := generic.GenericObjectBundle{}
		obj := &appsv1alpha1.SubscriptionStatus{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testAppSbu",
				Namespace: "default",
				Annotations: map[string]string{
					constants.OriginOwnerReferenceAnnotation: "2aa5547c-c172-47ed-b70b-db468c84d327",
				},
			},
		}
		data = append(data, obj)
		evt := ToCloudEvent(leafHubName, string(enum.SubscriptionStatusType), version, data)

		By("Sync event with transport")
		err := producer.SendEvent(ctx, *evt)
		Expect(err).Should(Succeed())

		By("Check the table")
		Eventually(func() error {
			sql := fmt.Sprintf("SELECT leaf_hub_name,payload FROM %s.%s", database.StatusSchema,
				database.SubscriptionStatusesTableName)

			rows, err := database.GetGorm().Raw(sql).Rows()
			if err != nil {
				return err
			}
			defer func() {
				if err := rows.Close(); err != nil {
					fmt.Printf("failed to close rows: %v\n", err)
				}
			}()
			for rows.Next() {
				var hubName string
				var payload []byte
				app := &appsv1alpha1.SubscriptionStatus{}
				if err := rows.Scan(&hubName, &payload); err != nil {
					return err
				}
				err := json.Unmarshal(payload, app)
				if err != nil {
					return err
				}

				fmt.Println("SubscriptionReport: ", hubName, app.Name)
				if hubName == leafHubName && app.Name == obj.Name {
					return nil
				}
			}
			return fmt.Errorf("not found expected resource on the table")
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})
})
