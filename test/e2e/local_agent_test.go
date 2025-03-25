package tests

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
)

var _ = Describe("Local Agent", Label("e2e-test-local-agent"), Ordered, func() {
	It("enable local agent in globalhub", func() {
		Eventually(func() error {
			mgh := &globalhubv1alpha4.MulticlusterGlobalHub{}
			err := globalHubClient.Get(ctx, types.NamespacedName{
				Namespace: testOptions.GlobalHub.Namespace,
				Name:      "multiclusterglobalhub",
			}, mgh)
			if err != nil {
				return err
			}
			mgh.Spec.InstallAgentOnLocal = true
			return globalHubClient.Update(ctx, mgh)
		}, 3*time.Minute, 1*time.Second).Should(Succeed())
	})
	It("local agent deployment should exist in globalhub", func() {
		// Check agent lease updated
		Eventually(func() error {
			err := checkDeployAvailable(globalHubClient, testOptions.GlobalHub.Namespace, "multicluster-global-hub-agent")
			if err != nil {
				return err
			}
			updated, err := isLeaseUpdated("multicluster-global-hub-agent-lock",
				testOptions.GlobalHub.Namespace, "multicluster-global-hub-agent", globalHubClient)
			if err != nil {
				return err
			}
			if !updated {
				return fmt.Errorf("agent lease not updated")
			}
			return nil
		}, 5*time.Minute, 1*time.Second).Should(Succeed())
	})

	It("validate the clusters on database", func() {
		Eventually(func() (err error) {
			curManagedCluster, err := getManagedCluster(httpClient)
			if err != nil {
				return err
			}
			if len(curManagedCluster) != (ExpectedMH*ExpectedMC + 2) {
				return fmt.Errorf("managed cluster number: want %d, got %d", (ExpectedMH*ExpectedMC + 2), len(curManagedCluster))
			}
			return nil
		}, 6*time.Minute, 10*time.Second).ShouldNot(HaveOccurred())
	})

	It("disable local agent in globalhub", func() {
		Eventually(func() error {
			mgh := &globalhubv1alpha4.MulticlusterGlobalHub{}
			err := globalHubClient.Get(ctx, types.NamespacedName{
				Namespace: testOptions.GlobalHub.Namespace,
				Name:      "multiclusterglobalhub",
			}, mgh)
			if err != nil {
				return err
			}
			mgh.Spec.InstallAgentOnLocal = false
			return globalHubClient.Update(ctx, mgh)
		}, 3*time.Minute, 1*time.Second).Should(Succeed())
	})
	It("local agent deploy should remove in globalhub", func() {
		// Check agent lease updated
		Eventually(func() error {
			err := checkDeployAvailable(globalHubClient, testOptions.GlobalHub.Namespace, "multicluster-global-hub-agent")
			if err != nil && errors.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("agent should be removed, but still exist. err: %v", err)
		}, 5*time.Minute, 1*time.Second).Should(Succeed())
	})
})
