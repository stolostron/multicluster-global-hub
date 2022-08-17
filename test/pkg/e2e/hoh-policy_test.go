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

	"github.com/stolostron/multicluster-global-hub/test/pkg/utils"
)

const (
	INFORM_POLICY_YAML  = "../../resources/policy/inform-limitrange-policy.yaml"
	ENFORCE_POLICY_YAML = "../../resources/policy/enforce-limitrange-policy.yaml"

	POLICY_LABEL_KEY   = "global-policy"
	POLICY_LABEL_VALUE = "test"
)

var _ = Describe("Apply policy to the managed clusters", Ordered, Label("e2e-tests-policy"), func() {
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

		By("Get managed cluster name")
		Eventually(func() error {
			managedClusters := getManagedCluster(httpClient, token)
			if len(managedClusters) <= 1 {
				return fmt.Errorf("wrong number of managed cluster, should be %d, but found %d", 2, len(managedClusters))
			}
			managedClusterName1 = managedClusters[0].Name
			managedClusterName2 = managedClusters[1].Name
			return nil
		}, 5*60*time.Second, 5*1*time.Second).ShouldNot(HaveOccurred())
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
						klog.V(5).Info(fmt.Printf("Inform policy[ %s -> %s ]: %s",
							POLICY_LABEL_KEY, POLICY_LABEL_VALUE, string(policyStr)))
						return nil
					} else {
						return fmt.Errorf("the cluster %s with policyInfo %s and compliance %s ",
							managedClusterName1, policyInfo.ErrorInfo, policyInfo.Compliance)
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
						klog.V(5).Info(fmt.Printf("Enforce policy[ %s -> %s ]: %s",
							POLICY_LABEL_KEY, POLICY_LABEL_VALUE, string(policyStr)))
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

	It("add policy to managedcluster2 by adding label", func() {
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
						klog.V(5).Info(fmt.Printf("add policy by label: %s", string(policyStr)))
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

	It("remove managedcluster1 policy by deleting label", func() {
		By("remove the label from the managedclusterName1")
		patches := []patch{
			{
				Op:    "remove",
				Path:  "/metadata/labels/" + POLICY_LABEL_KEY,
				Value: POLICY_LABEL_VALUE,
			},
		}
		updateClusterLabel(httpClient, patches, token, managedClusterName1)

		By("Check the label is removed from the managedclusterName1")
		Eventually(func() error {
			managedClusters := getManagedCluster(httpClient, token)
			for _, cluster := range managedClusters {
				if cluster.Name == managedClusterName1 {
					if val, ok := cluster.Labels[POLICY_LABEL_KEY]; ok && val == POLICY_LABEL_VALUE {
						return fmt.Errorf("the label %s: %s has't removed from the cluster %s",
							POLICY_LABEL_KEY, POLICY_LABEL_VALUE, cluster.Name)
					}
				}
			}
			printClusterLabel(managedClusters)
			return nil
		}, 5*60*time.Second, 5*1*time.Second).ShouldNot(HaveOccurred())

		By("Check the policy is removed from the managedclusterName1")
		Eventually(func() error {
			transportedPolicies := listTransportedPolicies(token, httpClient)
			for _, policyInfo := range transportedPolicies {
				if policyInfo.ClusterName == managedClusterName1 {
					return fmt.Errorf("the cluster %s policy(%s: %s)should be removed",
						managedClusterName1, policyInfo.ErrorInfo, policyInfo.Compliance)
				}
			}
			policyStr, _ := json.MarshalIndent(transportedPolicies, "", "  ")
			klog.V(5).Info(fmt.Printf("remove policy from managedcluster1: %s", string(policyStr)))
			return nil
		}, 5*60*time.Second, 5*1*time.Second).ShouldNot(HaveOccurred())
	})

	AfterAll(func() {
		By("Delete the enforced policy")
		_, err := clients.Kubectl(clients.HubClusterName(), "delete", "-f", ENFORCE_POLICY_YAML)
		Expect(err).ShouldNot(HaveOccurred())

		By("Delete the LimitRange CR from managedcluster1 and managedcluster2")
		deleteInfo, err := clients.Kubectl(managedClusterName1, "delete", "LimitRange", "container-mem-limit-range")
		Expect(err).ShouldNot(HaveOccurred())
		klog.V(5).Info(managedClusterName1, ": ", deleteInfo)

		deleteInfo, err = clients.Kubectl(managedClusterName2, "delete", "LimitRange", "container-mem-limit-range")
		Expect(err).ShouldNot(HaveOccurred())
		klog.V(5).Info(managedClusterName2, ": ", deleteInfo)

		By("Check the policy is deleted from managedcluster1 and managedcluster2")
		Eventually(func() error {
			transportedPolicies := listTransportedPolicies(token, httpClient)
			for _, policyInfo := range transportedPolicies {
				if policyInfo.ClusterName == managedClusterName2 ||
					policyInfo.ClusterName == managedClusterName1 {
					return fmt.Errorf("the cluster %s should delete the policy", policyInfo.ClusterName)
				}
			}
			policyStr, _ := json.MarshalIndent(transportedPolicies, "", "  ")
			klog.V(5).Info(fmt.Printf("delete policy from clusters: %s", string(policyStr)))
			return nil
		}, 5*60*time.Second, 5*1*time.Second).ShouldNot(HaveOccurred())

		By("Delete the label from managedcluster2")
		patches := []patch{
			{
				Op:    "remove",
				Path:  "/metadata/labels/" + POLICY_LABEL_KEY,
				Value: POLICY_LABEL_VALUE,
			},
		}
		updateClusterLabel(httpClient, patches, token, managedClusterName2)

		By("Check the label is removed from the managedclusterName2")
		Eventually(func() error {
			managedClusters := getManagedCluster(httpClient, token)
			for _, cluster := range managedClusters {
				if cluster.Name == managedClusterName2 {
					if val, ok := cluster.Labels[POLICY_LABEL_KEY]; ok && val == POLICY_LABEL_VALUE {
						return fmt.Errorf("the label %s: %s has't removed from the cluster %s",
							POLICY_LABEL_KEY, POLICY_LABEL_VALUE, cluster.Name)
					}
				}
			}
			printClusterLabel(managedClusters)
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
