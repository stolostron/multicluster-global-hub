package status

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

// go test ./test/integration/agent/status -v -ginkgo.focus "ClusterGroupUpgradeEventEmitter"
var _ = Describe("ClusterGroupUpgradeEventEmitter", Ordered, func() {
	It("should pass the managed cluster event", func() {
		By("Create namespace for cgu events")
		err := runtimeClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cgu-ns1",
			},
		}, &client.CreateOptions{})
		Expect(err).Should(Succeed())

		By("Create the cgu event")
		evt := &corev1.Event{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cgu-ns1.event.17cd34e8c8b27fdd",
				Namespace: "cgu-ns1",
				Annotations: map[string]string{
					"cgu.openshift.io/event-type":           "global",
					"cgu.openshift.io/total-clusters-count": "2",
				},
			},
			InvolvedObject: corev1.ObjectReference{
				Kind: constants.ClusterGroupUpgradeKind,
				// TODO: the cluster namespace should be empty! but if not set the namespace,
				// it will throw the error: involvedObject.namespace: Invalid value: "": does not match event.namespace
				Namespace: "cgu-ns1",
				Name:      "test-cgu1",
			},
			Reason:              "CguSuccess",
			Message:             "ClusterGroupUpgrade test-cgu1 succeeded remediating policies",
			ReportingController: "cgu-controller",
			ReportingInstance:   "cgu-controller-6794cf54d9-j7lgm",
			Type:                "Normal",
		}
		Expect(runtimeClient.Create(ctx, evt)).NotTo(HaveOccurred())

		Eventually(func() error {
			key := string(enum.ClusterGroupUpgradesEventType)
			receivedEvent, ok := receivedEvents[key]
			if !ok {
				return fmt.Errorf("not get the event: %s", key)
			}
			fmt.Println(">>>>>>>>>>>>>>>>>>> cgu event", receivedEvent)
			outEvents := event.ClusterGroupUpgradeEventBundle{}
			err = json.Unmarshal(receivedEvent.Data(), &outEvents)
			if err != nil {
				return err
			}
			if len(outEvents) == 0 {
				return fmt.Errorf("got an empty event payload %s", key)
			}

			if outEvents[0].EventName != evt.Name {
				return fmt.Errorf("want %v, but got %v", evt, outEvents[0])
			}

			Expect(outEvents[0].EventType).To(Equal("Normal"))
			Expect(outEvents[0].Reason).To(Equal("CguSuccess"))
			Expect(outEvents[0].Message).To(Equal("ClusterGroupUpgrade test-cgu1 succeeded remediating policies"))
			Expect(outEvents[0].ReportingController).To(Equal("cgu-controller"))
			Expect(outEvents[0].ReportingInstance).To(Equal("cgu-controller-6794cf54d9-j7lgm"))

			annsJSONB, err := json.Marshal(evt.Annotations)
			Expect(err).ToNot(HaveOccurred(), "failed marshalling evt annotations")

			Expect([]byte(outEvents[0].EventAnns)).To(Equal(annsJSONB))

			return nil
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})
})
