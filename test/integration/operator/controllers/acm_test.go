package controllers

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/acm"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// go test ./test/integration/operator/controllers -ginkgo.focus "MulticlusterhubController" -v
var _ = Describe("MulticlusterhubController", Ordered, func() {
	var controller *acm.ACMResourceController
	var mch *mchv1.MultiClusterHub
	var namespace string
	var mgh *v1alpha4.MulticlusterGlobalHub
	mchName := "test-mch"

	BeforeAll(func() {
		// Initialize the controller and necessary resources
		controller = &acm.ACMResourceController{
			Manager:   runtimeManager, // assuming testManager is set up for testing
			Resources: make(map[string]bool),
		}

		namespace = fmt.Sprintf("namespace-%s", rand.String(6))

		Expect(runtimeClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		})).To(Succeed())

		mgh = &v1alpha4.MulticlusterGlobalHub{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "multiclusterglobalhub",
				Namespace: namespace,
			},
			Spec: v1alpha4.MulticlusterGlobalHubSpec{
				EnableMetrics: true,
			},
		}
		Expect(runtimeClient.Create(ctx, mgh)).To(Succeed())
		config.SetMGHNamespacedName(types.NamespacedName{Namespace: mgh.Namespace, Name: mgh.Name})
	})

	AfterAll(func() {
		// Clean up created resources
		Expect(runtimeClient.Delete(ctx, mch)).To(Succeed())
		Expect(runtimeClient.Delete(ctx, mgh)).To(Succeed())
	})

	It("should set ACMResourceReady to false and requeue", func() {
		req := reconcile.Request{
			NamespacedName: client.ObjectKey{
				Name:      mchName,
				Namespace: namespace,
			},
		}

		mch = &mchv1.MultiClusterHub{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mchName,
				Namespace: namespace,
			},
			Status: mchv1.MultiClusterHubStatus{
				Phase: mchv1.HubPending, // Initialize to non-running state
			},
		}
		Expect(runtimeClient.Create(ctx, mch)).To(Succeed())
		time.Sleep(1 * time.Second)

		_, err := controller.Reconcile(ctx, req)
		utils.PrettyPrint(err)
		Expect(err).ToNot(HaveOccurred())

		err = runtimeClient.Get(ctx, client.ObjectKeyFromObject(mgh), mgh)
		Expect(err).ToNot(HaveOccurred())
		// utils.PrettyPrint(mgh.Status)
	})

	It("should set ACMResourceReady to true", func() {
		req := reconcile.Request{
			NamespacedName: client.ObjectKey{
				Name:      mchName,
				Namespace: namespace,
			},
		}

		// Update MCH resource to running phase
		mch.Status.Phase = mchv1.HubRunning
		Expect(runtimeClient.Status().Update(ctx, mch)).To(Succeed())

		Eventually(func() error {
			Expect(runtimeClient.Get(ctx, client.ObjectKeyFromObject(mch), mch))
			if mch.Status.Phase != mchv1.HubRunning {
				return fmt.Errorf("mch status not running, %v", mch.Status)
			}
			return nil
		}, 3*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

		_, err := controller.Reconcile(ctx, req)
		Expect(err).ToNot(HaveOccurred())

		// Validate if ACMResourceReady was set to true
		Expect(config.IsACMResourceReady()).To(BeTrue())

		err = runtimeClient.Get(ctx, client.ObjectKeyFromObject(mgh), mgh)
		Expect(err).ToNot(HaveOccurred())
		utils.PrettyPrint(mgh.Status)
	})
})
