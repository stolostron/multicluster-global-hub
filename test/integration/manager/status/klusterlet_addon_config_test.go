package status

import (
	"encoding/json"
	"fmt"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	addonv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

// go test ./test/integration/manager/status -v -ginkgo.focus "KlusterletAddonConfig"
var _ = Describe("KlusterletAddonConfig", Ordered, func() {
	It("should be able to sync klusterletaddonconfig decision event", func() {
		By("Create event")

		evt := cloudevents.NewEvent()
		evt.SetType(string(enum.KlusterletAddonConfigType))
		evt.SetSource("hub1")
		evt.SetExtension(constants.CloudEventExtensionKeyClusterName, "hub2")
		evt.SetExtension(eventversion.ExtVersion, "0.1")

		addonConfig := &addonv1.KlusterletAddonConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster1",
				Namespace: "cluster1",
			},
		}
		payloadBytes, err := json.Marshal(addonConfig)
		Expect(err).To(Succeed())
		_ = evt.SetData(cloudevents.ApplicationJSON, payloadBytes)

		version := eventversion.NewVersion()
		version.Incr()

		By("Sync event with transport")
		err = producer.SendEvent(ctx, evt)
		Expect(err).Should(Succeed())

		By("Check the table")
		Eventually(func() error {
			db := database.GetGorm()
			var initialized []models.ManagedClusterMigration
			if err = db.Where("from_hub = ? AND to_hub = ?", "hub1", "hub2").Find(&initialized).Error; err != nil {
				return err
			}

			for _, migration := range initialized {
				if migration.ClusterName == "cluster1" {
					return nil
				}
			}

			return fmt.Errorf("not found expected migration resource on the table")
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})
})
