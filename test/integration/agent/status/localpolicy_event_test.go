package status

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/filter"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// go test ./test/integration/agent/status -v -ginkgo.focus "LocalPolicyEventEmitter"
var _ = Describe("LocalPolicyEventEmitter", Ordered, func() {
	var cachedRootPolicyEvent *corev1.Event
	policyName := "event-local-policy"
	policyNamespace := "default"
	It("should pass the root policy event", func() {
		By("Creating a root policy")
		rootPolicy := &policyv1.Policy{
			ObjectMeta: metav1.ObjectMeta{
				Name:       policyName,
				Namespace:  policyNamespace,
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
		Expect(runtimeClient.Create(ctx, rootPolicy)).NotTo(HaveOccurred())

		evt := &corev1.Event{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "event-local-policy.123r543243242",
				Namespace: "default",
			},
			InvolvedObject: corev1.ObjectReference{
				Kind:      string(policyv1.Kind),
				Name:      policyName,
				Namespace: policyNamespace,
			},
			Reason:  "PolicyPropagation",
			Message: "Policy default/policy1 was propagated to cluster1",
			Source: corev1.EventSource{
				Component: "policy-propagator",
			},
		}
		Expect(runtimeClient.Create(ctx, evt)).NotTo(HaveOccurred())

		Eventually(func() error {
			key := string(enum.LocalRootPolicyEventType)
			receivedEvent, ok := receivedEvents[key]
			if !ok {
				return fmt.Errorf("not get the event: %s", key)
			}
			fmt.Println(">>>>>>>>>>>>>>>>>>> root policy event1", receivedEvent)
			outEvents := []event.RootPolicyEvent{}
			err := json.Unmarshal(receivedEvent.Data(), &outEvents)
			if err != nil {
				return err
			}
			if len(outEvents) == 0 {
				return fmt.Errorf("got an empty event payload: %s", key)
			}

			if outEvents[0].EventName != evt.Name {
				return fmt.Errorf("want %v, but got %v", evt, outEvents[0])
			}
			cachedRootPolicyEvent = evt
			return nil
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})

	It("should skip the older root policy event", func() {
		By("Modify the cache time")
		err := runtimeClient.Get(ctx, client.ObjectKeyFromObject(cachedRootPolicyEvent), cachedRootPolicyEvent)
		Expect(err).Should(Succeed())

		name := strings.Replace(string(enum.LocalRootPolicyEventType), enum.EventTypePrefix, "", -1)
		// the delta is 0 seconds, the next 3 seconds(3 - 0) events will be filtered
		filter.DeltaDuration = 0 // set the buffer into 0
		filter.CacheSyncInterval = 1
		cacheTime := 5 * time.Second
		filter.CacheTime(name, cachedRootPolicyEvent.CreationTimestamp.Time.Add(cacheTime))

		By("Create a expired event")
		expiredEvent := &corev1.Event{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "policy1.expired.123r543243333",
				Namespace: "default",
			},
			InvolvedObject: corev1.ObjectReference{
				Kind:      string(policyv1.Kind),
				Name:      policyName,
				Namespace: policyNamespace,
			},
			Reason:  "PolicyPropagation",
			Message: "Policy default/policy1 was propagated to cluster2",
			Source: corev1.EventSource{
				Component: "policy-propagator",
			},
		}
		Expect(runtimeClient.Create(ctx, expiredEvent)).NotTo(HaveOccurred())
		time.Sleep(cacheTime)

		By("Create a new event")
		newEvent := &corev1.Event{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "policy1.newer.123r543245555",
				Namespace: "default",
			},
			InvolvedObject: corev1.ObjectReference{
				Kind:      string(policyv1.Kind),
				Name:      policyName,
				Namespace: policyNamespace,
			},
			Reason:  "PolicyPropagation",
			Message: "Policy default/policy1 was propagated to cluster3",
			Source: corev1.EventSource{
				Component: "policy-propagator",
			},
		}
		Expect(runtimeClient.Create(ctx, newEvent)).NotTo(HaveOccurred())

		Eventually(func() error {
			key := string(enum.LocalRootPolicyEventType)
			receivedEvent, ok := receivedEvents[key]
			if !ok {
				return fmt.Errorf("not get the event: %s", key)
			}
			outEvents := []event.RootPolicyEvent{}
			err := json.Unmarshal(receivedEvent.Data(), &outEvents)
			if err != nil {
				return err
			}
			if len(outEvents) == 0 {
				return fmt.Errorf("got an empty event payload %s", key)
			}

			gotEvent := false
			for _, outEvent := range outEvents {
				if outEvent.EventName == expiredEvent.Name {
					Fail("should not get the expired event: policy1.expired.123r543243333")
				}
				if outEvent.EventName == newEvent.Name {
					gotEvent = true
				}
			}
			if gotEvent {
				fmt.Println(">>>>>>>>>>>>>>>>>>> root policy event2", receivedEvent)
				return nil
			}

			fmt.Println(">>>>>>> not get the new event: policy1.newer.123r543245555")
			utils.PrettyPrint(outEvents)
			return fmt.Errorf("want event: %+v", newEvent)
		}, 30*time.Second, 1*time.Second).ShouldNot(HaveOccurred())
	})

	It("should pass the replicated policy event", func() {
		Skip("the event is duplicated with event from replicated policy history, so skip...")
		By("Create namespace and cluster for the replicated policy")
		err := runtimeClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "policy-event-cluster",
			},
		}, &client.CreateOptions{})
		Expect(err).Should(Succeed())

		By("Create the cluster")
		cluster := &clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "policy-event-cluster",
			},
		}
		Expect(runtimeClient.Create(ctx, cluster, &client.CreateOptions{})).Should(Succeed())
		cluster.Status = clusterv1.ManagedClusterStatus{
			ClusterClaims: []clusterv1.ManagedClusterClaim{
				{
					Name:  constants.ClusterIdClaimName,
					Value: "3f406177-34b2-4852-88dd-ff2809680336",
				},
			},
		}
		Expect(runtimeClient.Status().Update(ctx, cluster)).Should(Succeed())

		By("Create the replicated policy")
		replicatedPolicyName := fmt.Sprintf("%s.%s", policyNamespace, policyName)
		replicatedPolicy := &policyv1.Policy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      replicatedPolicyName,
				Namespace: "policy-event-cluster",
				Labels: map[string]string{
					constants.PolicyEventRootPolicyNameLabelKey: replicatedPolicyName,
					constants.PolicyEventClusterNameLabelKey:    "policy-event-cluster",
				},
			},
			Spec: policyv1.PolicySpec{
				Disabled:        false,
				PolicyTemplates: []*policyv1.PolicyTemplate{},
			},
		}
		Expect(runtimeClient.Create(ctx, replicatedPolicy)).ToNot(HaveOccurred())

		By("Create the replicated policy event")
		evt := &corev1.Event{
			ObjectMeta: metav1.ObjectMeta{
				Name:      replicatedPolicyName + ".17af98f19c06811e",
				Namespace: "policy-event-cluster",
			},
			InvolvedObject: corev1.ObjectReference{
				Kind:      string(policyv1.Kind),
				Namespace: "policy-event-cluster",
				Name:      replicatedPolicyName,
			},
			Reason:  "PolicyStatusSync",
			Message: "Policy default.policy1 status was updated in cluster",
			Source: corev1.EventSource{
				Component: "policy-status-sync",
			},
		}
		Expect(runtimeClient.Create(ctx, evt)).NotTo(HaveOccurred())

		Eventually(func() error {
			key := string(enum.LocalReplicatedPolicyEventType)
			receivedEvent, ok := receivedEvents[key]
			if !ok {
				return fmt.Errorf("not get the event: %s", key)
			}
			fmt.Println(">>>>>>>>>>>>>>>>>>> replicated policy event", receivedEvent)
			outEvents := []event.ReplicatedPolicyEvent{}
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
			return nil
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})
})
