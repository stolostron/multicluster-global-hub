package tests

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	appsv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1"
	appsv1alpha1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	APP_LABEL_KEY     = "app"
	APP_LABEL_VALUE   = "test"
	APP_SUB_YAML      = "../../resources/app/app-helloworld-appsub.yaml"
	APP_SUB_NAME      = "helloworld-appsub"
	APP_SUB_NAMESPACE = "helloworld"
)

var _ = Describe("Deploy the application to the managed cluster", Label("e2e-tests-app"), Ordered, func() {
	var appClient client.Client
	var managedClusters []clusterv1.ManagedCluster

	BeforeAll(func() {
		Eventually(func() error {
			By("Config request of the api")
			transport := &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			}
			httpClient = &http.Client{Timeout: time.Second * 20, Transport: transport}
			var err error
			managedClusters, err = getManagedCluster(httpClient, httpToken)
			if err != nil {
				return err
			}
			if len(managedClusters) != ExpectedManagedClusterNum {
				return fmt.Errorf("managed cluster is not exist")
			}
			return nil
		}, 3*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())

		By("Get the appsubreport client")
		scheme := runtime.NewScheme()
		appsv1.SchemeBuilder.AddToScheme(scheme)
		appsv1alpha1.AddToScheme(scheme)
		var err error
		appClient, err = clients.ControllerRuntimeClient(GlobalHubName, scheme)
		Expect(err).ShouldNot(HaveOccurred())
	})

	It(fmt.Sprintf("add the app label[ %s: %s ]", APP_LABEL_KEY, APP_LABEL_VALUE), func() {
		By("Add label to the managedcluster")
		patches := []patch{
			{
				Op:    "add",
				Path:  "/metadata/labels/" + APP_LABEL_KEY,
				Value: APP_LABEL_VALUE,
			},
		}

		By("Check the label is added")
		Eventually(func() error {
			err := updateClusterLabel(httpClient, patches, httpToken, GetClusterID(managedClusters[0]))
			if err != nil {
				return err
			}
			managedClusterInfo, err := getManagedClusterByName(httpClient, httpToken, managedClusters[0].Name)
			if err != nil {
				return err
			}
			if val, ok := managedClusterInfo.Labels[APP_LABEL_KEY]; ok {
				if val == APP_LABEL_VALUE && managedClusterInfo.Name == managedClusters[0].Name {
					return nil
				}
			}
			return fmt.Errorf("the label %s: %s is not exist", APP_LABEL_KEY, APP_LABEL_VALUE)
		}, 3*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())
	})

	Context("deploy the application", func() {
		It("deploy the application/subscription", func() {
			By("Check the appsub is applied to the cluster")
			Eventually(func() error {
				_, err := clients.Kubectl(GlobalHubName, "apply", "-f", APP_SUB_YAML)
				if err != nil {
					return err
				}
				return checkAppsubreport(appClient, httpClient, APP_SUB_NAME, APP_SUB_NAMESPACE, httpToken, 1,
					[]string{managedClusters[0].Name})
			}, 5*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())
		})

		for i := 1; i < len(managedClusters); i++ {
			It(fmt.Sprintf("Add the app label[ %s: %s ]", APP_LABEL_KEY, APP_LABEL_VALUE), func() {
				By("Add the lablel to managedcluster")
				patches := []patch{
					{
						Op:    "add",
						Path:  "/metadata/labels/" + APP_LABEL_KEY,
						Value: APP_LABEL_VALUE,
					},
				}

				By("Check the label is added to managedcluster")
				Eventually(func() error {
					err := updateClusterLabel(httpClient, patches, httpToken, GetClusterID((managedClusters[i])))
					if err != nil {
						return err
					}
					managedClusterInfo, err := getManagedClusterByName(httpClient, httpToken, managedClusters[i].Name)
					if err != nil {
						return err
					}
					if val, ok := managedClusterInfo.Labels[APP_LABEL_KEY]; ok {
						if val == APP_LABEL_VALUE && managedClusterInfo.Name == managedClusters[i].Name {
							return nil
						}
					}
					return fmt.Errorf("the label %s: %s is not exist", APP_LABEL_KEY, APP_LABEL_VALUE)
				}, 3*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())

				By("Check the appsub apply to the clusters")
				Eventually(func() error {
					return checkAppsubreport(appClient, httpClient, APP_SUB_NAME, APP_SUB_NAMESPACE, httpToken, 2, []string{managedClusters[0].Name, managedClusters[1].Name})
				}, 5*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())
			})
		}

		AfterEach(func() {
			if CurrentSpecReport().Failed() {
				appsubreport, err := getAppsubReport(appClient, httpClient, APP_SUB_NAME, APP_SUB_NAMESPACE, httpToken)
				if err == nil {
					appsubreportStr, _ := json.MarshalIndent(appsubreport, "", "  ")
					klog.V(5).Info("Appsubreport: ", string(appsubreportStr))
				}
			}
		})
	})

	AfterAll(func() {
		By("Remove from clusters")
		patches := []patch{
			{
				Op:    "remove",
				Path:  "/metadata/labels/" + APP_LABEL_KEY,
				Value: APP_LABEL_VALUE,
			},
		}
		Eventually(func() error {
			for _, managedCluster := range managedClusters {
				err := updateClusterLabel(httpClient, patches, httpToken, GetClusterID(managedCluster))
				if err != nil {
					return err
				}
			}
			return nil
		}, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())

		By("Remove the appsub resource")
		Eventually(func() error {
			_, err := clients.Kubectl(GlobalHubName, "delete", "-f", APP_SUB_YAML)
			if err != nil {
				return err
			}
			return nil
		}, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())
	})
})

func getAppsubReport(appClient client.Client, httpClient *http.Client, name, namespace,
	token string,
) (*appsv1alpha1.SubscriptionReport, error) {
	appsubreport := &appsv1alpha1.SubscriptionReport{}
	appsub := &appsv1.Subscription{}
	err := appClient.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: name}, appsub)
	if err != nil {
		return nil, err
	}

	appsubUID := string(appsub.GetUID())
	getSubscriptionReportURL := fmt.Sprintf("%s/global-hub-api/v1/subscriptionreport/%s",
		testOptions.HubCluster.Nonk8sApiServer, appsubUID)
	req, err := http.NewRequest("GET", getSubscriptionReportURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	klog.V(5).Info(fmt.Sprintf("get subscription report reponse body: \n%s\n", body))

	err = json.Unmarshal(body, appsubreport)
	if err != nil {
		return nil, err
	}

	return appsubreport, nil
}

func checkAppsubreport(appClient client.Client, httpClient *http.Client, name, namespace,
	token string, expectDeployNum int, expectClusterNames []string,
) error {
	appsubreport, err := getAppsubReport(appClient, httpClient, name, namespace, token)
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
