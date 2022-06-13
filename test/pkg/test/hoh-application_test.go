package tests

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stolostron/hub-of-hubs/test/pkg/utils"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	appsv1alpha1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	APP_LABEL_KEY     = "app"
	APP_LABEL_VALUE   = "test"
	APP_SUB_YAML      = "../../resources/app/app-pacman-appsub.yaml"
	APP_SUB_NAME      = "pacman-appsub"
	APP_SUB_NAMESPACE = "pacman"
)

var _ = Describe("application", Label("app"), Ordered, func() {

	var token string
	var httpClient *http.Client
	var managedClusterName1 string
	var managedClusterName2 string
	var appClient client.Client

	BeforeAll(func() {
		By("Get token for the non-k8s-api")
		initToken, err := utils.FetchBearerToken(testOptions)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(len(initToken)).Should(BeNumerically(">", 0))
		token = initToken

		By("Config request of the api")
		transport := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		httpClient = &http.Client{Timeout: time.Second * 10, Transport: transport}

		By("Get a managed cluster name")
		managedClusters := getManagedCluster(httpClient, token)
		Expect(len(managedClusters)).Should(BeNumerically(">", 1), "at least 2 managed clusters")
		managedClusterName1 = managedClusters[0].Name
		managedClusterName2 = managedClusters[1].Name

		By("Get the appsubreport client")
		cfg, err := clients.RestConfig(utils.HUB_OF_HUB_CLUSTER_NAME)
		Expect(err).ShouldNot(HaveOccurred())
		scheme := runtime.NewScheme()
		appsv1alpha1.AddToScheme(scheme)
		appClient, err = client.New(cfg, client.Options{Scheme: scheme})
		Expect(err).ShouldNot(HaveOccurred())
	})

	It("add label to the managedcluster1", func() {
		patches := []patch{
			{
				Op:    "add",
				Path:  "/metadata/labels/" + APP_LABEL_KEY,
				Value: APP_LABEL_VALUE,
			},
		}
		updateClusterLabel(httpClient, patches, token, managedClusterName1)
		By("Check the label is added")
		Eventually(func() error {
			managedClusters := getManagedCluster(httpClient, token)
			for _, cluster := range managedClusters {
				if val, ok := cluster.Labels[APP_LABEL_KEY]; ok {
					if val == APP_LABEL_VALUE && cluster.Name == managedClusterName1 {
						return nil
					}
				}
			}
			return fmt.Errorf("the label %s: %s is not exist", APP_LABEL_KEY, APP_LABEL_VALUE)
		}, 5*60*time.Second, 5*1*time.Second).ShouldNot(HaveOccurred())
	})

	It("create the application/subscription", func() {
		By("Apply the appsub to labeled cluster")
		_, err := clients.Kubectl(utils.HUB_OF_HUB_CLUSTER_NAME, "apply", "-f", APP_SUB_YAML)
		Expect(err).ShouldNot(HaveOccurred())

		By("Check the appsub is applied to the cluster")
		Eventually(func() error {
			return checkAppsubreport(appClient, 1, 1, managedClusterName1)
		}, 5*60*time.Second, 5*1*time.Second).ShouldNot(HaveOccurred())
	})

	It("scale the application/subscription by label", func() {
		By("Add the lablel to managedcluster2")
		patches := []patch{
			{
				Op:    "add",
				Path:  "/metadata/labels/" + APP_LABEL_KEY,
				Value: APP_LABEL_VALUE,
			},
		}
		updateClusterLabel(httpClient, patches, token, managedClusterName2)

		By("Check the label is added to managedcluster2")
		Eventually(func() error {
			managedClusters := getManagedCluster(httpClient, token)
			for _, cluster := range managedClusters {
				if val, ok := cluster.Labels[APP_LABEL_KEY]; ok {
					if val == APP_LABEL_VALUE && cluster.Name == managedClusterName2 {
						return nil
					}
				}
			}
			return fmt.Errorf("the label %s: %s is not exist", APP_LABEL_KEY, APP_LABEL_VALUE)
		}, 5*60*time.Second, 5*1*time.Second).ShouldNot(HaveOccurred())

		By("Check the appsub apply to the clusters")
		Eventually(func() error {
			err := checkAppsubreport(appClient, 2, 2, managedClusterName1)
			if err != nil {
				return err
			}
			err = checkAppsubreport(appClient, 2, 2, managedClusterName2)
			if err != nil {
				return err
			}
			return nil
		}, 5*60*time.Second, 5*1*time.Second).ShouldNot(HaveOccurred())
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
		updateClusterLabel(httpClient, patches, token, managedClusterName1)
		updateClusterLabel(httpClient, patches, token, managedClusterName2)

		By("Check label is removed from clusters")
		Eventually(func() error {
			managedClusters := getManagedCluster(httpClient, token)
			for _, cluster := range managedClusters {
				if val, ok := cluster.Labels[APP_LABEL_KEY]; ok {
					if val == APP_LABEL_VALUE {
						return fmt.Errorf("the label %s: %s is not removed from %s", APP_LABEL_KEY, APP_LABEL_VALUE, cluster.Name)
					}
				}
			}
			return nil
		}, 5*60*time.Second, 5*1*time.Second).ShouldNot(HaveOccurred())

		By("Remove the appsub resource")
		_, err := clients.Kubectl(utils.HUB_OF_HUB_CLUSTER_NAME, "delete", "-f", APP_SUB_YAML)
		Expect(err).ShouldNot(HaveOccurred())
	})
})

func checkAppsubreport(appClient client.Client, expectDeployNum, expectClusterNum int, expectClusterName string) error {
	appsubreport := &appsv1alpha1.SubscriptionReport{}
	err := appClient.Get(context.TODO(), types.NamespacedName{Namespace: APP_SUB_NAMESPACE, Name: APP_SUB_NAME}, appsubreport)
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
	klog.V(5).Info("# cluster: ", clusterNum)
	// TODO if deployNum == expectDeployNum && clusterNum == expectClusterNum {
	if deployNum == expectDeployNum {
		if expectClusterNum == 0 && expectDeployNum == 0 {
			return nil
		}
		for _, res := range appsubreport.Results {
			if res.Result == "deployed" && res.Source == expectClusterName {
				return nil
			}
		}
		return fmt.Errorf("deploy results isn't correct %v", appsubreport.Results)
	}
	return fmt.Errorf("the appsub %s: %s hasn't deplyed to the cluster: %s", APP_SUB_NAMESPACE, APP_SUB_NAME, expectClusterName)
}
