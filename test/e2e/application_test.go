package tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	appsv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1"
	appsv1alpha1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1alpha1"
)

const (
	APP_LABEL_KEY     = "app"
	APP_LABEL_VALUE   = "test"
	APP_SUB_YAML      = "./manifests/app/app-helloworld-appsub.yaml"
	APP_SUB_NAME      = "helloworld-appsub"
	APP_SUB_NAMESPACE = "helloworld"
)

var _ = Describe("Deploy the application to the managed cluster", Label("e2e-test-app"), Ordered, func() {
	Context("deploy the application", func() {
		It("deploy the application/subscription", func() {
			By("Add label to the managedcluster")
			assertAddLabel(managedClusters[0], APP_LABEL_KEY, APP_LABEL_VALUE)

			By("Check the appsub is applied to the cluster")
			Eventually(func() error {
				_, err := testClients.Kubectl(testOptions.GlobalHub.Name, "apply", "-f", APP_SUB_YAML)
				if err != nil {
					return err
				}
				return checkAppsubreport(httpClient, APP_SUB_NAME, APP_SUB_NAMESPACE, 1,
					[]string{managedClusters[0].Name})
			}, 5*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())
		})

		It("scale the application/subscription by adding label to cluster", func() {
			By("Add label to the managedcluster")
			assertAddLabel(managedClusters[1], APP_LABEL_KEY, APP_LABEL_VALUE)

			By("Check the appsub apply to the clusters")
			Eventually(func() error {
				return checkAppsubreport(httpClient, APP_SUB_NAME, APP_SUB_NAMESPACE, 2,
					[]string{managedClusters[0].Name, managedClusters[1].Name})
			}, 5*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())
		})

		AfterEach(func() {
			if CurrentSpecReport().Failed() {
				appsubreport, err := getAppsubReport(httpClient, APP_SUB_NAME, APP_SUB_NAMESPACE)
				if err == nil {
					appsubreportStr, _ := json.MarshalIndent(appsubreport, "", "  ")
					klog.V(5).Info("Appsubreport: ", string(appsubreportStr))
				}
			}
		})
	})

	AfterAll(func() {
		By("Remove label from clusters")
		Eventually(func() error {
			for _, managedCluster := range managedClusters {
				assertRemoveLabel(managedCluster, APP_LABEL_KEY, APP_LABEL_VALUE)
			}
			return nil
		}, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())

		By("Remove the appsub resource")
		Eventually(func() error {
			_, err := testClients.Kubectl(testOptions.GlobalHub.Name, "delete", "-f", APP_SUB_YAML)
			if err != nil {
				return err
			}
			return nil
		}, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())
	})
})

func getAppsubReport(httpClient *http.Client, name, namespace string,
) (*appsv1alpha1.SubscriptionReport, error) {
	appClient, err := testClients.RuntimeClient(testOptions.GlobalHub.Name, operatorScheme)
	if err != nil {
		return nil, err
	}

	appsubreport := &appsv1alpha1.SubscriptionReport{}
	appsub := &appsv1.Subscription{}
	err = appClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, appsub)
	if err != nil {
		return nil, err
	}

	appsubUID := string(appsub.GetUID())
	getSubscriptionReportURL := fmt.Sprintf("%s/global-hub-api/v1/subscriptionreport/%s",
		testOptions.GlobalHub.Nonk8sApiServer, appsubUID)
	req, err := http.NewRequest("GET", getSubscriptionReportURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(body, appsubreport)
	if err != nil {
		return nil, err
	}

	return appsubreport, nil
}

func checkAppsubreport(httpClient *http.Client, name, namespace string,
	expectDeployNum int, expectClusterNames []string,
) error {
	appsubreport, err := getAppsubReport(httpClient, name, namespace)
	if err != nil {
		return err
	}
	deployNum, err := strconv.Atoi(appsubreport.Summary.Deployed)
	if err != nil {
		return err
	}
	clusterNum, err := strconv.Atoi(appsubreport.Summary.Clusters)
	if err != nil {
		return err
	}
	if deployNum >= expectDeployNum && clusterNum >= len(expectClusterNames) {
		matchedClusterNum := 0
		for _, expectClusterName := range expectClusterNames {
			for _, res := range appsubreport.Results {
				if res.Result == "deployed" && res.Source == expectClusterName {
					matchedClusterNum++
				}
			}
		}
		if matchedClusterNum == len(expectClusterNames) {
			return nil
		}
		return fmt.Errorf("deploy results isn't correct %v", appsubreport.Results)
	}
	return fmt.Errorf("the appsub %s: %s hasn't deployed to the cluster: %s", APP_SUB_NAMESPACE,
		APP_SUB_NAME, strings.Join(expectClusterNames, ","))
}
