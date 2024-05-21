package event

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

// go test ./agent/pkg/status/controller/event -ginkgo.focus "ManagedClusterEventEmitter" -v
var _ = Describe("ManagedClusterEventEmitter", Ordered, func() {
	It("should pass the managed cluster event", func() {
		By("Create namespace and cluster for managed cluster event")
		err := kubeClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cluster2",
			},
		}, &client.CreateOptions{})
		Expect(err).Should(Succeed())

		By("Create the cluster")
		cluster := &clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cluster2",
			},
		}
		Expect(kubeClient.Create(ctx, cluster, &client.CreateOptions{})).Should(Succeed())

		By("Create a event before the clusterId is ready")
		evt1 := &corev1.Event{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster2.event.cache.17cd34e8c8b27fcc",
				Namespace: "cluster2",
			},
			InvolvedObject: v1.ObjectReference{
				Kind: constants.ManagedClusterKind,
				// TODO: the cluster namespace should be empty! but if not set the namespace,
				// it will throw the error: involvedObject.namespace: Invalid value: "": does not match event.namespace
				Namespace: "cluster2",
				Name:      cluster.Name,
			},
			Reason:              "WaitForImporting",
			Message:             "The cluster2 is waiting for importing",
			ReportingController: "managedcluster-import-controller",
			ReportingInstance:   "managedcluster-import-controller-managedcluster-import-controller-v2-7c76cf646f-rb5ws",
			Type:                "Normal",
		}
		Expect(kubeClient.Create(ctx, evt1)).NotTo(HaveOccurred())
		time.Sleep(2 * time.Second)

		By("Claim the clusterId")
		cluster.Status = clusterv1.ManagedClusterStatus{
			ClusterClaims: []clusterv1.ManagedClusterClaim{
				{
					Name:  "id.k8s.io",
					Value: "4f406177-34b2-4852-88dd-ff2809680444",
				},
			},
		}
		Expect(kubeClient.Status().Update(ctx, cluster)).Should(Succeed())

		By("Create the cluster event after the clusterId is ready")
		evt2 := &corev1.Event{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster2.event.17cd34e8c8b27fdd",
				Namespace: "cluster2",
			},
			InvolvedObject: v1.ObjectReference{
				Kind: constants.ManagedClusterKind,
				// TODO: the cluster namespace should be empty! but if not set the namespace,
				// it will throw the error: involvedObject.namespace: Invalid value: "": does not match event.namespace
				Namespace: "cluster2",
				Name:      cluster.Name,
			},
			Reason:              "AvailableUnknown",
			Message:             "The managed cluster (cluster2) cannot connect to the hub cluster.",
			ReportingController: "registration-controller",
			ReportingInstance:   "registration-controller-cluster-manager-registration-controller-6794cf54d9-j7lgm",
			Type:                "Warning",
		}
		Expect(kubeClient.Create(ctx, evt2)).NotTo(HaveOccurred())

		Eventually(func() error {
			key := string(enum.ManagedClusterEventType)
			receivedEvent, ok := receivedEvents[key]
			if !ok {
				return fmt.Errorf("not get the event: %s", key)
			}
			fmt.Println(">>>>>>>>>>>>>>>>>>> managed cluster event", receivedEvent)
			outEvents := event.ManagedClusterEventBundle{}
			err = json.Unmarshal(receivedEvent.Data(), &outEvents)
			if err != nil {
				return err
			}
			if len(outEvents) == 0 {
				return fmt.Errorf("got an empty event payload %s", key)
			}

			if len(outEvents) != 2 {
				return fmt.Errorf("should send the 2 events in 1 bundle")
			}

			if outEvents[0].EventName != evt1.Name {
				return fmt.Errorf("want %v, but got %v", evt1, outEvents[0])
			}
			if outEvents[1].EventName != evt2.Name {
				return fmt.Errorf("want %v, but got %v", evt2, outEvents[1])
			}
			return nil
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})
})
