package dbsyncer_test

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/cluster"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var _ = Describe("HubClusterHeartbeatSyncer", Ordered, func() {
	const (
		leafHubName = "hub1"
		messageKey  = constants.HubClusterHeartbeatMsgKey
	)

	It("sync the hubClusterHeartbeat bundle", func() {
		By("Create hubClusterHeartbeat bundle")
		statusBundle := cluster.NewAgentHubClusterHeartbeatBundle(leafHubName)
		statusBundle.GetVersion().Incr()

		By("Create transport message")
		payloadBytes, err := json.Marshal(statusBundle)
		Expect(err).ShouldNot(HaveOccurred())

		transportMessageKey := fmt.Sprintf("%s.%s", leafHubName, messageKey)
		transportMessage := &transport.Message{
			Key:     transportMessageKey,
			MsgType: constants.StatusBundle,
			Payload: payloadBytes,
		}

		By("Sync message with transport")
		err = producer.Send(ctx, transportMessage)
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
		}, 30*time.Second, 2*time.Second).ShouldNot(HaveOccurred())
	})
})
