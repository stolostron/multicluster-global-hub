package tests

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
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
			managedClusters, err := getManagedCluster(httpClient, token)
			if err != nil {
				return err
			}
			managedClusterName1 = managedClusters[0].Name
			managedClusterName2 = managedClusters[1].Name
			return nil
		}, 3*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())
	})

	It("add the label to a managedcluster for the policy", func() {
		patches := []patch{
			{
				Op:    "add", // or remove
				Path:  "/metadata/labels/" + POLICY_LABEL_KEY,
				Value: POLICY_LABEL_VALUE,
			},
		}
		Eventually(func() error {
			err := updateClusterLabel(httpClient, patches, token, managedClusterName1)
			if err != nil {
				return err
			}
			return nil
		}, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())

		By("Check the label is added")
		Eventually(func() error {
			managedCluster, err := getManagedClusterByName(httpClient, token, managedClusterName1)
			if err != nil {
				return err
			}
			if val, ok := managedCluster.Labels[POLICY_LABEL_KEY]; ok {
				if val == POLICY_LABEL_VALUE && managedCluster.Name == managedClusterName1 {
					return nil
				}
			}
			return fmt.Errorf("the label %s: %s is not exist", POLICY_LABEL_KEY, POLICY_LABEL_VALUE)
		}, 3*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())
	})

	It("create a inform policy for the labeled cluster", func() {
		Eventually(func() error {
			_, err := clients.Kubectl(clients.HubClusterName(), "apply", "-f", INFORM_POLICY_YAML)
			if err != nil {
				return err
			}
			return nil
		}, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())

		Eventually(func() error {
			status, err := getPoliciesStatus(token, httpClient)
			if err != nil {
				return err
			}
			for _, policyInfo := range status {
				if policyInfo.ClusterName == managedClusterName1 {
					if policyInfo.ErrorInfo == "none" && policyInfo.Compliance == "non_compliant" {
						policyStr, _ := json.MarshalIndent(status, "", "  ")
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
		Eventually(func() error {
			_, err := clients.Kubectl(clients.HubClusterName(), "apply", "-f", ENFORCE_POLICY_YAML)
			if err != nil {
				return err
			}
			return nil
		}, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())

		Eventually(func() error {
			status, err := getPoliciesStatus(token, httpClient)
			if err != nil {
				return err
			}
			for _, policyInfo := range status {
				if policyInfo.ClusterName == managedClusterName1 {
					if policyInfo.ErrorInfo == "none" && policyInfo.Compliance == "compliant" {
						policyStr, _ := json.MarshalIndent(status, "", "  ")
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
		}, 3*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())
	})

	It("add policy to managedcluster2 by adding label", func() {
		patches := []patch{
			{
				Op:    "add", // or remove
				Path:  "/metadata/labels/" + POLICY_LABEL_KEY,
				Value: POLICY_LABEL_VALUE,
			},
		}

		Eventually(func() error {
			err := updateClusterLabel(httpClient, patches, token, managedClusterName2)
			if err != nil {
				return err
			}
			return nil
		}, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())

		By("Check the label is added")
		Eventually(func() error {
			managedCluster, err := getManagedClusterByName(httpClient, token, managedClusterName2)
			if err != nil {
				return err
			}
			if val, ok := managedCluster.Labels[POLICY_LABEL_KEY]; ok {
				if val == POLICY_LABEL_VALUE && managedCluster.Name == managedClusterName2 {
					return nil
				}
			}
			return fmt.Errorf("the label %s: %s is not exist", POLICY_LABEL_KEY, POLICY_LABEL_VALUE)
		}, 3*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())

		By("Check the policy is added")
		Eventually(func() error {
			status, err := getPoliciesStatus(token, httpClient)
			if err != nil {
				return err
			}
			for _, policyInfo := range status {
				if policyInfo.ClusterName == managedClusterName2 {
					if policyInfo.ErrorInfo == "none" && policyInfo.Compliance == "compliant" {
						policyStr, _ := json.MarshalIndent(status, "", "  ")
						klog.V(5).Info(fmt.Printf("add policy by label: %s", string(policyStr)))
						return nil
					} else {
						return fmt.Errorf("the cluster %s with policy error Information %s and compliacne %s",
							managedClusterName2, policyInfo.ErrorInfo, policyInfo.Compliance)
					}
				}
			}
			return fmt.Errorf("the policy have not applied to the managed cluster %s", managedClusterName2)
		}, 3*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())
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
		Eventually(func() error {
			err := updateClusterLabel(httpClient, patches, token, managedClusterName1)
			if err != nil {
				return err
			}
			return nil
		}, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())

		By("Check the policy is removed from the managedclusterName1")
		Eventually(func() error {
			status, err := getPoliciesStatus(token, httpClient)
			if err != nil {
				return err
			}
			for _, policyInfo := range status {
				if policyInfo.ClusterName == managedClusterName1 {
					return fmt.Errorf("the cluster %s policy(%s: %s)should be removed",
						managedClusterName1, policyInfo.ErrorInfo, policyInfo.Compliance)
				}
			}
			policyStr, _ := json.MarshalIndent(status, "", "  ")
			klog.V(5).Info(fmt.Printf("remove policy from managedcluster1: %s", string(policyStr)))
			return nil
		}, 3*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())
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
			status, err := getPoliciesStatus(token, httpClient)
			if err != nil {
				return err
			}
			for _, policyInfo := range status {
				if policyInfo.ClusterName == managedClusterName2 ||
					policyInfo.ClusterName == managedClusterName1 {
					return fmt.Errorf("the cluster %s should delete the policy", policyInfo.ClusterName)
				}
			}
			policyStr, _ := json.MarshalIndent(status, "", "  ")
			klog.V(5).Info(fmt.Printf("delete policy from clusters: %s", string(policyStr)))
			return nil
		}, 3*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())

		By("Delete the label from managedcluster2")
		patches := []patch{
			{
				Op:    "remove",
				Path:  "/metadata/labels/" + POLICY_LABEL_KEY,
				Value: POLICY_LABEL_VALUE,
			},
		}
		Eventually(func() error {
			err := updateClusterLabel(httpClient, patches, token, managedClusterName2)
			if err != nil {
				return err
			}
			return nil
		}, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())
	})
})

type policyStatus struct {
	PolicyId    string `json:"id"`
	ClusterName string `json:"clusterName"`
	LeafHubName string `json:"leafHubName"`
	ErrorInfo   string `json:"errorInfo"`
	Compliance  string `json:"compliance"`
}

func getPoliciesStatus(token string, httpClient *http.Client) ([]policyStatus, error) {
	policyStatusUrl := fmt.Sprintf("%s/multicloud/hub-of-hubs-nonk8s-api/policiesstatus", testOptions.HubCluster.Nonk8sApiServer)
	req, err := http.NewRequest("GET", policyStatusUrl, nil)
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

	var policiesStatus []policyStatus
	err = json.Unmarshal(body, &policiesStatus)
	if err != nil {
		return nil, err
	}
	return policiesStatus, nil
}
