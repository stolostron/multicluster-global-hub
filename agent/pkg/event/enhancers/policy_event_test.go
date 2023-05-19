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
		By("Creating a policy")
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

	It("should pass policy event enhancer", func() {
		// create a kube.enhancedenvet
		in := kube.EnhancedEvent{
			Event: corev1.Event{
				Message: "foovar",
				ObjectMeta: metav1.ObjectMeta{
					Name:      "event1",
					Namespace: "default",
				},
			},
			InvolvedObject: kube.EnhancedObjectReference{
				ObjectReference: corev1.ObjectReference{
					Kind:      string(policyv1.Kind),
					Name:      "managed-cluster-policy",
					Namespace: cluster.Name,
				},
				Labels: map[string]string{
					constants.PolicyEventRootPolicyNameLabelKey: fmt.Sprintf("%s.%s", rootPolicy.Namespace, rootPolicy.Name),
					constants.PolicyEventClusterNameLabelKey:    cluster.Name,
				},
			},
		}
		NewPolicyEventEnhancer(runtimeClient).Enhance(ctx, &in)
		// b, _ := json.MarshalIndent(in, "", "  ")
		// fmt.Println(string(b))
		Expect(in.InvolvedObject.Labels[constants.PolicyEventRootPolicyIdLabelKey]).To(
			Equal(string(rootPolicy.GetUID())))
		Expect(in.InvolvedObject.Labels[constants.PolicyEventClusterIdLabelKey]).To(
			Equal(string(cluster.GetUID())))
		Expect(in.InvolvedObject.Labels[constants.PolicyEventClusterComplianceLabelKey]).To(
			Equal(string(rootPolicy.Status.ComplianceState)))

		Expect(true).To(BeTrue())
	})
})
