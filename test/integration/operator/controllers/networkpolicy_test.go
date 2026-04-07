package controllers

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/networkpolicy"
	testutils "github.com/stolostron/multicluster-global-hub/test/integration/utils"
)

// go test ./test/integration/operator/controllers -ginkgo.focus "networkpolicy" -v
var _ = Describe("networkpolicy", Ordered, func() {
	var mgh *v1alpha4.MulticlusterGlobalHub
	var namespace string
	var reconciler *networkpolicy.NetworkPolicyReconciler

	BeforeAll(func() {
		namespace = fmt.Sprintf("namespace-%s", rand.String(6))
		mghName := "test-mgh"

		// Create namespace
		err := runtimeClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		})
		Expect(err).To(Succeed())

		// Create MGH
		mgh = &v1alpha4.MulticlusterGlobalHub{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mghName,
				Namespace: namespace,
			},
			Spec: v1alpha4.MulticlusterGlobalHubSpec{},
		}
		Expect(runtimeClient.Create(ctx, mgh)).To(Succeed())
		Expect(runtimeClient.Get(ctx, client.ObjectKeyFromObject(mgh), mgh)).To(Succeed())
	})

	It("should start the networkpolicy controller", func() {
		initOption := config.ControllerOption{
			Manager:    runtimeManager,
			KubeClient: kubeClient,
		}

		controller, err := networkpolicy.StartController(initOption)
		Expect(err).To(Succeed())
		Expect(controller).NotTo(BeNil())

		// Cast to get the reconciler for manual reconciliation
		var ok bool
		reconciler, ok = controller.(*networkpolicy.NetworkPolicyReconciler)
		Expect(ok).To(BeTrue())
	})

	It("should render and deploy networkpolicy manifests", func() {
		// Trigger reconciliation
		_, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: mgh.Namespace,
				Name:      mgh.Name,
			},
		})
		Expect(err).To(Succeed())

		// Verify default-deny-all NetworkPolicy is created
		Eventually(func() error {
			np := &networkingv1.NetworkPolicy{}
			return runtimeClient.Get(ctx, types.NamespacedName{
				Name:      "default-deny-all",
				Namespace: mgh.Namespace,
			}, np)
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

		// Verify allow-dns NetworkPolicy is created
		Eventually(func() error {
			np := &networkingv1.NetworkPolicy{}
			return runtimeClient.Get(ctx, types.NamespacedName{
				Name:      "allow-dns",
				Namespace: mgh.Namespace,
			}, np)
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

		// Verify multicluster-global-hub-operator NetworkPolicy is created
		Eventually(func() error {
			np := &networkingv1.NetworkPolicy{}
			return runtimeClient.Get(ctx, types.NamespacedName{
				Name:      "multicluster-global-hub-operator",
				Namespace: mgh.Namespace,
			}, np)
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})

	It("should verify networkpolicy content", func() {
		// Check default-deny-all policy content
		np := &networkingv1.NetworkPolicy{}
		err := runtimeClient.Get(ctx, types.NamespacedName{
			Name:      "default-deny-all",
			Namespace: mgh.Namespace,
		}, np)
		Expect(err).To(Succeed())
		Expect(np.Spec.PodSelector).NotTo(BeNil())
		Expect(np.Spec.PolicyTypes).To(ContainElements(
			networkingv1.PolicyTypeIngress,
			networkingv1.PolicyTypeEgress,
		))

		// Check operator NetworkPolicy has correct labels
		operatorNP := &networkingv1.NetworkPolicy{}
		err = runtimeClient.Get(ctx, types.NamespacedName{
			Name:      "multicluster-global-hub-operator",
			Namespace: mgh.Namespace,
		}, operatorNP)
		Expect(err).To(Succeed())
		Expect(operatorNP.Labels).To(HaveKey("global-hub.open-cluster-management.io/managed-by"))
		Expect(operatorNP.Spec.PodSelector.MatchLabels).To(HaveKeyWithValue("name", "multicluster-global-hub-operator"))
		Expect(operatorNP.Spec.PolicyTypes).To(ContainElement(networkingv1.PolicyTypeEgress))
	})

	It("should recreate networkpolicy when deleted", func() {
		// Delete one of the NetworkPolicies
		np := &networkingv1.NetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "default-deny-all",
				Namespace: mgh.Namespace,
			},
		}
		err := runtimeClient.Delete(ctx, np)
		Expect(err).To(Succeed())

		// Verify it's deleted
		Eventually(func() bool {
			err := runtimeClient.Get(ctx, types.NamespacedName{
				Name:      "default-deny-all",
				Namespace: mgh.Namespace,
			}, np)
			return errors.IsNotFound(err)
		}, 5*time.Second, 100*time.Millisecond).Should(BeTrue())

		// Trigger reconciliation
		_, err = reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: mgh.Namespace,
				Name:      mgh.Name,
			},
		})
		Expect(err).To(Succeed())

		// Verify NetworkPolicy is recreated
		Eventually(func() error {
			return runtimeClient.Get(ctx, types.NamespacedName{
				Name:      "default-deny-all",
				Namespace: mgh.Namespace,
			}, np)
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})

	It("should not reconcile when MGH is paused", func() {
		// Update MGH to be paused
		mgh.Annotations = map[string]string{
			"mgh-pause": "true",
		}
		err := runtimeClient.Update(ctx, mgh)
		Expect(err).To(Succeed())

		// Delete a NetworkPolicy
		np := &networkingv1.NetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "allow-dns",
				Namespace: mgh.Namespace,
			},
		}
		err = runtimeClient.Delete(ctx, np)
		Expect(err).To(Succeed())

		// Trigger reconciliation
		_, err = reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: mgh.Namespace,
				Name:      mgh.Name,
			},
		})
		Expect(err).To(Succeed())

		// Verify NetworkPolicy is NOT recreated (remains deleted)
		Consistently(func() bool {
			err := runtimeClient.Get(ctx, types.NamespacedName{
				Name:      "allow-dns",
				Namespace: mgh.Namespace,
			}, np)
			return errors.IsNotFound(err)
		}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())

		// Unpause MGH for cleanup
		mgh.Annotations = nil
		err = runtimeClient.Update(ctx, mgh)
		Expect(err).To(Succeed())
	})

	It("should not reconcile when MGH is being deleted", func() {
		// Get the current MGH
		err := runtimeClient.Get(ctx, client.ObjectKeyFromObject(mgh), mgh)
		Expect(err).To(Succeed())

		// Add finalizer to prevent immediate deletion
		mgh.Finalizers = []string{"test-finalizer"}
		err = runtimeClient.Update(ctx, mgh)
		Expect(err).To(Succeed())

		// Mark MGH for deletion
		err = runtimeClient.Delete(ctx, mgh)
		Expect(err).To(Succeed())

		// Verify MGH has deletion timestamp
		err = runtimeClient.Get(ctx, client.ObjectKeyFromObject(mgh), mgh)
		Expect(err).To(Succeed())
		Expect(mgh.DeletionTimestamp).NotTo(BeNil())

		// Trigger reconciliation
		result, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: mgh.Namespace,
				Name:      mgh.Name,
			},
		})
		Expect(err).To(Succeed())
		Expect(result.Requeue).To(BeFalse())

		// Remove finalizer to allow deletion
		mgh.Finalizers = nil
		err = runtimeClient.Update(ctx, mgh)
		Expect(err).To(Succeed())
	})

	AfterAll(func() {
		Eventually(func() error {
			if err := testutils.DeleteMgh(ctx, runtimeClient, mgh); err != nil {
				return err
			}
			return deleteNamespace(namespace)
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})
})
