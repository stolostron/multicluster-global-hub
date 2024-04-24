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
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

// go test ./agent/pkg/status/controller/event -v -ginkgo.focus "LocalRootPolicyEventEmitter"
var _ = Describe("LocalRootPolicyEventEmitter", Ordered, func() {
	eventTime := time.Now()
	It("should pass the root policy event", func() {
		By("Creating a root policy")
		rootPolicy := &policyv1.Policy{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "policy1",
				Namespace:  "default",
				Finalizers: []string{constants.GlobalHubCleanupFinalizer},
			},
			Spec: policyv1.PolicySpec{
				Disabled:        true,
				PolicyTemplates: []*policyv1.PolicyTemplate{},
			},
			Status: policyv1.PolicyStatus{
				ComplianceState: policyv1.Compliant,
			},
		}
		Expect(kubeClient.Create(ctx, rootPolicy)).NotTo(HaveOccurred())

		evt := &corev1.Event{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "policy1.123r543243242",
				Namespace: "default",
			},
			InvolvedObject: v1.ObjectReference{
				Kind:      string(policyv1.Kind),
				Namespace: "default",
				Name:      "policy1",
			},
			Reason:  "PolicyPropagation",
			Message: "Policy default/policy1 was propagated to cluster1",
			Source: corev1.EventSource{
				Component: "policy-propagator",
			},
			LastTimestamp: metav1.Time{Time: eventTime},
		}
		Expect(kubeClient.Create(ctx, evt)).NotTo(HaveOccurred())

		receivedEvent := <-consumer.EventChan()
		fmt.Println(">>>>>>>>>>>>>>>>>>> create event1", receivedEvent)
		Expect(string(enum.LocalRootPolicyEventType)).To(Equal(receivedEvent.Type()))

		rootPolicyEvents := []event.RootPolicyEvent{}
		err := json.Unmarshal(receivedEvent.Data(), &rootPolicyEvents)
		Expect(err).Should(Succeed())
		Expect(rootPolicyEvents[0].EventName).To(Equal(evt.Name))
	})

	It("should skip the older root policy event", func() {
		By("Create a expired event")
		expiredEventName := "policy1.expired.123r543243333"
		evt := &corev1.Event{
			ObjectMeta: metav1.ObjectMeta{
				Name:      expiredEventName,
				Namespace: "default",
			},
			InvolvedObject: v1.ObjectReference{
				Kind:      string(policyv1.Kind),
				Namespace: "default",
				Name:      "policy1",
			},
			Reason:  "PolicyPropagation",
			Message: "Policy default/policy1 was propagated to cluster2",
			Source: corev1.EventSource{
				Component: "policy-propagator",
			},
			LastTimestamp: metav1.Time{Time: eventTime.Add(-5 * time.Second)},
		}
		Expect(kubeClient.Create(ctx, evt)).NotTo(HaveOccurred())

		By("Create a new event")
		newerEventName := "policy1.newer.123r543245555"
		evt = &corev1.Event{
			ObjectMeta: metav1.ObjectMeta{
				Name:      newerEventName,
				Namespace: "default",
			},
			InvolvedObject: v1.ObjectReference{
				Kind:      string(policyv1.Kind),
				Namespace: "default",
				Name:      "policy1",
			},
			Reason:  "PolicyPropagation",
			Message: "Policy default/policy1 was propagated to cluster3",
			Source: corev1.EventSource{
				Component: "policy-propagator",
			},
			LastTimestamp: metav1.Time{Time: eventTime.Add(10 * time.Second)},
		}
		Expect(kubeClient.Create(ctx, evt)).NotTo(HaveOccurred())

		receivedEvent := <-consumer.EventChan()
		fmt.Println(">>>>>>>>>>>>>>>>>>> create event2", receivedEvent)
		Expect(string(enum.LocalRootPolicyEventType)).To(Equal(receivedEvent.Type()))

		rootPolicyEvents := []event.RootPolicyEvent{}
		err := json.Unmarshal(receivedEvent.Data(), &rootPolicyEvents)
		Expect(err).Should(Succeed())
		Expect(len(rootPolicyEvents)).To(Equal(1))
		Expect(rootPolicyEvents[0].EventName).NotTo(Equal(expiredEventName))
		Expect(rootPolicyEvents[0].EventName).To(Equal(newerEventName))
	})
})
