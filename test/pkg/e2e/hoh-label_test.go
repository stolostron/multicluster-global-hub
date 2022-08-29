package tests

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"github.com/stolostron/multicluster-global-hub/test/pkg/utils"
)

const (
	CLUSTER_LABEL_KEY   = "label"
	CLUSTER_LABEL_VALUE = "test"
)

var _ = Describe("Updating cluster label from HoH manager", Label("e2e-tests-label"), Ordered, func() {
	var token string
	var httpClient *http.Client
	var managedClusterName string

	BeforeAll(func() {
		By("Get token for the non-k8s-api")
		var err error
		token, err = utils.FetchBearerToken(testOptions)
		Expect(err).ShouldNot(HaveOccurred())

		By("Config request of the api")
		transport := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		httpClient = &http.Client{Timeout: time.Second * 10, Transport: transport}
		Eventually(func() error {
			managedClusters, err := getManagedCluster(httpClient, token)
			if err != nil {
				return err
			}
			managedClusterName = managedClusters[0].Name
			return nil
		}, 3*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())
	})

	It("add the label to the managed cluster", func() {
		patches := []patch{
			{
				Op:    "add", // or remove
				Path:  "/metadata/labels/" + CLUSTER_LABEL_KEY,
				Value: CLUSTER_LABEL_VALUE,
			},
		}

		Eventually(func() error {
			err := updateClusterLabel(httpClient, patches, token, managedClusterName)
			if err != nil {
				return err
			}
			return nil
		}, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())

		By("Check the label is added")
		Eventually(func() error {
			managedCluster, err := getManagedClusterByName(httpClient, token, managedClusterName)
			if err != nil {
				return err
			}
			if val, ok := managedCluster.Labels[CLUSTER_LABEL_KEY]; ok {
				if val == CLUSTER_LABEL_VALUE {
					return nil
				}
			}
			return fmt.Errorf("the label [%s: %s] is not exist", CLUSTER_LABEL_KEY, CLUSTER_LABEL_VALUE)
		}, 3*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())
	})

	It("remove the label from the managed cluster", func() {
		patches := []patch{
			{
				Op:    "remove",
				Path:  "/metadata/labels/" + CLUSTER_LABEL_KEY,
				Value: CLUSTER_LABEL_VALUE,
			},
		}
		Eventually(func() error {
			err := updateClusterLabel(httpClient, patches, token, managedClusterName)
			if err != nil {
				return err
			}
			return nil
		}, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())

		By("Check the label is deleted")
		Eventually(func() error {
			managedCluster, err := getManagedClusterByName(httpClient, token, managedClusterName)
			if err != nil {
				return err
			}

			if val, ok := managedCluster.Labels[CLUSTER_LABEL_KEY]; ok {
				if val == CLUSTER_LABEL_VALUE {
					return fmt.Errorf("the label %s: %s should not be exist", CLUSTER_LABEL_KEY, CLUSTER_LABEL_VALUE)
				}
			}
			return nil
		}, 3*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())
	})
})

type patch struct {
	Op    string `json:"op" binding:"required"`
	Path  string `json:"path" binding:"required"`
	Value string `json:"value"`
}

func getLeafHubName(managedClusterName string) string {
	result := ""
	for _, cluster := range testOptions.ManagedClusters {
		if strings.Compare(cluster.Name, managedClusterName) == 0 {
			result = cluster.LeafHubName
		}
	}
	return result
}

func getManagedCluster(client *http.Client, token string) ([]clusterv1.ManagedCluster, error) {
	managedClusterUrl := fmt.Sprintf("%s/multicloud/hub-of-hubs-nonk8s-api/managedclusters", testOptions.HubCluster.Nonk8sApiServer)
	req, err := http.NewRequest("GET", managedClusterUrl, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var managedClusters []clusterv1.ManagedCluster
	err = json.Unmarshal(body, &managedClusters)
	if err != nil {
		return nil, err
	}
	if len(managedClusters) != 2 {
		return nil, fmt.Errorf("cannot get two managed clusters")
	}

	return managedClusters, nil
}

func getManagedClusterByName(client *http.Client, token, managedClusterName string) (
	*clusterv1.ManagedCluster, error,
) {
	managedClusterUrl := fmt.Sprintf("%s/multicloud/hub-of-hubs-nonk8s-api/managedclusters/%s",
		testOptions.HubCluster.Nonk8sApiServer, managedClusterName)
	req, err := http.NewRequest("GET", managedClusterUrl, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var managedCluster *clusterv1.ManagedCluster
	err = json.Unmarshal(body, &managedCluster)
	if err != nil {
		return nil, err
	}

	return managedCluster, nil
}

func updateClusterLabel(client *http.Client, patches []patch, token, managedClusterName string) error {
	updateLabelUrl := fmt.Sprintf("%s/multicloud/hub-of-hubs-nonk8s-api/managedclusters/%s",
		testOptions.HubCluster.Nonk8sApiServer, managedClusterName)
	// set method and body
	jsonBody, err := json.Marshal(patches)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("PATCH", updateLabelUrl, bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}

	// add header
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Add("Accept", "application/json")

	// add query
	q := req.URL.Query()
	q.Add("hubCluster", getLeafHubName(managedClusterName))
	req.URL.RawQuery = q.Encode()

	// do request
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	return nil
}
