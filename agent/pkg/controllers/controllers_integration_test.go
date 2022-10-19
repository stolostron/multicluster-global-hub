// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controllers_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clustersv1alpha1 "open-cluster-management.io/api/cluster/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/controllers"
)

const (
	MCHVersion = "2.6.0"
)

var _ = Describe("controller", Ordered, func() {
	ctx, cancel := context.WithCancel(context.Background())
	var mgr ctrl.Manager

	BeforeEach(func() {
		Expect(cfg).NotTo(BeNil())
	})

	It("create a manager", func() {
		By("Creating the Manager")
		var err error
		mgr, err = ctrl.NewManager(cfg, ctrl.Options{
			MetricsBindAddress: "0", // disable the metrics serving
		})
		Expect(err).NotTo(HaveOccurred())

		By("Adding the controllers to the manager")
		Expect(controllers.AddToManager(mgr)).NotTo(HaveOccurred())

		go func() {
			defer GinkgoRecover()
			Expect(mgr.Start(ctx)).NotTo(HaveOccurred())
		}()

		By("Waiting for the manager to be ready")
		Expect(mgr.GetCache().WaitForCacheSync(ctx)).To(BeTrue())
	})

	It("clusterclaim testing", func() {
		By("Create MCH instance to trigger reconciliation")
		Expect(mgr.GetClient().Create(ctx, &mchv1.MultiClusterHub{
			TypeMeta: metav1.TypeMeta{
				Kind:       "MultiClusterHub",
				APIVersion: "operator.open-cluster-management.io/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "multiclusterhub",
				Namespace: "default",
			},
			Spec: mchv1.MultiClusterHubSpec{},
		})).NotTo(HaveOccurred())

		mch := &mchv1.MultiClusterHub{}
		Eventually(func() bool {
			err := mgr.GetClient().Get(ctx, types.NamespacedName{
				Name:      "multiclusterhub",
				Namespace: "default",
			}, mch)
			return err == nil
		}, 1*time.Second, 100*time.Millisecond).Should(BeTrue())

		mch.Status = mchv1.MultiClusterHubStatus{CurrentVersion: MCHVersion}
		Expect(mgr.GetClient().Status().Update(ctx, mch)).NotTo(HaveOccurred())

		By("Expect clusterClaim to be created")
		clusterClaim := &clustersv1alpha1.ClusterClaim{}
		Eventually(func() bool {
			err := mgr.GetClient().Get(ctx, types.NamespacedName{
				Name: "version.open-cluster-management.io",
			}, clusterClaim)
			if err != nil {
				return false
			}
			if clusterClaim.Spec.Value == "" {
				return false
			}
			return true
		}, 1*time.Second, 100*time.Millisecond).Should(BeTrue())
		Expect(clusterClaim.Spec.Value).Should(Equal(MCHVersion))

		By("Expect clusterClaim version to be updated")
		mch.Status = mchv1.MultiClusterHubStatus{CurrentVersion: "2.7.0"}
		Expect(mgr.GetClient().Status().Update(ctx, mch)).NotTo(HaveOccurred())
		clusterClaim = &clustersv1alpha1.ClusterClaim{}
		Eventually(func() bool {
			err := mgr.GetClient().Get(ctx, types.NamespacedName{
				Name: "version.open-cluster-management.io",
			}, clusterClaim)
			return err == nil && clusterClaim.Spec.Value == "2.7.0"
		}, 1*time.Second, 100*time.Millisecond).Should(BeTrue())

		By("Expect clusterClaim to be re-created once it is deleted")
		Expect(mgr.GetClient().Delete(context.Background(), &clustersv1alpha1.ClusterClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: "version.open-cluster-management.io",
			},
		})).NotTo(HaveOccurred())

		newClusterClaim := &clustersv1alpha1.ClusterClaim{}
		Eventually(func() bool {
			err := mgr.GetClient().Get(ctx, types.NamespacedName{
				Name: "version.open-cluster-management.io",
			}, newClusterClaim)
			fmt.Fprintf(GinkgoWriter, "the old ClusterClaim: %v\n", clusterClaim)
			fmt.Fprintf(GinkgoWriter, "the new ClusterClaim: %v\n", newClusterClaim)
			return err == nil && clusterClaim.GetResourceVersion() != newClusterClaim.GetResourceVersion()
		}, 1*time.Second, 100*time.Millisecond).Should(BeTrue())
	})

	AfterAll(func() {
		defer cancel()
	})
})
