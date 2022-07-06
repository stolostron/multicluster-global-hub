package tests

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/klog"

	"github.com/stolostron/hub-of-hubs/test/pkg/utils"
)

const (
	INFORM_POLICY_YAML  = "../../resources/policy/inform-limitrange-policy.yaml"
	ENFORCE_POLICY_YAML = "../../resources/policy/enforce-limitrange-policy.yaml"

	POLICY_LABEL_KEY   = "policy"
	POLICY_LABEL_VALUE = "test"
)

var _ = Describe("Apply policy to the managed clusters", Ordered, Label("policy"), func() {
	var token string
	var httpClient *http.Client
	var managedClusterName1 string
	var managedClusterName2 string

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
	})

	It("add the label to a managedcluster for the policy", func() {
		patches := []patch{
			{
				Op:    "add", // or remove
				Path:  "/metadata/labels/" + POLICY_LABEL_KEY,
				Value: POLICY_LABEL_VALUE,
			},
		}
		updateClusterLabel(httpClient, patches, token, managedClusterName1)
		By("Check the label is added")
		Eventually(func() error {
			managedClusters := getManagedCluster(httpClient, token)
			for _, cluster := range managedClusters {
				if val, ok := cluster.Labels[POLICY_LABEL_KEY]; ok {
					if val == POLICY_LABEL_VALUE && cluster.Name == managedClusterName1 {
						By("Print the result after adding the label")
						managedClusters := getManagedCluster(httpClient, token)
						printClusterLabel(managedClusters)
						return nil
					}
				}
			}
			err := fmt.Errorf("the label %s: %s is not exist", POLICY_LABEL_KEY, POLICY_LABEL_VALUE)
			return err
		}, 5*60*time.Second, 5*1*time.Second).ShouldNot(HaveOccurred())
	})

	It("create a inform policy for the labeled cluster", func() {
		_, err := clients.Kubectl(clients.HubClusterName(), "apply", "-f", INFORM_POLICY_YAML)
		Expect(err).ShouldNot(HaveOccurred())

		Eventually(func() error {
			transportedPolicies := listTransportedPolicies(token, httpClient)
			for _, policyInfo := range transportedPolicies {
				if policyInfo.ClusterName == managedClusterName1 {
					if policyInfo.ErrorInfo == "none" && policyInfo.Compliance == "non_compliant" {
						policyStr, _ := json.MarshalIndent(transportedPolicies, "", "  ")
						klog.V(5).Info(fmt.Printf("Inform policy[ %s -> %s ]: %s", POLICY_LABEL_KEY, POLICY_LABEL_VALUE, string(policyStr)))
						return nil
					} else {
						err := fmt.Errorf("the cluster %s with [error] 'none' -> %s and [compliance] 'non_compliant' ->  %s ",
							managedClusterName1, policyInfo.ErrorInfo, policyInfo.Compliance)
						klog.V(5).Info(err)
						return err
					}
				}
			}
			return fmt.Errorf("the policy have not applied to the managed cluster %s", managedClusterName1)
		}, 5*60*time.Second, 5*1*time.Second).ShouldNot(HaveOccurred())
	})

	It("enforce the inform policy", func() {
		_, err := clients.Kubectl(clients.HubClusterName(), "apply", "-f", ENFORCE_POLICY_YAML)
		Expect(err).ShouldNot(HaveOccurred())

		Eventually(func() error {
			transportedPolicies := listTransportedPolicies(token, httpClient)
			for _, policyInfo := range transportedPolicies {
				if policyInfo.ClusterName == managedClusterName1 {
					if policyInfo.ErrorInfo == "none" && policyInfo.Compliance == "compliant" {
						policyStr, _ := json.MarshalIndent(transportedPolicies, "", "  ")
						klog.V(5).Info(fmt.Printf("Enforce policy[ %s -> %s ]: %s", POLICY_LABEL_KEY, POLICY_LABEL_VALUE, string(policyStr)))
						return nil
					} else {
						return fmt.Errorf("the cluster %s with policy error Information %s and compliacne %s",
							managedClusterName1, policyInfo.ErrorInfo, policyInfo.Compliance)
					}
				}
			}
			return fmt.Errorf("the policy have not applied to the managed cluster %s", managedClusterName1)
		}, 5*60*time.Second, 5*1*time.Second).ShouldNot(HaveOccurred())
	})

	It("add the label for anothter managedcluster to vertify the placement", func() {
		patches := []patch{
			{
				Op:    "add", // or remove
				Path:  "/metadata/labels/" + POLICY_LABEL_KEY,
				Value: POLICY_LABEL_VALUE,
			},
		}

		updateClusterLabel(httpClient, patches, token, managedClusterName2)

		By("Check the label is added")
		Eventually(func() error {
			managedClusters := getManagedCluster(httpClient, token)
			for _, cluster := range managedClusters {
				if val, ok := cluster.Labels[POLICY_LABEL_KEY]; ok {
					if val == POLICY_LABEL_VALUE && cluster.Name == managedClusterName2 {
						By("Print the result after adding the label")
						managedClusters := getManagedCluster(httpClient, token)
						printClusterLabel(managedClusters)
						return nil
					}
				}
			}
			return fmt.Errorf("the label %s: %s is not exist", POLICY_LABEL_KEY, POLICY_LABEL_VALUE)
		}, 5*60*time.Second, 5*1*time.Second).ShouldNot(HaveOccurred())

		By("Check the policy is added")
		Eventually(func() error {
			transportedPolicies := listTransportedPolicies(token, httpClient)
			for _, policyInfo := range transportedPolicies {
				if policyInfo.ClusterName == managedClusterName2 {
					if policyInfo.ErrorInfo == "none" && policyInfo.Compliance == "compliant" {
						policyStr, _ := json.MarshalIndent(transportedPolicies, "", "  ")
						klog.V(5).Info(fmt.Printf("scale the enforced policy[ %s -> %s ]: %s", POLICY_LABEL_KEY, POLICY_LABEL_VALUE, string(policyStr)))
						return nil
					} else {
						return fmt.Errorf("the cluster %s with policy error Information %s and compliacne %s",
							managedClusterName2, policyInfo.ErrorInfo, policyInfo.Compliance)
					}
				}
			}
			return fmt.Errorf("the policy have not applied to the managed cluster %s", managedClusterName2)
		}, 5*60*time.Second, 5*1*time.Second).ShouldNot(HaveOccurred())
	})

	AfterAll(func() {
		_, err := clients.Kubectl(clients.HubClusterName(), "delete", "-f", ENFORCE_POLICY_YAML)
		Expect(err).ShouldNot(HaveOccurred())

		deleteInfo, err := clients.Kubectl(managedClusterName1, "delete", "LimitRange", "container-mem-limit-range")
		Expect(err).ShouldNot(HaveOccurred())
		klog.V(5).Info(managedClusterName1, ": ", deleteInfo)

		deleteInfo, err = clients.Kubectl(managedClusterName2, "delete", "LimitRange", "container-mem-limit-range")
		Expect(err).ShouldNot(HaveOccurred())
		klog.V(5).Info(managedClusterName2, ": ", deleteInfo)

		patches := []patch{
			{
				Op:    "remove",
				Path:  "/metadata/labels/" + POLICY_LABEL_KEY,
				Value: POLICY_LABEL_VALUE,
			},
		}
		updateClusterLabel(httpClient, patches, token, managedClusterName1)
		updateClusterLabel(httpClient, patches, token, managedClusterName2)

		By("Check the label is removed from the clusters")
		Eventually(func() error {
			managedClusters := getManagedCluster(httpClient, token)
			for _, cluster := range managedClusters {
				if val, ok := cluster.Labels[POLICY_LABEL_KEY]; ok {
					if val == POLICY_LABEL_VALUE {
						return fmt.Errorf("the label %s: %s has't removed from the cluster %s", POLICY_LABEL_KEY, POLICY_LABEL_VALUE, cluster.Name)
					}
				}
			}
			return nil
		}, 5*60*time.Second, 5*1*time.Second).ShouldNot(HaveOccurred())
	})
})

type policyStatus struct {
	PolicyId    string `json:"id"`
	ClusterName string `json:"clusterName"`
	LeafHubName string `json:"leafHubName"`
	ErrorInfo   string `json:"errorInfo"`
	Compliance  string `json:"compliance"`
}

func listTransportedPolicies(token string, httpClient *http.Client) []policyStatus {
	policyStatusUrl := fmt.Sprintf("%s/multicloud/hub-of-hubs-nonk8s-api/policiesstatus", testOptions.HubCluster.Nonk8sApiServer)
	req, err := http.NewRequest("GET", policyStatusUrl, nil)
	Expect(err).ShouldNot(HaveOccurred())
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	By("Get response of the api")
	resp, err := httpClient.Do(req)
	Expect(err).ShouldNot(HaveOccurred())
	defer resp.Body.Close()

	By("Parse response to managed cluster")
	body, err := ioutil.ReadAll(resp.Body)
	Expect(err).ShouldNot(HaveOccurred())

	var policiesStatus []policyStatus
	err = json.Unmarshal(body, &policiesStatus)
	Expect(err).ShouldNot(HaveOccurred())
	return policiesStatus
}
