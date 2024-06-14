package status

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	appsv1alpha1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1alpha1"

	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

// go test ./test/integration/agent/status -v -ginkgo.focus "Application"
var _ = Describe("Application", Ordered, func() {
	var consumer transport.Consumer

	BeforeAll(func() {
		consumer = chanTransport.Consumer(ApplicationTopic)
	})

	It("should be able to sync subscriptionreports", func() {
		By("Create subscriptionreport in testing managed hub")
		testSubscriptionReport := &appsv1alpha1.SubscriptionReport{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-subscriptionreport-1",
				Namespace: "default",
			},
			ReportType: "Application",
			Summary: appsv1alpha1.SubscriptionReportSummary{
				Deployed:          "1",
				InProgress:        "0",
				Failed:            "0",
				PropagationFailed: "0",
				Clusters:          "1",
			},
			Results: []*appsv1alpha1.SubscriptionReportResult{
				{
					Source: "hub1-mc1",
					Result: "deployed",
				},
			},
			Resources: []*corev1.ObjectReference{
				{
					Kind:       "Deployment",
					Namespace:  "default",
					Name:       "nginx-sample",
					APIVersion: "apps/v1",
				},
			},
		}
		Expect(runtimeClient.Create(ctx, testSubscriptionReport)).ToNot(HaveOccurred())

		By("Check the app can be read from cloudevents consumer")
		evt := <-consumer.EventChan()
		fmt.Println(evt)
		Expect(evt).ShouldNot(BeNil())
		Expect(evt.Type()).Should(Equal(string(enum.SubscriptionReportType)))
	})
})
