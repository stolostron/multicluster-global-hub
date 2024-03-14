package dbsyncer_test

import (
	"fmt"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

// go test ./manager/pkg/statussyncer/syncers -ginkgo.focus "HubClusterHeartbeatHandler"
var _ = Describe("HubClusterHeartbeatHandler", Ordered, func() {
	It("sync the hubClusterHeartbeat bundle", func() {
		By("Create hubClusterHeartbeat event")
		version := eventversion.NewVersion()
		version.Incr()
		leafHubName := "hub1"
		evt := ToCloudEvent(leafHubName, string(enum.HubClusterHeartbeatType), version, generic.GenericObjectData{})

		By("Sync event with transport")
		err := producer.SendEvent(ctx, *evt)
		Expect(err).Should(Succeed())

		By("Check the leaf hubs table")
		Eventually(func() error {
			db := database.GetGorm()
			heartbeats := []models.LeafHubHeartbeat{}
			ret := db.Find(&heartbeats)
			if ret.Error != nil {
				return ret.Error
			}

			count := 0
			for _, heartbeat := range heartbeats {
				fmt.Println(heartbeat.Name, heartbeat.LastUpdateAt, heartbeat.Status)
				count++
			}
			if count > 0 {
				return nil
			}
			return fmt.Errorf("not found heartbeat record on the table")
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})
})

func ToCloudEvent(source, eventType string, version *eventversion.Version, data interface{}) *cloudevents.Event {
	e := cloudevents.NewEvent()
	e.SetSource(source)
	e.SetType(eventType)
	e.SetExtension(eventversion.ExtVersion, version.String())
	_ = e.SetData(cloudevents.ApplicationJSON, data)
	return &e
}
