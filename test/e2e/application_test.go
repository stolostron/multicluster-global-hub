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

	pkgutils "github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const (
	APP_LABEL_KEY     = "app"
	APP_LABEL_VALUE   = "test"
	APP_SUB_YAML      = "./../manifest/app/app-helloworld-appsub.yaml"
	APP_SUB_NAME      = "helloworld-appsub"
	APP_SUB_NAMESPACE = "helloworld"
	Timeout           = 5 * time.Minute
	PollInterval      = 1 * time.Second
)

var _ = Describe("Deploy the application to the managed cluster", Label("e2e-test-app"), Ordered, func() {
	BeforeAll(func() {
		By("Creating the appsub in global hub")
		_, err := testClients.Kubectl(testOptions.GlobalHub.Name, "apply", "-f", APP_SUB_YAML)
		Expect(err).ToNot(HaveOccurred(), "Failed to create appsub")
	})

	Context("Deploying and scaling the application", func() {
		It("Deploys the application to the first managed cluster", func() {
			By(fmt.Sprintf("Adding label %s=%s to managed cluster %s", APP_LABEL_KEY, APP_LABEL_VALUE, managedClusters[0].Name))
			assertAddLabel(managedClusters[0], APP_LABEL_KEY, APP_LABEL_VALUE)

			By(fmt.Sprintf("Verifying appsub is applied to cluster %s", managedClusters[0].Name))
			Eventually(func() error {
				return checkAppsubreport(httpClient, APP_SUB_NAME, APP_SUB_NAMESPACE, 1, []string{managedClusters[0].Name})
			}, Timeout, PollInterval).ShouldNot(HaveOccurred(), "Appsub not applied to cluster %s", managedClusters[0].Name)
		})

		It("Scales the application by adding label to another cluster", func() {
			By(fmt.Sprintf("Adding label %s=%s to managed cluster %s", APP_LABEL_KEY, APP_LABEL_VALUE, managedClusters[1].Name))
			assertAddLabel(managedClusters[1], APP_LABEL_KEY, APP_LABEL_VALUE)

			By(fmt.Sprintf("Verifying appsub is applied to clusters %s and %s", managedClusters[0].Name, managedClusters[1].Name))
			Eventually(func() error {
				return checkAppsubreport(httpClient, APP_SUB_NAME, APP_SUB_NAMESPACE, 2, []string{managedClusters[0].Name, managedClusters[1].Name})
			}, Timeout, PollInterval).ShouldNot(HaveOccurred(), "Appsub not scaled to cluster %s", managedClusters[1].Name)
		})

		AfterEach(func() {
			if CurrentSpecReport().Failed() {
				appsubreport, err := getAppsubReport(httpClient, APP_SUB_NAME, APP_SUB_NAMESPACE)
				if err != nil {
					klog.Errorf("Failed to fetch appsubreport: %v", err)
					return
				}
				fmt.Println(">> appsubreport: ")
				pkgutils.PrettyPrint(appsubreport)
			}
		})
	})

	AfterAll(func() {
		By("Removing labels from managed clusters")
		for _, cluster := range managedClusters {
			By(fmt.Sprintf("Removing label %s=%s from cluster %s", APP_LABEL_KEY, APP_LABEL_VALUE, cluster.Name))
			assertRemoveLabel(cluster, APP_LABEL_KEY, APP_LABEL_VALUE)
		}

		By("Deleting the appsub resource")
		Eventually(func() error {
			_, err := testClients.Kubectl(testOptions.GlobalHub.Name, "delete", "-f", APP_SUB_YAML)
			return err
		}, time.Minute, PollInterval).ShouldNot(HaveOccurred(), "Failed to delete appsub")
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
		return fmt.Errorf("failed to fetch appsubreport for %s/%s: %w", namespace, name, err)
	}

	// Parse deployed and cluster counts
	deployNum, err := strconv.Atoi(appsubreport.Summary.Deployed)
	if err != nil {
		return fmt.Errorf("invalid deployed count %q in appsubreport: %w", appsubreport.Summary.Deployed, err)
	}
	clusterNum, err := strconv.Atoi(appsubreport.Summary.Clusters)
	if err != nil {
		return fmt.Errorf("invalid cluster count %q in appsubreport: %w", appsubreport.Summary.Clusters, err)
	}

	// Validate deployment and cluster counts
	if deployNum < expectDeployNum {
		return fmt.Errorf("expected %d deployments, got %d for appsub %s/%s", expectDeployNum, deployNum, namespace, name)
	}

	// Check if all expected clusters are deployed
	matchedClusters := make(map[string]bool)
	for _, cluster := range expectClusterNames {
		matchedClusters[cluster] = false
	}
	for _, res := range appsubreport.Results {
		if res.Result == "deployed" {
			matchedClusters[res.Source] = true
		} else {
			fmt.Printf(">> appsub %s/%s not deployed to cluster %s, result: %s\n", namespace, name, res.Source, res.Result)
		}
	}

	// Verify all expected clusters were matched
	var missingClusters []string
	for cluster, matched := range matchedClusters {
		if !matched {
			missingClusters = append(missingClusters, cluster)
		}
	}

	if len(missingClusters) > 0 {
		fmt.Println(">> appsub should been deployed to the clusters: ", expectClusterNames)
		return fmt.Errorf("appsub %s/%s not deployed to clusters: %s", namespace, name, strings.Join(missingClusters, ", "))
	}

	// Validate deployment and cluster counts
	if deployNum < expectDeployNum {
		return fmt.Errorf("expected %d deployments, got %d for appsub %s/%s", expectDeployNum, deployNum, namespace, name)
	}
	if clusterNum < len(expectClusterNames) {
		return fmt.Errorf("expected at least %d clusters, got %d for appsub %s/%s", len(expectClusterNames), clusterNum, namespace, name)
	}
	return nil
}
