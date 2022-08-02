package tests

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/klog"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"github.com/stolostron/hub-of-hubs/test/pkg/utils"
)

var _ = Describe("Check the managed cluster from HoH manager", Label("e2e-tests-cluster"), Ordered, func() {
	var token string
	var httpClient *http.Client
	
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
	})

	It("list all the managed cluster", func() {
		managedClusterUrl := fmt.Sprintf("%s/multicloud/hub-of-hubs-nonk8s-api/managedclusters", testOptions.HubCluster.Nonk8sApiServer)
		klog.V(6).Infof("managedClusterUrl: %s", managedClusterUrl)
		req, err := http.NewRequest("GET", managedClusterUrl, nil)
		Expect(err).ShouldNot(HaveOccurred())
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
		
		By("Get response of the api")
		resp, err := httpClient.Do(req)
		Expect(err).ShouldNot(HaveOccurred())
		defer resp.Body.Close()

		By("Parse response to managed cluster")
		body, err := ioutil.ReadAll(resp.Body)
		Expect(err).ShouldNot(HaveOccurred())

		By("Return parsed managedcluster")
		var managedClusters []clusterv1.ManagedCluster
		json.Unmarshal(body, &managedClusters)
		Expect(len(managedClusters)).Should(BeNumerically(">", 0), "should get the managed cluster")

		By("Print managedcluster")
		var out bytes.Buffer
    _ = json.Indent(&out, body, "", "  ")
		klog.V(6).Infof("managedClusters: %s", out.String())
	})
})
