package tests

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/test/pkg/utils"
)

const (
	INFORM_POLICY_YAML  = "../../resources/policy/inform-limitrange-policy.yaml"
	ENFORCE_POLICY_YAML = "../../resources/policy/enforce-limitrange-policy.yaml"

	POLICY_LABEL_KEY   = "global-policy"
	POLICY_LABEL_VALUE = "test"
	POLICY_NAME        = "policy-limitrange"
	POLICY_NAMESPACE   = "default"
)

var _ = Describe("Apply policy to the managed clusters", Ordered, Label("e2e-tests-policy"), func() {
	var token string
	var httpClient *http.Client
	var managedClusterName1 string
	var managedClusterName2 string
	var managedClusterUID1 string
	var managedClusterUID2 string
	var globalClient client.Client
	var regionalClient client.Client

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
			managedClusterUID1 = string(managedClusters[0].GetUID())
			managedClusterUID2 = string(managedClusters[1].GetUID())
			return nil
		}, 3*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())

		By("Get the appsubreport client")
		scheme := runtime.NewScheme()
		policiesv1.AddToScheme(scheme)
		corev1.AddToScheme(scheme)
		globalClient, err = clients.ControllerRuntimeClient(clients.HubClusterName(), scheme)
		Expect(err).ShouldNot(HaveOccurred())
		regionalClient, err = clients.ControllerRuntimeClient((clients.LeafHubClusterName()), scheme)
		Expect(err).ShouldNot(HaveOccurred())
	})

	It("add the label to a managedcluster1 for the policy", func() {
		patches := []patch{
			{
				Op:    "add", // or remove
				Path:  "/metadata/labels/" + POLICY_LABEL_KEY,
				Value: POLICY_LABEL_VALUE,
			},
		}
		Eventually(func() error {
			err := updateClusterLabel(httpClient, patches, token, managedClusterUID1)
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
		By("Create the inform policy in global hub")
		Eventually(func() error {
			_, err := clients.Kubectl(clients.HubClusterName(), "apply", "-f", INFORM_POLICY_YAML)
			if err != nil {
				return err
			}
			return nil
		}, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())

		By("Check the inform policy in global hub")
		Eventually(func() error {
			status, err := getPolicyStatus(globalClient, httpClient, POLICY_NAME, POLICY_NAMESPACE, token)
			if err != nil {
				return err
			}
			for _, policyInfo := range status.Status {
				if policyInfo.ClusterName == managedClusterName1 {
					if policyInfo.ComplianceState == policiesv1.NonCompliant {
						return nil
					}
				}
			}
			return fmt.Errorf("the policy have not applied to the managed cluster %s", managedClusterName1)
		}, 3*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())

		By("Check the inform policy in regional hub")
		Eventually(func() error {
			status, err := getRegionalPolicyStatus(regionalClient, POLICY_NAME, POLICY_NAMESPACE)
			if err != nil {
				return err
			}

			policyStatusStr, _ := json.MarshalIndent(status, "", "  ")
			klog.V(5).Info(fmt.Sprintf("get policy status: %s", policyStatusStr))

			for _, policyInfo := range status.Status {
				if policyInfo.ClusterName == managedClusterName1 {
					if policyInfo.ComplianceState == policiesv1.NonCompliant {
						return nil
					}
				}
			}
			return fmt.Errorf("the policy have not applied to the managed cluster %s", managedClusterName1)
		}, 3*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())
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
			status, err := getPolicyStatus(globalClient, httpClient, POLICY_NAME, POLICY_NAMESPACE, token)
			if err != nil {
				return err
			}
			for _, policyInfo := range status.Status {
				if policyInfo.ClusterName == managedClusterName1 {
					if policyInfo.ComplianceState == policiesv1.Compliant {
						return nil
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
			err := updateClusterLabel(httpClient, patches, token, managedClusterUID2)
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

		By("Check the policy is created in global hub")
		Eventually(func() error {
			status, err := getPolicyStatus(globalClient, httpClient, POLICY_NAME, POLICY_NAMESPACE, token)
			if err != nil {
				return err
			}
			for _, policyInfo := range status.Status {
				if policyInfo.ClusterName == managedClusterName2 {
					if policyInfo.ComplianceState == policiesv1.Compliant {
						return nil
					}
				}
			}
			return fmt.Errorf("the policy have not applied to the managed cluster %s", managedClusterName2)
		}, 3*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())

		By("Check the policy is created in regional hub")
		Eventually(func() error {
			status, err := getRegionalPolicyStatus(regionalClient, POLICY_NAME, POLICY_NAMESPACE)
			if err != nil {
				return err
			}
			for _, policyInfo := range status.Status {
				if policyInfo.ClusterName == managedClusterName2 {
					if policyInfo.ComplianceState == policiesv1.Compliant {
						return nil
					}
				}
			}
			return fmt.Errorf("the policy have not applied to the managed cluster %s", managedClusterName2)
		}, 3*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())
	})

	It("remove managedcluster1 policy by deleting label", func() {
		By("Check the policy is created in managedcluster1")
		Eventually(func() error {
			status, err := getPolicyStatus(globalClient, httpClient, POLICY_NAME, POLICY_NAMESPACE, token)
			if err != nil {
				return err
			}
			for _, policyInfo := range status.Status {
				if policyInfo.ClusterName == managedClusterName1 {
					return nil
				}
			}
			return fmt.Errorf("the policy should be in the managedcluster1")
		}, 3*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())

		By("remove the label from the managedcluster1")
		patches := []patch{
			{
				Op:    "remove",
				Path:  "/metadata/labels/" + POLICY_LABEL_KEY,
				Value: POLICY_LABEL_VALUE,
			},
		}
		Eventually(func() error {
			err := updateClusterLabel(httpClient, patches, token, managedClusterUID1)
			if err != nil {
				return err
			}
			return nil
		}, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())

		By("Check the policy is removed from the managedcluster1")
		Eventually(func() error {
			status, err := getPolicyStatus(globalClient, httpClient, POLICY_NAME, POLICY_NAMESPACE, token)
			if err != nil {
				return err
			}
			for _, policyInfo := range status.Status {
				if policyInfo.ClusterName == managedClusterName1 {
					return fmt.Errorf("the cluster %s policy(%s)should be removed", managedClusterName1, POLICY_NAME)
				}
			}
			return nil
		}, 3*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())
	})

	AfterAll(func() {
		By("Delete the enforce policy from global hub")
		_, err := clients.Kubectl(clients.HubClusterName(), "delete", "-f", ENFORCE_POLICY_YAML)
		Expect(err).ShouldNot(HaveOccurred())

		By("Check the enforce policy is deleted from regional hub")
		Eventually(func() error {
			_, err := getRegionalPolicyStatus(regionalClient, POLICY_NAME, POLICY_NAMESPACE)
			if errors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			return fmt.Errorf("the policy should be removed from regional hub")
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
			err := updateClusterLabel(httpClient, patches, token, managedClusterUID2)
			if err != nil {
				return err
			}
			return nil
		}, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())

		By("Delete the LimitRange CR from managedcluster1 and managedcluster2")
		deleteInfo, err := clients.Kubectl(managedClusterName1, "delete", "LimitRange", "container-mem-limit-range")
		Expect(err).ShouldNot(HaveOccurred())
		klog.V(5).Info(managedClusterName1, ": ", deleteInfo)

		deleteInfo, err = clients.Kubectl(managedClusterName2, "delete", "LimitRange", "container-mem-limit-range")
		Expect(err).ShouldNot(HaveOccurred())
		klog.V(5).Info(managedClusterName2, ": ", deleteInfo)
	})
})

func getPolicyStatus(client client.Client, httpClient *http.Client, name, namespace,
	token string,
) (*policiesv1.PolicyStatus, error) {
	policy := &policiesv1.Policy{}
	err := client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: name}, policy)
	if err != nil {
		return nil, err
	}

	policyUID := string(policy.GetUID())
	getPolicyStatusURL := fmt.Sprintf("%s/global-hub-api/v1/policy/%s/status",
		testOptions.HubCluster.Nonk8sApiServer, policyUID)
	req, err := http.NewRequest("GET", getPolicyStatusURL, nil)
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

	klog.V(5).Info(fmt.Sprintf("get policy status reponse body: \n%s\n", body))

	err = json.Unmarshal(body, policy)
	if err != nil {
		return nil, err
	}

	return &policy.Status, nil
}

func getRegionalPolicyStatus(client client.Client, name, namespace string) (*policiesv1.PolicyStatus, error) {
	policy := &policiesv1.Policy{}
	err := client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: name}, policy)
	if err != nil {
		return nil, err
	}

	return &policy.Status, nil
}
