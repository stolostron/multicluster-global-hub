package syncers_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/controller/syncers"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var _ = Describe("Test resync Bundle", func() {
	It("sync resync bundle with no cache", func() {
		resyncMsgKeys := []string{string(enum.HubClusterInfoType)}
		payloadBytes, err := json.Marshal(resyncMsgKeys)
		Expect(err).NotTo(HaveOccurred())

		evt := utils.ToCloudEvent(constants.ResyncMsgKey, transport.Broadcast, payloadBytes)
		err = producer.SendEvent(ctx, evt)
		Expect(err).NotTo(HaveOccurred())
	})

	syncers.SupportResyc(string(enum.HubClusterInfoType), metadata.NewBundleVersion())
	It("sync resync bundle with cache", func() {
		resyncMsgKeys := []string{string(enum.HubClusterInfoType)}
		payloadBytes, err := json.Marshal(resyncMsgKeys)
		Expect(err).NotTo(HaveOccurred())

		evt := utils.ToCloudEvent(constants.ResyncMsgKey, transport.Broadcast, payloadBytes)
		err = producer.SendEvent(ctx, evt)
		Expect(err).NotTo(HaveOccurred())
	})

	It("sync resync bundle with unknown message", func() {
		resyncMsgKeys := "unknownMsg"
		payloadBytes, err := json.Marshal(resyncMsgKeys)
		Expect(err).NotTo(HaveOccurred())

		evt := utils.ToCloudEvent(constants.ResyncMsgKey, transport.Broadcast, payloadBytes)
		err = producer.SendEvent(ctx, evt)
		Expect(err).NotTo(HaveOccurred())
	})
})
