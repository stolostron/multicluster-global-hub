package controller

import (
	"context"
	"fmt"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm/clause"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/processes/hubmanagement"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var _ = Describe("hub management", func() {
	It("update pvc which do not need backup", func() {
		now := time.Now()
		hubs := []models.LeafHubHeartbeat{
			{
				Name:         "heartbeat-hub01",
				LastUpdateAt: now,
			},
			{
				Name:         "heartbeat-hub02",
				LastUpdateAt: now.Add(-120 * time.Second),
				Status:       hubmanagement.HubActive,
			},
			{
				Name:         "heartbeat-hub03",
				LastUpdateAt: now.Add(-20 * time.Second),
				Status:       hubmanagement.HubActive,
			},
			{
				Name:         "heartbeat-hub04",
				LastUpdateAt: now.Add(-180 * time.Second),
				Status:       hubmanagement.HubInactive,
			},
		}
		db := database.GetGorm()
		err := db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&hubs).Error
		Expect(err).To(Succeed())

		var heartbeatHubs []models.LeafHubHeartbeat
		err = db.Find(&heartbeatHubs).Error
		Expect(err).To(Succeed())

		Expect(heartbeatHubs).NotTo(BeEmpty())
		for _, heartbeatHub := range heartbeatHubs {
			fmt.Println(heartbeatHub.Name, heartbeatHub.LastUpdateAt, heartbeatHub.Status)
			if heartbeatHub.Name == "heartbeat-hub04" {
				Expect(hubmanagement.HubInactive).To(Equal(heartbeatHub.Status))
				continue
			}
			Expect(hubmanagement.HubActive).To(Equal(heartbeatHub.Status))
		}

		// only update the heartbeat Time
		hub4 := models.LeafHubHeartbeat{
			Name:         "heartbeat-hub04",
			LastUpdateAt: now.Add(-60 * time.Second),
		}
		fmt.Println(">> heartbeat: heartbeat-hub04 ")
		err = db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&hub4).Error
		Expect(err).To(Succeed())

		var updatedHub4 models.LeafHubHeartbeat
		err = db.Where("leaf_hub_name = ?", "heartbeat-hub04").Find(&updatedHub4).Error
		Expect(err).To(Succeed())
		fmt.Println(hub4.Name, hub4.LastUpdateAt, hub4.Status)
		Expect(hubmanagement.HubInactive).To(Equal(hub4.Status)) // status isn't is updated

		timeFormat := "2006-01-02 15:04:05" // timestamp is updated
		Expect(now.Add(-60 * time.Second).Format(timeFormat)).To(Equal(updatedHub4.LastUpdateAt.Format(timeFormat)))

		// update
		hubManagement := hubmanagement.NewHubManagement(&tmpProducer{}, 1*time.Second, 90*time.Second)
		Expect(hubManagement.Start(ctx)).To(Succeed())

		time.Sleep(3 * time.Second)

		fmt.Println(">> hub management[90s]: heartbeat-hub02 -> inactive, heartbeat-hub04 -> active")
		var updatedHubs []models.LeafHubHeartbeat
		err = db.Find(&updatedHubs).Error
		Expect(err).To(Succeed())
		Expect(updatedHubs).NotTo(BeEmpty())
		for _, updatedHub := range updatedHubs {
			fmt.Println(updatedHub.Name, updatedHub.LastUpdateAt, updatedHub.Status)
			if updatedHub.Name == "heartbeat-hub02" {
				Expect(hubmanagement.HubInactive).To(Equal(updatedHub.Status))
				continue
			}
			Expect(hubmanagement.HubActive).To(Equal(updatedHub.Status))
		}
	})
})

type tmpProducer struct{}

func (p *tmpProducer) SendEvent(ctx context.Context, evt cloudevents.Event) error {
	return nil
}

func (p *tmpProducer) Reconnect(config *transport.TransportInternalConfig, topic string) error {
	return nil
}
