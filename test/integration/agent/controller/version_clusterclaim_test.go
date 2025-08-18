// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clustersv1alpha1 "open-cluster-management.io/api/cluster/v1alpha1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

const (
	MCHVersion = "2.6.0"
)

var _ = Describe("claim controllers", Ordered, func() {
	BeforeEach(func() {
		cleanup(ctx, mgr.GetClient())
	})

	AfterAll(func() {
		cleanup(ctx, mgr.GetClient())
	})
	It("clusterClaim testing only clusterManager is installed", func() {
		By("Create clusterManager instance")
		Expect(mgr.GetClient().Create(ctx, &operatorv1.ClusterManager{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cluster-manager",
			},
			Spec: operatorv1.ClusterManagerSpec{},
		})).NotTo(HaveOccurred())

		By("Create clusterClaim to trigger hubClaim controller")
		Expect(mgr.GetClient().Create(ctx, &clustersv1alpha1.ClusterClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test2",
			},
			Spec: clustersv1alpha1.ClusterClaimSpec{},
		})).NotTo(HaveOccurred())

		clusterClaim := &clustersv1alpha1.ClusterClaim{}

		Eventually(func() error {
			return mgr.GetClient().Get(ctx, types.NamespacedName{
				Name: constants.HubClusterClaimName,
			}, clusterClaim)
		}, 3*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
		Expect(clusterClaim.Spec.Value).Should(Equal(
			constants.HubInstalledByUser))
	})

	It("clusterClaim testing clusterManager and mch are not installed", func() {
		By("Create clusterClaim to trigger hubClaim controller")
		Expect(mgr.GetClient().Create(ctx, &clustersv1alpha1.ClusterClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
			},
			Spec: clustersv1alpha1.ClusterClaimSpec{},
		})).NotTo(HaveOccurred())

		clusterClaim := &clustersv1alpha1.ClusterClaim{}

		Eventually(func() error {
			err := mgr.GetClient().Get(ctx, types.NamespacedName{
				Name: constants.HubClusterClaimName,
			}, clusterClaim)
			if err != nil {
				return err
			}
			if clusterClaim.Spec.Value == constants.HubNotInstalled {
				return nil
			}
			return fmt.Errorf("the claim(%s) expect %s, but got %s", constants.HubClusterClaimName,
				constants.HubNotInstalled, clusterClaim.Spec.Value)
		}, 3*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})

	It("clusterclaim testing", func() {
		By("Create MCH instance to trigger reconciliation")
		Expect(mgr.GetClient().Create(ctx, &mchv1.MultiClusterHub{
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
				Name: constants.VersionClusterClaimName,
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

		Eventually(func() bool {
			err := mgr.GetClient().Get(ctx, types.NamespacedName{
				Name: constants.HubClusterClaimName,
			}, clusterClaim)
			if err != nil {
				return false
			}
			if clusterClaim.Spec.Value != constants.HubInstalledByUser {
				return false
			}
			return true
		}, 1*time.Second, 100*time.Millisecond).Should(BeTrue())
		Expect(clusterClaim.Spec.Value).Should(Equal(
			constants.HubInstalledByUser))

		By("Expect clusterClaim version to be updated")
		mch.Status = mchv1.MultiClusterHubStatus{CurrentVersion: "2.7.0"}
		Expect(mgr.GetClient().Status().Update(ctx, mch)).NotTo(HaveOccurred())
		clusterClaim = &clustersv1alpha1.ClusterClaim{}
		Eventually(func() bool {
			err := mgr.GetClient().Get(ctx, types.NamespacedName{
				Name: constants.VersionClusterClaimName,
			}, clusterClaim)
			return err == nil && clusterClaim.Spec.Value == "2.7.0"
		}, 1*time.Second, 100*time.Millisecond).Should(BeTrue())

		By("Expect clusterClaim to be re-created once it is deleted")
		Expect(mgr.GetClient().Delete(context.Background(), &clustersv1alpha1.ClusterClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: constants.VersionClusterClaimName,
			},
		})).NotTo(HaveOccurred())

		newClusterClaim := &clustersv1alpha1.ClusterClaim{}
		Eventually(func() bool {
			err := mgr.GetClient().Get(ctx, types.NamespacedName{
				Name: constants.VersionClusterClaimName,
			}, newClusterClaim)
			if _, err := fmt.Fprintf(GinkgoWriter, "the old ClusterClaim: %v\n", clusterClaim); err != nil {
				panic(err)
			}
			if _, err := fmt.Fprintf(GinkgoWriter, "the new ClusterClaim: %v\n", newClusterClaim); err != nil {
				panic(err)
			}
			return err == nil && clusterClaim.GetResourceVersion() != newClusterClaim.GetResourceVersion()
		}, 1*time.Second, 100*time.Millisecond).Should(BeTrue())

		By("Expect hub clusterClaim is updated")
		mch.Spec.DisableHubSelfManagement = true
		Expect(mgr.GetClient().Update(ctx, mch)).NotTo(HaveOccurred())
		clusterClaim = &clustersv1alpha1.ClusterClaim{}
		Eventually(func() bool {
			err := mgr.GetClient().Get(ctx, types.NamespacedName{
				Name: constants.HubClusterClaimName,
			}, clusterClaim)
			if err != nil {
				return false
			}
			if clusterClaim.Spec.Value != constants.HubInstalledByUser {
				return false
			}
			return true
		}, 1*time.Second, 100*time.Millisecond).Should(BeTrue())
		Expect(clusterClaim.Spec.Value).Should(Equal(
			constants.HubInstalledByUser))
	})
})

func cleanup(ctx context.Context, client client.Client) {
	Eventually(func() error {
		err := client.Delete(ctx, &mchv1.MultiClusterHub{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "multiclusterhub",
				Namespace: "default",
			},
			Spec: mchv1.MultiClusterHubSpec{},
		})
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}, 1*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

	Eventually(func() error {
		err := client.Delete(ctx, &operatorv1.ClusterManager{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cluster-manager",
			},
			Spec: operatorv1.ClusterManagerSpec{},
		})
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}, 1*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

	Eventually(func() error {
		err := client.Delete(ctx, &clustersv1alpha1.ClusterClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: constants.HubClusterClaimName,
			},
			Spec: clustersv1alpha1.ClusterClaimSpec{},
		})
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}, 1*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	Eventually(func() error {
		err := client.Delete(ctx, &clustersv1alpha1.ClusterClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: constants.VersionClusterClaimName,
			},
			Spec: clustersv1alpha1.ClusterClaimSpec{},
		})
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}, 1*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
}
