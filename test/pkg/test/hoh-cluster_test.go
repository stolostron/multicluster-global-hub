package tests

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stolostron/hub-of-hubs/test/pkg/utils"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

const (
	CLUSTER_LABEL_KEY   = "cluster"
	CLUSTER_LABEL_VALUE = "test"
)

var _ = Describe("label", Ordered, func() {
	var token string
	var httpClient *http.Client
	var managedClusterName string

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
		managedClusterName = managedClusters[0].Name
	})

	It("add the label to the managed cluster", func() {
		patches := []patch{
			{
				Op:    "add", // or remove
				Path:  "/metadata/labels/" + CLUSTER_LABEL_KEY,
				Value: CLUSTER_LABEL_VALUE,
			},
		}
		updateClusterLabel(httpClient, patches, token, managedClusterName)

		By("Check the label is added")
		Eventually(func() error {
			managedClusters := getManagedCluster(httpClient, token)
			for _, cluster := range managedClusters {
				if val, ok := cluster.Labels[CLUSTER_LABEL_KEY]; ok {
					if val == CLUSTER_LABEL_VALUE {
						return nil
					}
				}
			}
			return fmt.Errorf("the label [%s: %s] is not exist", CLUSTER_LABEL_KEY, CLUSTER_LABEL_VALUE)
		}, 60*time.Second*5, 1*time.Second*5).ShouldNot(HaveOccurred())
	})

	It("remove the label from the maanaged cluster", func() {
		patches := []patch{
			{
				Op:    "remove",
				Path:  "/metadata/labels/" + CLUSTER_LABEL_KEY,
				Value: CLUSTER_LABEL_VALUE,
			},
		}
		updateClusterLabel(httpClient, patches, token, managedClusterName)

		By("Check the label is deleted")
		Eventually(func() error {
			managedClusters := getManagedCluster(httpClient, token)
			for _, cluster := range managedClusters {
				if val, ok := cluster.Labels[CLUSTER_LABEL_KEY]; ok {
					if val == CLUSTER_LABEL_VALUE {
						return fmt.Errorf("the label %s: %s should not be exist", CLUSTER_LABEL_KEY, CLUSTER_LABEL_VALUE)
					}
				}
			}
			return nil
		}, 60*time.Second*5, 1*time.Second*5).ShouldNot(HaveOccurred())
	})
})

type patch struct {
	Op    string `json:"op" binding:"required"`
	Path  string `json:"path" binding:"required"`
	Value string `json:"value"`
}

func getLeafHubName(managedClusterName string) string {
	Expect(managedClusterName).ShouldNot(BeEmpty())
	result := ""
	for _, cluster := range testOptions.ManagedClusters {
		if strings.Compare(cluster.Name, managedClusterName) == 0 {
			result = cluster.LeafHubName
		}
	}
	Expect(result).ShouldNot(BeEmpty())
	return result
}

func getManagedCluster(client *http.Client, token string) []clusterv1.ManagedCluster {
	managedClusterUrl := fmt.Sprintf("https://multicloud-console.apps.%s./multicloud/hub-of-hubs-nonk8s-api/managedclusters", testOptions.HubCluster.BaseDomain)
	req, err := http.NewRequest("GET", managedClusterUrl, nil)
	Expect(err).ShouldNot(HaveOccurred())
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	By("Get response of the api")
	resp, err := client.Do(req)
	Expect(err).ShouldNot(HaveOccurred())
	defer resp.Body.Close()

	By("Parse response to managed cluster")
	body, err := ioutil.ReadAll(resp.Body)
	Expect(err).ShouldNot(HaveOccurred())

	var managedClusters []clusterv1.ManagedCluster
	json.Unmarshal(body, &managedClusters)
	Expect(len(managedClusters)).Should(BeNumerically(">", 1), "should get the managed cluster")

	By("Return parsed managedcluster")
	return managedClusters
}

func updateClusterLabel(client *http.Client, patches []patch, token, managedClusterName string) {
	updateLabelUrl := fmt.Sprintf("https://multicloud-console.apps.%s./multicloud/hub-of-hubs-nonk8s-api/managedclusters/%s", testOptions.HubCluster.BaseDomain, managedClusterName)
	// set method and body
	jsonBody, err := json.Marshal(patches)
	Expect(err).ShouldNot(HaveOccurred())
	req, err := http.NewRequest("PATCH", updateLabelUrl, bytes.NewBuffer(jsonBody))
	Expect(err).ShouldNot(HaveOccurred())

	// add header
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Add("Accept", "application/json")

	// add query
	q := req.URL.Query()
	q.Add("hubCluster", getLeafHubName(managedClusterName))
	req.URL.RawQuery = q.Encode()

	// do request
	response, err := client.Do(req)
	Expect(err).ShouldNot(HaveOccurred())
	defer response.Body.Close()
}
