package placement

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

var _ = Describe("Placement integration test", Ordered, func() {
	It("should be able to sync placementrule", func() {
		By("Create global placementrule")
		testGlobalPlacementRuleOriginUID := "test-globalplacementrule-uid"
		testGlobalPlacementRule := &placementrulev1.PlacementRule{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-globalplacementrule-1",
				Namespace:    "default",
				Annotations: map[string]string{
					constants.OriginOwnerReferenceAnnotation: testGlobalPlacementRuleOriginUID,
				},
			},
			Spec: placementrulev1.PlacementRuleSpec{},
		}
		Expect(kubeClient.Create(ctx, testGlobalPlacementRule)).ToNot(HaveOccurred())

		By("Check the placementrule can be read from cloudevents consumer")
		evt := <-consumer.EventChan()
		fmt.Println(evt)
		Expect(evt).ShouldNot(BeNil())
		Expect(evt.Type()).Should(Equal(string(enum.PlacementRuleSpecType)))
	})

	It("should be able to sync placement", func() {
		By("Create global placement")
		testGlobalPlacementOriginUID := "test-globalplacement-uid"
		testGlobalPlacement := &clusterv1beta1.Placement{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-globalplacement-1",
				Namespace: "default",
				Annotations: map[string]string{
					constants.OriginOwnerReferenceAnnotation: testGlobalPlacementOriginUID,
				},
			},
			Spec: clusterv1beta1.PlacementSpec{},
		}
		Expect(kubeClient.Create(ctx, testGlobalPlacement)).ToNot(HaveOccurred())

		By("Check the placement can be read from cloudevents consumer")
		evt := <-consumer.EventChan()
		fmt.Println(evt)
		Expect(evt).ShouldNot(BeNil())
		Expect(evt.Type()).Should(Equal(string(enum.PlacementSpecType)))
	})

	It("should be able to sync placement decision", func() {
		By("Create placementdecision")
		testGlobalPlacementDecisionOriginUID := "test-globalplacement-decision-uid"
		testPlacementDecision := &clusterv1beta1.PlacementDecision{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-placementdecision-1",
				Namespace: "default",
				Annotations: map[string]string{
					constants.OriginOwnerReferenceAnnotation: testGlobalPlacementDecisionOriginUID,
				},
			},
			Status: clusterv1beta1.PlacementDecisionStatus{},
		}
		Expect(kubeClient.Create(ctx, testPlacementDecision)).ToNot(HaveOccurred())

		By("Check the placementdecision can be read from cloudevents consumer")
		evt := <-consumer.EventChan()
		fmt.Println(evt)
		Expect(evt).ShouldNot(BeNil())
		Expect(evt.Type()).Should(Equal(string(enum.PlacementDecisionType)))
	})
})
