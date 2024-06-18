package status

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

// go test ./test/integration/agent/status -v -ginkgo.focus "ManagedCluster"
var _ = Describe("ManagedCluster", Ordered, func() {
	var testMangedCluster *clusterv1.ManagedCluster
	var consumer transport.Consumer
	BeforeAll(func() {
		consumer = chanTransport.Consumer(ManagedClusterTopic)
	})

	It("should be able to sync managed clusters", func() {
		By("Create managed clusters in testing managed hub")
		testMangedCluster = &clusterv1.ManagedCluster{
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

		Expect(runtimeClient.Create(ctx, testMangedCluster)).Should(Succeed())

		By("Check the managed cluster status bundle can be read from cloudevents consumer")
		evt := <-consumer.EventChan()
		fmt.Println("init cluster", evt)
		Expect(evt).ShouldNot(BeNil())
		Expect(evt.Type()).Should(Equal(string(enum.ManagedClusterType)))
	})

	It("should be able to delete managed clusters", func() {
		By("Delete managed clusters in testing managed hub")
		err := runtimeClient.Delete(ctx, testMangedCluster)
		Expect(err).To(Succeed())

		By("Check the managed cluster status bundle can be read from cloudevents consumer")
		Eventually(func() error {
			evt := <-consumer.EventChan()
			fmt.Println("empty cluster: ", evt)
			if evt == nil {
				return errors.New("the event shouldn't be nil")
			}
			if evt.Type() != string(enum.ManagedClusterType) {
				return fmt.Errorf("want the eventType: %s, but got %s", string(enum.ManagedClusterType), evt.Type())
			}
			clusters := []clusterv1.ManagedCluster{}
			err := json.Unmarshal(evt.Data(), &clusters)
			if err != nil {
				return err
			}
			for _, c := range clusters {
				if c.Name == testMangedCluster.Name {
					return fmt.Errorf("the cluster %s should be deleted", c.Name)
				}
			}
			return nil
		}, 30*time.Second, 100*time.Millisecond).Should(Succeed())
	})
})
