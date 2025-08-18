package jobs_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/stolostron/multicluster-global-hub/pkg/jobs"
)

var _ = Describe("Prune Resource Finalizer", Ordered, func() {
	var policy *policyv1.Policy

	BeforeAll(func() {
		By("Create policy instance with global hub finalizer")
		policy = &policyv1.Policy{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "policy1",
				Namespace:  "default",
				Finalizers: []string{"test.finalizer/cleanup"},
			},
			Spec: policyv1.PolicySpec{
				Disabled:        true,
				PolicyTemplates: []*policyv1.PolicyTemplate{},
			},
		}
		Expect(runtimeClient.Create(ctx, policy)).NotTo(HaveOccurred())
		Eventually(func() bool {
			return containGlobalFinalizer(policy)
		}, 1*time.Second, 100*time.Millisecond).Should(BeTrue())
	})

	It("run the agent prune job to skip prune finalizer", func() {
		By("Trigger the prune finalizer job")
		job := jobs.NewPruneFinalizer(ctx, runtimeClientWithoutScheme)
		Expect(job).NotTo(BeNil())
		Expect(job.Run()).Should(BeNil())

		By("Check the finalizer is exists in policy")
		Eventually(func() bool {
			return containGlobalFinalizer(policy)
		}, 1*time.Second, 100*time.Millisecond).Should(BeTrue())
	})

	It("run the agent prune job to prune finalizer", func() {
		By("Trigger the prune finalizer job")
		job := jobs.NewPruneFinalizer(ctx, runtimeClient)
		Expect(job).NotTo(BeNil())
		Expect(job.Run()).Should(BeNil())

		By("Check the finalizer is exists in policy")
		Eventually(func() bool {
			return containGlobalFinalizer(policy)
		}, 1*time.Second, 100*time.Millisecond).Should(BeFalse())
	})
})

func containGlobalFinalizer(object client.Object) bool {
	err := runtimeClient.Get(ctx, types.NamespacedName{
		Namespace: object.GetNamespace(),
		Name:      object.GetName(),
	}, object)
	if err != nil {
		return false
	}
	return controllerutil.ContainsFinalizer(object, "test.finalizer/cleanup")
}
