package tests

import (
	"context"
	"fmt"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Check all the connection of clients and necessary parameter validation", Label("e2e-tests-validation"), func() {
	Context("Check all the clients could connect to the HoH servers", func() {
		It("connect to the apiserver with kubernetes interface", func() {
			hubClient := testClients.KubeClient()
			deployClient := hubClient.AppsV1().Deployments(testOptions.GlobalHub.Namespace)
			deployList, err := deployClient.List(context.TODO(), metav1.ListOptions{Limit: 2})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(len(deployList.Items) > 0).To(BeTrue())
		})

		It("check whether the cluster is running properly", func() {
			hubClient := testClients.KubeClient()
			healthy, err := hubClient.Discovery().RESTClient().Get().AbsPath("/healthz").DoRaw(context.TODO())
			Expect(err).ShouldNot(HaveOccurred())
			Expect(string(healthy)).To(Equal("ok"))
		})

	})

	Context("Check all the parameters for e2e-tests", func() {
		ManagedClusterNum := 1
		It("Check the num of hub clusters and managed clusters on options-local.yaml", func() {
			opt := testOptions.GlobalHub.ManagedHubs
			var leafhubClusters []string
			var managedClusters []string

			for _, c := range opt {
				leafhubClusters = append(leafhubClusters, c.Name)
				for _, h := range c.ManagedClusters {
					managedClusters = append(managedClusters, h.Name)
				}
			}
			if len(leafhubClusters) != ExpectedLeafHubNum || len(managedClusters) != ManagedClusterNum*ExpectedLeafHubNum {
				Expect(fmt.Errorf("generate %d hub cluster and %d managed cluster error", ExpectedLeafHubNum, ExpectedManagedClusterNum)).Should(Succeed())
			}
		})

		It("Check the num of hub clusters and managed clusters in the kubeconfig", func() {
			for i := 1; i <= ExpectedLeafHubNum; i++ {
				hubFileName := fmt.Sprintf("../../resources/kubeconfig/kubeconfig-hub%d", i)
				_, err := os.Stat(hubFileName)
				if os.IsNotExist(err) {
					Expect(fmt.Errorf("kubeconfig-hub%d is not exist", i)).Should(Succeed())
				}
				for j := 1; j <= ManagedClusterNum; j++ {
					managedFileName := fmt.Sprintf("../../resources/kubeconfig/kubeconfig-hub%d-cluster%d", i, j)
					_, err := os.Stat(managedFileName)
					if os.IsNotExist(err) {
						Expect(fmt.Errorf("kubeconfig-hub%d-cluster%d is not exist", i, j)).Should(Succeed())
					}
				}
			}
		})
	})
})
