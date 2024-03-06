package managedclusters

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

var _ = Describe("Managed cluster integration test", Ordered, func() {
	It("should be able to sync managed clusters", func() {

		By("Create managed clusters in testing managed hub")
		testMangedCluster := &clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-mc-1",
				Labels: map[string]string{
					"cloud":  "Other",
					"vendor": "Other",
				},
				Annotations: map[string]string{
					"cloud":  "Other",
					"vendor": "Other",
				},
			},
			Spec: clusterv1.ManagedClusterSpec{
				HubAcceptsClient:     true,
				LeaseDurationSeconds: 60,
			},
		}
		Expect(kubeClient.Create(ctx, testMangedCluster)).Should(Succeed())

		By("Check the managed cluster status bundle can be read from cloudevents consumer")
		evt := mockTrans.GetEvent()
		fmt.Println(evt)
		Expect(evt).ShouldNot(BeNil())
		Expect(evt.Type()).Should(Equal(string(enum.ManagedClusterType)))
	})
})
