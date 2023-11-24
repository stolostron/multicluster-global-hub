package syncers_test

import (
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var _ = Describe("Test resend Bundle", func() {
	It("sync resend bundle", func() {
		resendMsgKeys := []string{constants.HubClusterInfoMsgKey}
		payloadBytes, err := json.Marshal(resendMsgKeys)
		Expect(err).NotTo(HaveOccurred())
		lastUpdateTimestamp := time.Now()
		err = producer.Send(ctx, &transport.Message{
			Destination: transport.Broadcast,
			ID:          constants.ResendMsgKey,
			MsgType:     constants.SpecBundle,
			Version:     lastUpdateTimestamp.Format(timeFormat),
			Payload:     payloadBytes,
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("sync resend bundle with unknown message", func() {
		resendMsgKeys := "unknown"
		payloadBytes, err := json.Marshal(resendMsgKeys)
		Expect(err).NotTo(HaveOccurred())
		lastUpdateTimestamp := time.Now()
		err = producer.Send(ctx, &transport.Message{
			Destination: transport.Broadcast,
			ID:          constants.ResendMsgKey,
			MsgType:     constants.SpecBundle,
			Version:     lastUpdateTimestamp.Format(timeFormat),
			Payload:     payloadBytes,
		})
		Expect(err).NotTo(HaveOccurred())
	})
})
