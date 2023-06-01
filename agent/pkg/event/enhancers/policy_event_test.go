package enhancers

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/resmoio/kubernetes-event-exporter/pkg/kube"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

var _ = Describe("policy enhancer", Ordered, func() {
	var rootPolicy *policyv1.Policy
	var cluster *clusterv1.ManagedCluster
	BeforeAll(func() {
		By("Creating a root policy")
		rootPolicy = &policyv1.Policy{
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
		Expect(runtimeClient.Create(ctx, rootPolicy)).NotTo(HaveOccurred())

		By("Creating a cluster")
		cluster = &clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cluster1",
			},
		}
		Expect(runtimeClient.Create(ctx, cluster)).Should(Succeed())
	})

	It("should pass cluster policy event enhancer", func() {
		// create a kube.enhancedenvet
		in := kube.EnhancedEvent{
			Event: corev1.Event{
				Message: "Policy policy1 status was updated to Compliant in cluster namespace cluster1",
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default.policy1.17647d9f03cafef6",
					Namespace: cluster.Name,
				},
			},
			InvolvedObject: kube.EnhancedObjectReference{
				ObjectReference: corev1.ObjectReference{
					Kind:      string(policyv1.Kind),
					Name:      "default.policy1",
					Namespace: cluster.Name,
				},
				Labels: map[string]string{
					constants.PolicyEventRootPolicyNameLabelKey: fmt.Sprintf("%s.%s", rootPolicy.Namespace, rootPolicy.Name),
					constants.PolicyEventClusterNameLabelKey:    cluster.Name,
				},
			},
		}
		NewPolicyEventEnhancer(runtimeClient).Enhance(ctx, &in)
		Eventually(func() error {
			if in.InvolvedObject.Labels[constants.PolicyEventComplianceLabelKey] != "Compliant" {
				return fmt.Errorf("compliance from message should be Compliant")
			}
			if in.InvolvedObject.Labels[constants.PolicyEventRootPolicyIdLabelKey] != string(rootPolicy.GetUID()) {
				return fmt.Errorf("the label value of root policy id should be %s", rootPolicy.GetUID())
			}
			if in.InvolvedObject.Labels[constants.PolicyEventClusterIdLabelKey] != string(cluster.GetUID()) {
				return fmt.Errorf("the label value of cluster id should be %s", cluster.GetUID())
			}
			return nil
		}).Should(Succeed())
	})

	It("should pass root policy event enhancer", func() {
		// create a kube.enhancedenvet
		in := kube.EnhancedEvent{
			Event: corev1.Event{
				Message: "Policy policy1 status was updated in cluster namespace cluster1",
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy1.123r543243242",
					Namespace: "default",
				},
			},
			InvolvedObject: kube.EnhancedObjectReference{
				ObjectReference: corev1.ObjectReference{
					Kind:      string(policyv1.Kind),
					Name:      "policy1",
					Namespace: "default",
				},
			},
		}
		NewPolicyEventEnhancer(runtimeClient).Enhance(ctx, &in)
		Expect(in.InvolvedObject.Labels[constants.PolicyEventComplianceLabelKey]).To(Equal("Unknown"))
	})

	It("should parse the policy status with regular expression", func() {
		By("Parsing the expected message")
		expectedMessage := "Policy policy1 status was updated to Compliant in cluster namespace cluster1"
		matches := PolicyMessageStatusRe.FindStringSubmatch(expectedMessage)
		Expect(len(matches)).To(Equal(4))
		Expect(matches[1]).To(Equal("policy1"))
		Expect(matches[2]).To(Equal("Compliant"))
		Expect(matches[3]).To(Equal("cluster1"))

		status := parsePolicyStatus(expectedMessage)
		Expect(status).To(Equal("Compliant"))

		By("Parsing the unexpected message")
		unExpectedMessage := "Policy policy2 status was updated in cluster namespace cluster2"
		matches = PolicyMessageStatusRe.FindStringSubmatch(unExpectedMessage)
		Expect(matches).To(BeNil())

		status = parsePolicyStatus(unExpectedMessage)
		Expect(status).To(Equal(""))
	})
})
