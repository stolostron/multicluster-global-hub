package status

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

// go test /test/integration/manager/status -v -ginkgo.focus "LocalPolicyEventHandler"
var _ = Describe("LocalReplicatedPolicyEventHandler", Ordered, func() {
	It("should handle the local replicated policy event", func() {
		By("Create Event")
		leafHubName := "hub1"
		version := eventversion.NewVersion()
		version.Incr()

		data := event.ReplicatedPolicyEventBundle{}
		eventName := "local-policy-namespace.policy-limitrange.17b0db242743213210"
		policyID := "13b2e003-2bdf-4c82-9bdf-f1aa7ccf608d"
		clusterID := "f302ce61-98e7-4d63-8dd2-65951e32fd95"
		clusterName := "cluster1"
		compliance := "NonCompliant"
		data = append(data, &event.ReplicatedPolicyEvent{
			BaseEvent: event.BaseEvent{
				EventName:      eventName,
				EventNamespace: "kind-hub1-cluster1",
				Message: `NonCompliant; violation - limitranges [container-mem-limit-range] not found
				 in namespace default`,
				Reason: "PolicyStatusSync",
				Count:  1,
				Source: v1.EventSource{
					Component: "policy-status-history-sync",
				},
				CreatedAt: time.Now(),
			},
			PolicyID:    policyID,
			ClusterID:   clusterID,
			ClusterName: clusterName,
			Compliance:  compliance,
		})

		evt := ToCloudEvent(leafHubName, string(enum.LocalReplicatedPolicyEventType), version, data)

		By("Sync event with transport")
		err := producer.SendEvent(ctx, *evt)
		Expect(err).Should(Succeed())

		By("Check event is created and expired policy is deleted from database")
		Eventually(func() error {
			db := database.GetGorm()
			var replicatedPolicyEvents []models.LocalReplicatedPolicyEvent

			err = db.Where("leaf_hub_name = ?", leafHubName).Find(&replicatedPolicyEvents).Error
			if err != nil {
				return err
			}

			for _, e := range replicatedPolicyEvents {
				fmt.Println("LocalPolicyEvent:", e.EventName, e.ClusterID, e.Compliance)
				if e.EventName == eventName && e.ClusterID == clusterID && e.ClusterName == clusterName &&
					e.Compliance == string(database.NonCompliant) {
					return nil
				}
			}
			return fmt.Errorf("failed to sync resource")
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})
})
