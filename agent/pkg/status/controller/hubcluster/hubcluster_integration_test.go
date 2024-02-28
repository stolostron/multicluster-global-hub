package hubcluster

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

var _ = Describe("Integration Test", Ordered, func() {
	It("should receive the heartbeat", func() {

		By("Check the local hearbeat event can be read from cloudevents consumer")
		Eventually(func() error {
			evt := heartbeatTrans.GetEvent()
			fmt.Println(evt)
			if evt.Type() != string(enum.HubClusterHeartbeatType) {
				return fmt.Errorf("want %v, got %v", string(enum.HubClusterHeartbeatType), evt.Type())
			}
			return nil
		}, 10*time.Second, 100*time.Millisecond).Should(Succeed())
	})

})
