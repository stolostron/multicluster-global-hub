package jobs_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	chnv1 "open-cluster-management.io/multicloud-operators-channel/pkg/apis/apps/v1"
	placementrulesv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	appsubv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1"
	appv1beta1 "sigs.k8s.io/application/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	commonconstants "github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/jobs"
)

var _ = Describe("Prune Resource Finalizer", func() {
	var app *appv1beta1.Application
	var appsub *appsubv1.Subscription
	var chn *chnv1.Channel
	var placementRule *placementrulesv1.PlacementRule
	var placement *clusterv1beta1.Placement
	var placementbinding *policyv1.PlacementBinding
	var policy *policyv1.Policy
	var managedClusterSetBinding *clusterv1beta1.ManagedClusterSetBinding
	var managedClusterSet *clusterv1beta1.ManagedClusterSet

	BeforeEach(func() {
		By("Create application instance with global hub finalizer")
		app = &appv1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "app1",
				Namespace:  "default",
				Finalizers: []string{commonconstants.GlobalHubCleanupFinalizer},
			},
		}
		Expect(runtimeClient.Create(ctx, app)).NotTo(HaveOccurred())
		Eventually(func() bool {
			return containGlobalFinalizer(app)
		}, 1*time.Second, 100*time.Millisecond).Should(BeTrue())

		By("Create subscription instance with global hub finalizer")
		appsub = &appsubv1.Subscription{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "appsub1",
				Namespace:  "default",
				Finalizers: []string{commonconstants.GlobalHubCleanupFinalizer},
			},
		}
		Expect(runtimeClient.Create(ctx, appsub)).NotTo(HaveOccurred())
		Eventually(func() bool {
			return containGlobalFinalizer(appsub)
		}, 1*time.Second, 100*time.Millisecond).Should(BeTrue())

		By("Create channel instance with global hub finalizer")
		chn = &chnv1.Channel{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "channel1",
				Namespace:  "default",
				Finalizers: []string{commonconstants.GlobalHubCleanupFinalizer},
			},
			Spec: chnv1.ChannelSpec{
				Type:               chnv1.ChannelTypeGit,
				Pathname:           "default",
				InsecureSkipVerify: true,
			},
		}
		Expect(runtimeClient.Create(ctx, chn)).NotTo(HaveOccurred())
		Eventually(func() bool {
			return containGlobalFinalizer(chn)
		}, 1*time.Second, 100*time.Millisecond).Should(BeTrue())

		By("Create placementrule instance with global hub finalizer")
		placementRule = &placementrulesv1.PlacementRule{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "placementrule1",
				Namespace:  "default",
				Finalizers: []string{commonconstants.GlobalHubCleanupFinalizer},
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
				Finalizers: []string{commonconstants.GlobalHubCleanupFinalizer},
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
				Finalizers: []string{commonconstants.GlobalHubCleanupFinalizer},
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

		By("Create managedclustersetbinding instance with global hub finalizer")
		managedClusterSetBinding = &clusterv1beta1.ManagedClusterSetBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-managedclustersetbinding-1",
				Namespace:  "default",
				Finalizers: []string{commonconstants.GlobalHubCleanupFinalizer},
			},
			Spec: clusterv1beta1.ManagedClusterSetBindingSpec{
				ClusterSet: "test-clusterset",
			},
		}
		Expect(runtimeClient.Create(ctx, managedClusterSetBinding)).NotTo(HaveOccurred())
		Eventually(func() bool {
			return containGlobalFinalizer(managedClusterSetBinding)
		}, 1*time.Second, 100*time.Millisecond).Should(BeTrue())

		By("Create managedclusterset instance with global hub finalizer")
		managedClusterSet = &clusterv1beta1.ManagedClusterSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-managedclusterset-1",
				Namespace:  "default",
				Finalizers: []string{commonconstants.GlobalHubCleanupFinalizer},
			},
		}
		Expect(runtimeClient.Create(ctx, managedClusterSet)).NotTo(HaveOccurred())
		Eventually(func() bool {
			return containGlobalFinalizer(managedClusterSet)
		}, 1*time.Second, 100*time.Millisecond).Should(BeTrue())

		By("Create placementbinding instance with global hub finalizer")
		placementbinding = &policyv1.PlacementBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-placementbinding-1",
				Namespace:  "default",
				Finalizers: []string{commonconstants.GlobalHubCleanupFinalizer},
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
		Expect(job.Run()).Should(Equal(0))

		By("Check the finalizer is exists in application")
		Eventually(func() bool {
			return containGlobalFinalizer(app)
		}, 1*time.Second, 100*time.Millisecond).Should(BeFalse())

		By("Check the finalizer is exists in subscription")
		Eventually(func() bool {
			return containGlobalFinalizer(appsub)
		}, 1*time.Second, 100*time.Millisecond).Should(BeFalse())

		By("Check the finalizer is exists in channel")
		Eventually(func() bool {
			return containGlobalFinalizer(chn)
		}, 1*time.Second, 100*time.Millisecond).Should(BeFalse())

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
		Eventually(func() bool {
			return containGlobalFinalizer(managedClusterSet)
		}, 1*time.Second, 100*time.Millisecond).Should(BeFalse())

		By("Check the finalizer is exists in managedclustersetbinding")
		Eventually(func() bool {
			return containGlobalFinalizer(managedClusterSetBinding)
		}, 1*time.Second, 100*time.Millisecond).Should(BeFalse())

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
	return controllerutil.ContainsFinalizer(object, commonconstants.GlobalHubCleanupFinalizer)
}
