package status

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"

	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

// go test ./test/integration/agent/status -v -ginkgo.focus "Placement"
var _ = Describe("Placement", Ordered, func() {
	var consumer transport.Consumer
	BeforeAll(func() {
		consumer = chanTransport.Consumer(PlacementTopic)
	})

	It("should be able to sync placement", func() {
		By("Create placement")
		testGlobalPlacement := &clusterv1beta1.Placement{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-globalplacement-1",
				Namespace: "default",
			},
			Spec: clusterv1beta1.PlacementSpec{},
		}
		Expect(runtimeClient.Create(ctx, testGlobalPlacement)).ToNot(HaveOccurred())

		By("Check the placement can be read from cloudevents consumer")
		evt := <-consumer.EventChan()
		fmt.Println(evt)
		Expect(evt).ShouldNot(BeNil())
		Expect(evt.Type()).Should(Equal(string(enum.PlacementSpecType)))
	})

	It("should be able to sync placement decision", func() {
		By("Create placementdecision")
		testPlacementDecision := &clusterv1beta1.PlacementDecision{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-placementdecision-1",
				Namespace: "default",
			},
			Status: clusterv1beta1.PlacementDecisionStatus{},
		}
		Expect(runtimeClient.Create(ctx, testPlacementDecision)).ToNot(HaveOccurred())

		By("Check the placementdecision can be read from cloudevents consumer")
		evt := <-consumer.EventChan()
		fmt.Println(evt)
		Expect(evt).ShouldNot(BeNil())
		Expect(evt.Type()).Should(Equal(string(enum.PlacementDecisionType)))
	})
})
