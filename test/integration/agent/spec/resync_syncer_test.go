package spec

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/syncers"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var _ = Describe("ResyncBundle", func() {
	It("resync the bundle", func() {
		// register the resync type
		syncers.EnableResync(string(enum.HubClusterInfoType), version.NewVersion())
		hubInfoVersion := syncers.GetEventVersion(string(enum.HubClusterInfoType))
		Expect(hubInfoVersion.Value).To(Equal(uint64(0)))

		// resync the cluster info
		payloadBytes, err := json.Marshal([]string{string(enum.HubClusterInfoType)})
		Expect(err).NotTo(HaveOccurred())
		err = genericProducer.SendEvent(ctx, utils.ToCloudEvent(constants.ResyncMsgKey, constants.CloudEventGlobalHubClusterName, transport.Broadcast, payloadBytes))
		Expect(err).NotTo(HaveOccurred())

		// resync the unknow message
		payloadBytes, err = json.Marshal([]string{"unknownMsg"})
		Expect(err).NotTo(HaveOccurred())
		err = genericProducer.SendEvent(ctx, utils.ToCloudEvent(constants.ResyncMsgKey, constants.CloudEventGlobalHubClusterName, transport.Broadcast, payloadBytes))
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() error {
			hubInfoVersion = syncers.GetEventVersion(string(enum.HubClusterInfoType))
			if hubInfoVersion.Value != uint64(1) {
				return fmt.Errorf("expected version: %d, but got: %d", 1, hubInfoVersion.Value)
			}
			unKnownMessageVresion := syncers.GetEventVersion("unknownMsg")
			if unKnownMessageVresion != nil {
				return fmt.Errorf("the message version should be empty: %s", "unknownMsg")
			}
			return nil
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})
})
