package status

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

// go test ./test/integration/agent/status -v -ginkgo.focus "ManagedClusterEventEmitter"
var _ = Describe("ManagedClusterEventEmitter", Ordered, func() {
	It("should pass the managed cluster event in batch mode", func() {
		By("Create namespace and cluster for managed cluster event")
		err := runtimeClient.Create(ctx, &corev1.Namespace{
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
		Expect(runtimeClient.Create(ctx, cluster, &client.CreateOptions{})).Should(Succeed())

		By("Claim the clusterId")
		cluster.Status = clusterv1.ManagedClusterStatus{
			ClusterClaims: []clusterv1.ManagedClusterClaim{
				{
					Name:  constants.ClusterIdClaimName,
					Value: "4f406177-34b2-4852-88dd-ff2809680444",
				},
			},
		}
		Expect(runtimeClient.Status().Update(ctx, cluster)).Should(Succeed())

		By("Wait for the managed cluster")
		Eventually(func() error {
			// wait for managed cluster
			return runtimeClient.Get(ctx, types.NamespacedName{Name: "cluster2"}, &clusterv1.ManagedCluster{})
		}, 3*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

		By("Create multiple cluster events to test batch mode")
		events := []*corev1.Event{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cluster2.event.17cd34e8c8b27fdd",
					Namespace: "cluster2",
				},
				InvolvedObject: corev1.ObjectReference{
					Kind:      constants.ManagedClusterKind,
					Namespace: "cluster2",
					Name:      cluster.Name,
				},
				Reason:              "AvailableUnknown",
				Message:             "The managed cluster (cluster2) cannot connect to the hub cluster.",
				ReportingController: "registration-controller",
				ReportingInstance:   "registration-controller-cluster-manager-registration-controller-6794cf54d9-j7lgm",
				Type:                "Warning",
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cluster2.event.28cd34e8c8b27fee",
					Namespace: "cluster2",
				},
				InvolvedObject: corev1.ObjectReference{
					Kind:      constants.ManagedClusterKind,
					Namespace: "cluster2",
					Name:      cluster.Name,
				},
				Reason:              "Available",
				Message:             "The managed cluster (cluster2) is now available.",
				ReportingController: "registration-controller",
				ReportingInstance:   "registration-controller-cluster-manager-registration-controller-6794cf54d9-j7lgm",
				Type:                "Normal",
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cluster2.event.39cd34e8c8b27fff",
					Namespace: "cluster2",
				},
				InvolvedObject: corev1.ObjectReference{
					Kind:      constants.ManagedClusterKind,
					Namespace: "cluster2",
					Name:      cluster.Name,
				},
				Reason:              "Updated",
				Message:             "The managed cluster (cluster2) status updated.",
				ReportingController: "registration-controller",
				ReportingInstance:   "registration-controller-cluster-manager-registration-controller-6794cf54d9-j7lgm",
				Type:                "Normal",
			},
		}

		for _, evt := range events {
			Expect(runtimeClient.Create(ctx, evt)).NotTo(HaveOccurred())
		}

		Eventually(func() error {
			// wait for managed cluster
			key := string(enum.ManagedClusterEventType)
			receivedEvent, ok := receivedEvents[key]
			if !ok {
				return fmt.Errorf("not get the event: %s", key)
			}
			// fmt.Println(">>>>>>>>>>>>>>>>>>> managed cluster event", receivedEvent)

			// Verify it's sent in batch mode
			sendMode, ok := receivedEvent.Extensions()[constants.CloudEventExtensionSendMode]
			if !ok {
				return fmt.Errorf("missing send mode extension")
			}
			if sendMode != string(constants.EventSendModeBatch) {
				return fmt.Errorf("expected batch mode, got %s", sendMode)
			}

			outEvents := event.ManagedClusterEventBundle{}
			err = json.Unmarshal(receivedEvent.Data(), &outEvents)
			if err != nil {
				return err
			}
			if len(outEvents) == 0 {
				return fmt.Errorf("got an empty event payload %s", key)
			}

			// In batch mode, we expect all events to be bundled together
			if len(outEvents) < 3 {
				return fmt.Errorf("expected at least 3 events in batch, got %d", len(outEvents))
			}

			// Verify that all created events are present
			eventNames := make(map[string]bool)
			for _, evt := range outEvents {
				eventNames[evt.EventName] = true
			}

			expectedNames := []string{
				"cluster2.event.17cd34e8c8b27fdd",
				"cluster2.event.28cd34e8c8b27fee",
				"cluster2.event.39cd34e8c8b27fff",
			}

			for _, expectedName := range expectedNames {
				if !eventNames[expectedName] {
					return fmt.Errorf("expected event %s not found in batch", expectedName)
				}
			}
			return nil
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})

	It("should pass managed cluster event in single mode", func() {
		By("Set agent config to single mode")
		originalEventMode := agentConfig.EventMode
		agentConfig.EventMode = string(constants.EventSendModeSingle)
		defer func() {
			agentConfig.EventMode = originalEventMode
		}()

		By("Create namespace and cluster for single mode test")
		err := runtimeClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cluster-single",
			},
		}, &client.CreateOptions{})
		Expect(err).Should(Succeed())

		By("Create the cluster")
		cluster := &clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cluster-single",
			},
		}
		Expect(runtimeClient.Create(ctx, cluster, &client.CreateOptions{})).Should(Succeed())

		By("Claim the clusterId")
		cluster.Status = clusterv1.ManagedClusterStatus{
			ClusterClaims: []clusterv1.ManagedClusterClaim{
				{
					Name:  constants.ClusterIdClaimName,
					Value: "6f406177-34b2-4852-88dd-ff2809680666",
				},
			},
		}
		Expect(runtimeClient.Status().Update(ctx, cluster)).Should(Succeed())

		By("Wait for the managed cluster")
		Eventually(func() error {
			return runtimeClient.Get(ctx, types.NamespacedName{Name: "cluster-single"}, &clusterv1.ManagedCluster{})
		}, 3*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

		By("Clear previous received events to avoid interference")
		delete(receivedEvents, string(enum.ManagedClusterEventType))

		By("Create a single cluster event to test single mode sending")
		evt := &corev1.Event{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster-single.event.single-test",
				Namespace: "cluster-single",
			},
			InvolvedObject: corev1.ObjectReference{
				Kind:      constants.ManagedClusterKind,
				Namespace: "cluster-single",
				Name:      cluster.Name,
			},
			Reason:              "SingleModeTest",
			Message:             "Testing single mode event sending",
			ReportingController: "test-controller",
			ReportingInstance:   "test-instance",
			Type:                "Normal",
		}
		Expect(runtimeClient.Create(ctx, evt)).NotTo(HaveOccurred())

		Eventually(func() error {
			key := string(enum.ManagedClusterEventType)
			receivedEvent, ok := receivedEvents[key]
			if !ok {
				return fmt.Errorf("not get the event: %s", key)
			}
			fmt.Println(">>>>>>>>>>>>>>>>>>> managed cluster event (single mode)", receivedEvent)

			// Verify it's sent in single mode
			sendMode, ok := receivedEvent.Extensions()[constants.CloudEventExtensionSendMode]
			if !ok {
				return fmt.Errorf("missing send mode extension")
			}
			if sendMode != string(constants.EventSendModeSingle) {
				return fmt.Errorf("expected single mode, got %s", sendMode)
			}

			// In single mode, the payload should be a single ManagedClusterEvent, not an array
			var singleEvent models.ManagedClusterEvent
			err = json.Unmarshal(receivedEvent.Data(), &singleEvent)
			if err != nil {
				// If unmarshaling as single event fails, try as array to provide better error message
				var eventArray event.ManagedClusterEventBundle
				if arrayErr := json.Unmarshal(receivedEvent.Data(), &eventArray); arrayErr == nil {
					return fmt.Errorf("received event array in single mode, expected single event")
				}
				return fmt.Errorf("failed to unmarshal event data: %v", err)
			}

			// Verify the single event data
			if singleEvent.EventName != evt.Name {
				return fmt.Errorf("event name mismatch: expected %s, got %s", evt.Name, singleEvent.EventName)
			}
			if singleEvent.ClusterName != cluster.Name {
				return fmt.Errorf("cluster name mismatch: expected %s, got %s", cluster.Name, singleEvent.ClusterName)
			}
			if singleEvent.Reason != evt.Reason {
				return fmt.Errorf("reason mismatch: expected %s, got %s", evt.Reason, singleEvent.Reason)
			}

			return nil
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})
})
