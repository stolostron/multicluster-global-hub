package grc

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
)

var _ = Describe("event bundle from cluster policy status", Ordered, func() {
	var rootPolicy *policiesv1.Policy
	var cluster *clusterv1.ManagedCluster
	var bundle bundle.Bundle
	var incarnation uint64

	BeforeAll(func() {
		By("Create a root policy")
		rootPolicy = &policiesv1.Policy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "root-policy",
				Namespace: "default",
			},
			Spec: policiesv1.PolicySpec{
				Disabled:        true,
				PolicyTemplates: []*policiesv1.PolicyTemplate{},
			},
		}
		err := runtimeClient.Create(ctx, rootPolicy)
		Expect(err).ToNot(HaveOccurred())

		By("Create a cluster for replicas policy")
		cluster = &clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cluster1",
			},
		}
		err = runtimeClient.Create(ctx, cluster)
		Expect(err).ToNot(HaveOccurred())

		By("Initialize the bundle")
		incarnation = uint64(1)
		bundle = NewClusterPolicyHistoryEventBundle(ctx, "hub1", incarnation, runtimeClient)
	})

	It("Should update the ClusterPolicyHistoryEventBundle", func() {
		By("Update the bundle without policy history events")
		Expect(bundle.GetBundleVersion().Generation).To(Equal(uint64(0)))
		targetPolicy := &policiesv1.Policy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "default",
				Labels: map[string]string{
					constants.PolicyEventRootPolicyNameLabelKey: fmt.Sprintf("%s.%s", rootPolicy.Namespace, rootPolicy.Name),
					constants.PolicyEventClusterNameLabelKey:    cluster.Name,
				},
			},
			Status: policiesv1.PolicyStatus{},
		}
		bundle.UpdateObject(targetPolicy)
		Expect(bundle.GetBundleVersion().Generation).To(Equal(uint64(0)))

		By("Update the bundle with policy history events")
		targetPolicy.Status.Details = []*policiesv1.DetailsPerTemplate{
			{
				ComplianceState: policiesv1.NonCompliant,
				History: []policiesv1.ComplianceHistory{
					{
						EventName:     "openshift-acm-policies.backplane-mobb-sp.176a8f372323ecac",
						LastTimestamp: metav1.Now(),
						Message: `Compliant; notification - clusterrolebindings [backplane-mobb-c0] found
	as specified, therefore this Object template is compliant`,
					},
				},
			},
		}
		bundle.UpdateObject(targetPolicy)
		Expect(bundle.GetBundleVersion().Generation).To(Equal(uint64(1)))
	})

	It("Should parse the compliance for event message", func() {
		message := `NonCompliant; violation - limitranges not found: [container-mem-limit-range]
		in namespace default missing`
		eventBundle, ok := bundle.(*ClusterPolicyHistoryEventBundle)
		Expect(ok).To(BeTrue())
		compliance := eventBundle.ParseCompliance(message)
		Expect(compliance).To(Equal(string(policiesv1.NonCompliant)))
	})

	It("Should update the PolicyStatusEvents for event bundle", func() {
		tmpBundle := NewClusterPolicyHistoryEventBundle(ctx, "hub1", 1, nil)
		eventBundle, ok := tmpBundle.(*ClusterPolicyHistoryEventBundle)
		Expect(ok).To(BeTrue())

		policyStatusEvents := make([]*models.LocalClusterPolicyEvent, 0)
		expiredPolicyEvents := make(map[string]*models.LocalClusterPolicyEvent)

		By("Add the new event to bundle")
		event := policiesv1.ComplianceHistory{
			EventName:     "openshift-acm-policies.backplane-mobb-sp.176a8f372323ecad",
			LastTimestamp: metav1.NewTime(time.Now()),
			Message: `Compliant; notification - clusterrolebindings [backplane-mobb-c0] found
			as specified, therefore this Object template is compliant`,
		}
		policyStatusEvents = eventBundle.updatePolicyEvents(event, "", expiredPolicyEvents,
			"rootPolicyId", "clusterId", policyStatusEvents)
		Expect(len(policyStatusEvents)).To(Equal(1))
		Expect(policyStatusEvents[0].Count).To(Equal(1))

		By("Update the bundle with different event time")
		expiredPolicyEvents[event.EventName] = &models.LocalClusterPolicyEvent{
			BaseLocalPolicyEvent: models.BaseLocalPolicyEvent{
				EventName: event.EventName,
				Message:   event.Message,
				CreatedAt: event.LastTimestamp.Time,
				Count:     1,
			},
		}
		event.LastTimestamp = metav1.NewTime(time.Now())
		policyStatusEvents = eventBundle.updatePolicyEvents(event, "", expiredPolicyEvents,
			"rootPolicyId", "clusterId", policyStatusEvents)
		Expect(len(policyStatusEvents)).To(Equal(2))
		Expect(policyStatusEvents[0].Count).To(Equal(1))
		Expect(policyStatusEvents[1].Count).To(Equal(2))
	})
})
