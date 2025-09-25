package jobs_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	clusterv1beta2 "open-cluster-management.io/api/cluster/v1beta2"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	placementrulesv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/jobs"
)

var _ = Describe("Prune Resource Finalizer", Ordered, func() {
	var placementRule *placementrulesv1.PlacementRule
	var placement *clusterv1beta1.Placement
	var placementbinding *policyv1.PlacementBinding
	var policy *policyv1.Policy
	var managedClusterSetBinding *clusterv1beta2.ManagedClusterSetBinding
	var managedClusterSet *clusterv1beta2.ManagedClusterSet
	var clusterV1Beta2API bool

	BeforeAll(func() {

		By("Create placementrule instance with global hub finalizer")
		placementRule = &placementrulesv1.PlacementRule{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "placementrule1",
				Namespace:  "default",
				Finalizers: []string{constants.GlobalHubCleanupFinalizer},
			},
			Spec: placementrulesv1.PlacementRuleSpec{
				SchedulerName: "test-schedulerName",
			},
		}
		Expect(runtimeClient.Create(ctx, placementRule)).NotTo(HaveOccurred())
		Eventually(func() bool {
			return containGlobalFinalizer(placementRule)
		}, 1*time.Second, 100*time.Millisecond).Should(BeTrue())

		By("Create placement instance with global hub finalizer")
		placement = &clusterv1beta1.Placement{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "placement1",
				Namespace:  "default",
				Finalizers: []string{constants.GlobalHubCleanupFinalizer},
			},
			Spec: clusterv1beta1.PlacementSpec{},
		}
		Expect(runtimeClient.Create(ctx, placement)).NotTo(HaveOccurred())
		Eventually(func() bool {
			return containGlobalFinalizer(placement)
		}, 1*time.Second, 100*time.Millisecond).Should(BeTrue())

		By("Create policy instance with global hub finalizer")
		policy = &policyv1.Policy{
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
		Expect(runtimeClient.Create(ctx, policy)).NotTo(HaveOccurred())
		Eventually(func() bool {
			return containGlobalFinalizer(policy)
		}, 1*time.Second, 100*time.Millisecond).Should(BeTrue())

		clusterV1Beta2API = true
		clusterV1Beta2Service := &apiregistrationv1.APIService{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("%s.%s", clusterv1beta2.GroupVersion.Version, clusterv1beta2.GroupName),
			},
		}
		if err := runtimeClient.Get(ctx, client.ObjectKeyFromObject(clusterV1Beta2Service), clusterV1Beta2Service); err != nil {
			clusterV1Beta2API = false
		}

		By("Create managedclustersetbinding instance with global hub finalizer")
		if clusterV1Beta2API {
			managedClusterSetBinding = &clusterv1beta2.ManagedClusterSetBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-managedclustersetbinding-1",
					Namespace:  "default",
					Finalizers: []string{constants.GlobalHubCleanupFinalizer},
				},
				Spec: clusterv1beta2.ManagedClusterSetBindingSpec{
					ClusterSet: "test-clusterset",
				},
			}
			Expect(runtimeClient.Create(ctx, managedClusterSetBinding)).NotTo(HaveOccurred())
			Eventually(func() bool {
				return containGlobalFinalizer(managedClusterSetBinding)
			}, 1*time.Second, 100*time.Millisecond).Should(BeTrue())
		}

		By("Create managedclusterset instance with global hub finalizer")
		if clusterV1Beta2API {
			managedClusterSet = &clusterv1beta2.ManagedClusterSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-managedclusterset-1",
					Namespace:  "default",
					Finalizers: []string{constants.GlobalHubCleanupFinalizer},
				},
			}
			Expect(runtimeClient.Create(ctx, managedClusterSet)).NotTo(HaveOccurred())
			Eventually(func() bool {
				return containGlobalFinalizer(managedClusterSet)
			}, 1*time.Second, 100*time.Millisecond).Should(BeTrue())
		}

		By("Create placementbinding instance with global hub finalizer")
		placementbinding = &policyv1.PlacementBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-placementbinding-1",
				Namespace:  "default",
				Finalizers: []string{constants.GlobalHubCleanupFinalizer},
			},
			PlacementRef: policyv1.PlacementSubject{
				APIGroup: "cluster.open-cluster-management.io",
				Kind:     "Placement",
				Name:     "placement1",
			},
			Subjects: []policyv1.Subject{
				{
					APIGroup: "policy.open-cluster-management.io",
					Kind:     "Policy",
					Name:     "policy1",
				},
			},
		}
		Expect(runtimeClient.Create(ctx, placementbinding)).NotTo(HaveOccurred())
		Eventually(func() bool {
			return containGlobalFinalizer(placementbinding)
		}, 1*time.Second, 100*time.Millisecond).Should(BeTrue())
	})

	It("run the agent prune job to prune finalizer", func() {
		By("Trigger the prune finalizer job")
		job := jobs.NewPruneFinalizer(ctx, runtimeClient)
		Expect(job).NotTo(BeNil())
		Expect(job.Run()).Should(BeNil())

		By("Check the finalizer is exists in placementRule")
		Eventually(func() bool {
			return containGlobalFinalizer(placementRule)
		}, 1*time.Second, 100*time.Millisecond).Should(BeFalse())

		By("Check the finalizer is exists in placement")
		Eventually(func() bool {
			return containGlobalFinalizer(placement)
		}, 1*time.Second, 100*time.Millisecond).Should(BeFalse())

		By("Check the finalizer is exists in policy")
		Eventually(func() bool {
			return containGlobalFinalizer(policy)
		}, 1*time.Second, 100*time.Millisecond).Should(BeFalse())

		By("Check the finalizer is exists in managedclusterset")
		if clusterV1Beta2API {
			Eventually(func() bool {
				return containGlobalFinalizer(managedClusterSet)
			}, 1*time.Second, 100*time.Millisecond).Should(BeFalse())
		}

		By("Check the finalizer is exists in managedclustersetbinding")
		if clusterV1Beta2API {
			Eventually(func() bool {
				return containGlobalFinalizer(managedClusterSetBinding)
			}, 1*time.Second, 100*time.Millisecond).Should(BeFalse())
		}

		By("Check the finalizer is exists in placementbinding")
		Eventually(func() bool {
			return containGlobalFinalizer(placementbinding)
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
	return controllerutil.ContainsFinalizer(object, constants.GlobalHubCleanupFinalizer)
}
