package syncers_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/cache"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/cluster"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var _ = Describe("Test resync Bundle", func() {
	It("sync resync bundle with no cache", func() {
		resyncMsgKeys := []string{constants.HubClusterInfoMsgKey}
		payloadBytes, err := json.Marshal(resyncMsgKeys)
		Expect(err).NotTo(HaveOccurred())
		err = producer.Send(ctx, &transport.Message{
			Key:         constants.ResyncMsgKey,
			Destination: transport.Broadcast,
			MsgType:     constants.SpecBundle,
			Payload:     payloadBytes,
		})
		Expect(err).NotTo(HaveOccurred())
	})

	clusterInfoBundle := cluster.NewAgentHubClusterInfoBundle("leafhub0")
	cache.RegistToCache(constants.HubClusterInfoMsgKey, clusterInfoBundle)
	It("sync resync bundle with cache", func() {
		resyncMsgKeys := []string{constants.HubClusterInfoMsgKey}
		payloadBytes, err := json.Marshal(resyncMsgKeys)
		Expect(err).NotTo(HaveOccurred())
		err = producer.Send(ctx, &transport.Message{
			Key:         constants.ResyncMsgKey,
			Destination: transport.Broadcast,
			MsgType:     constants.SpecBundle,
			Payload:     payloadBytes,
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("sync resync bundle with unknown message", func() {
		resyncMsgKeys := "unknownMsg"
		payloadBytes, err := json.Marshal(resyncMsgKeys)
		Expect(err).NotTo(HaveOccurred())
		err = producer.Send(ctx, &transport.Message{
			Key:         constants.ResyncMsgKey,
			Destination: transport.Broadcast,
			MsgType:     constants.SpecBundle,
			Payload:     payloadBytes,
		})
		Expect(err).NotTo(HaveOccurred())
	})
})
