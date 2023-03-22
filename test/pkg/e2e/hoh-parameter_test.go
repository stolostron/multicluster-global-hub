package tests

import (
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	LEAF_HUB_NAME   = "hub"
	HUB_OF_HUB_NAME = "hub-of-hubs"
	CTX_HUB         = "microshift"
	CTX_MANAGED     = "kind-hub"
)


var _ = Describe("Check parameters for e2e-tests", Ordered, Label("e2e-tests-parameter"), func() {
	
	var HubClusterNum = 2
	var ManagedClusterNum = 1

	It("Check the num of hub clusters and managed clusters on options-local.yaml", func() {

		opt := testOptions.ManagedClusters

		var hubClusters []string
		var managedClusters []string

		Eventually(func() error {
			for _, c := range opt {
				if c.Name == c.LeafHubName {
					hubClusters = append(hubClusters, c.Name)
				} else {
					managedClusters = append(managedClusters, c.Name)
				}
			}
			
			if len(hubClusters) != HubClusterNum || len(managedClusters) != HubClusterNum*ManagedClusterNum {
				return fmt.Errorf("generate %d hub cluster and %d managed cluster error", HubClusterNum, ManagedClusterNum)
			}
			return nil
		}, 3*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())
	})

	It("Check the num of hub clusters and managed clusters in the kubeconfig", func() {
		Eventually(func() error {
			for i := 1; i <= HubClusterNum; i++ {
				hubFileName := fmt.Sprintf("../../resources/kubeconfig/kubeconfig-hub%d", i)
				_, err := os.Stat(hubFileName)
				if os.IsNotExist(err) {
					return fmt.Errorf("kubeconfig-hub%d is not exist", i)
				}
				for j := 1; j <= ManagedClusterNum; j++ {
					managedFileName := fmt.Sprintf("../../resources/kubeconfig/kubeconfig-hub%d-cluster%d", i, j)
					_, err := os.Stat(managedFileName)
					if os.IsNotExist(err) {
						return fmt.Errorf("kubeconfig-hub%d-cluster%d is not exist", i, j)
					}
				}
			}
			return nil
		}, 3*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())
	})
})
