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
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

const (
	INFORM_POLICY_YAML  = "../../resources/policy/inform-limitrange-policy.yaml"
	ENFORCE_POLICY_YAML = "../../resources/policy/enforce-limitrange-policy.yaml"

	POLICY_LABEL_KEY      = "global-policy"
	POLICY_LABEL_VALUE    = "test"
	POLICY_NAME           = "policy-limitrange"
	POLICY_NAMESPACE      = "default"
	PLACEMENTBINDING_NAME = "binding-policy-limitrange"
	PLACEMENT_RULE_NAME   = "placementrule-policy-limitrange"
)

var _ = Describe("Apply policy to the managed clusters", Ordered, Label("e2e-tests-policy"), func() {
	var httpClient *http.Client
	var managedClusters []clusterv1.ManagedCluster
	var globalClient client.Client
	var regionalClient client.Client
	var regionalClients []client.Client
	var err error

	BeforeAll(func() {
		Eventually(func() error {
			By("Config request of the api")
			transport := &http.Transport{
					TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			}
			httpClient = &http.Client{Timeout: time.Second * 20, Transport: transport}
			managedClusters, err = getManagedCluster(httpClient, httpToken)
			if err != nil {
				return err
			}
			if len(managedClusters) == 0 {
				return fmt.Errorf("managed cluster is not exist")
			}
			return nil
		}, 3*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())

		By("Get the appsubreport client")
		scheme := runtime.NewScheme()
		policiesv1.AddToScheme(scheme)
		corev1.AddToScheme(scheme)
		placementrulev1.AddToScheme(scheme)
		globalClient, err = clients.ControllerRuntimeClient(clients.HubClusterName(), scheme)
		Expect(err).ShouldNot(HaveOccurred())
		for _, leafhubName := range clients.GetLeafHubClusterNames(){
			regionalClient, err = clients.ControllerRuntimeClient(leafhubName, scheme)
			regionalClients = append(regionalClients, regionalClient)
		}
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

		By("Check the label is added")
		Eventually(func() error {
			err := updateClusterLabel(httpClient, patches, httpToken, string(managedClusters[0].UID))
			if err != nil {
				return err
			}
			managedClusterInfo, err := getManagedClusterByName(httpClient, httpToken, managedClusters[0].Name)
			if err != nil {
				return err
			}
			if val, ok := managedClusterInfo.Labels[POLICY_LABEL_KEY]; ok {
				if val == POLICY_LABEL_VALUE && managedClusterInfo.Name == managedClusters[0].Name {
					return nil
				}
			}
			return fmt.Errorf("the label %s: %s is not exist", POLICY_LABEL_KEY, POLICY_LABEL_VALUE)
		}, 3*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())
	})

	It("create a inform policy for the labeled cluster", func() {
		By("Create the inform policy in global hub")
		Eventually(func() error {
			message, err := clients.Kubectl(clients.HubClusterName(), "apply", "-f", INFORM_POLICY_YAML)
			if err != nil {
				klog.V(5).Info(fmt.Sprintf("apply inform policy error: %s", message))
				return err
			}
			return nil
		}, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())

		By("Check the inform policy in global hub")
		Eventually(func() error {
			status, err := getPolicyStatus(globalClient, httpClient, POLICY_NAME, POLICY_NAMESPACE, httpToken)
			if err != nil {
				return err
			}
			for _, policyInfo := range status.Status {
				if policyInfo.ClusterName == managedClusters[0].Name {
					if policyInfo.ComplianceState == policiesv1.NonCompliant {
						return nil
					}
				}
			}
			return fmt.Errorf("the policy have not applied to the managed cluster %s", managedClusters[0].Name)
		}, 3*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())

		By("Check the inform policy in regional hub")
		Eventually(func() error {
			status, err := getRegionalPolicyStatus(regionalClients[0], POLICY_NAME, POLICY_NAMESPACE)
			if err != nil {
				return err
			}

			policyStatusStr, _ := json.MarshalIndent(status, "", "  ")
			klog.V(5).Info(fmt.Sprintf("get policy status: %s", policyStatusStr))

			for _, policyInfo := range status.Status {
				if policyInfo.ClusterName == managedClusters[0].Name {
					if policyInfo.ComplianceState == policiesv1.NonCompliant {
						return nil
					}
				}
			}
			return fmt.Errorf("the policy have not applied to the managed cluster %s", managedClusters[0].Name)
		}, 3*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())
	})

	It("add the label to a managedcluster for the policy", func() {
		for i:=1; i<len(managedClusters); i++ {
			patches := []patch{
				{
					Op:    "add", // or remove
					Path:  "/metadata/labels/" + POLICY_LABEL_KEY,
					Value: POLICY_LABEL_VALUE,
				},
			}

			By("Check the label is added")
			Eventually(func() error {
				err := updateClusterLabel(httpClient, patches, httpToken, string(managedClusters[i].UID))
				if err != nil {
					return err
				}
				managedClusterInfo, err := getManagedClusterByName(httpClient, httpToken, managedClusters[i].Name)
				if err != nil {
					return err
				}
				if val, ok := managedClusterInfo.Labels[POLICY_LABEL_KEY]; ok {
					if val == POLICY_LABEL_VALUE && managedClusterInfo.Name == managedClusters[i].Name {
						return nil
					}
				}
				return fmt.Errorf("the label %s: %s is not exist", POLICY_LABEL_KEY, POLICY_LABEL_VALUE)
			}, 3*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())
		}
	})

	It("create a inform policy for the labeled cluster", func() {
		By("Create the inform policy in global hub")
		Eventually(func() error {
			message, err := clients.Kubectl(clients.HubClusterName(), "apply", "-f", INFORM_POLICY_YAML)
			if err != nil {
				klog.V(5).Info(fmt.Sprintf("apply inform policy error: %s", message))
				return err
			}
			return nil
		}, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())

		By("Check the inform policy in global hub")
		Eventually(func() error {
			status, err := getPolicyStatus(globalClient, httpClient, POLICY_NAME, POLICY_NAMESPACE, httpToken)
			if err != nil {
				return err
			}
			for _, managedCluster := range managedClusters {
				var foundNonCompliantPolicy bool
				for _, policyInfo := range status.Status {
					if managedCluster.Name == policyInfo.ClusterName && policyInfo.ComplianceState == policiesv1.NonCompliant {
						foundNonCompliantPolicy = true
						break
					}
				}
				if !foundNonCompliantPolicy {
					return fmt.Errorf("the policy has not been applied to the managed cluster %s or it is already compliant", managedCluster.Name)
				}
			}
			return nil
		}, 3*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())

		By("Check the inform policy in regional hub2")
		Eventually(func() error {
			status, err := getRegionalPolicyStatus(regionalClients[1], POLICY_NAME, POLICY_NAMESPACE)
			if err != nil {
				return err
			}

			policyStatusStr, _ := json.MarshalIndent(status, "", "  ")
			klog.V(5).Info(fmt.Sprintf("get policy status: %s", policyStatusStr))
			
			for _, policyInfo := range status.Status {
				if policyInfo.ClusterName == managedClusters[1].Name {
					if policyInfo.ComplianceState == policiesv1.NonCompliant {
						return nil
					}
				}
			}
			return fmt.Errorf("the policy have not applied to the managed cluster %s", managedClusters[1].Name)
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
			status, err := getPolicyStatus(globalClient, httpClient, POLICY_NAME, POLICY_NAMESPACE, httpToken)
			if err != nil {
				return err
			}
			for _, managedCluster := range managedClusters {
				var foundNonCompliantPolicy bool
				for _, policyInfo := range status.Status {
					if policyInfo.ClusterName == managedCluster.Name && policyInfo.ComplianceState == policiesv1.Compliant{
						foundNonCompliantPolicy = true
						break
					}
				}
				if !foundNonCompliantPolicy {
					return fmt.Errorf("the policy has not been applied to the managed cluster %s or it is already compliant", managedCluster.Name)
				}
			}
			return nil
		}, 3*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())
	})

	It("remove managedcluster policy by deleting label", func() {
		for _, managedCluster := range managedClusters {
			By("Check the policy is created in managedcluster")
			Eventually(func() error {
				status, err := getPolicyStatus(globalClient, httpClient, POLICY_NAME, POLICY_NAMESPACE, httpToken)
				if err != nil {
					return err
				}
				for _, policyInfo := range status.Status {
					if policyInfo.ClusterName == managedCluster.Name {
						return nil
					}
				}
				return fmt.Errorf("the policy should be in the managedcluster")
			}, 3*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())

			By("remove the label from the managedcluster")
			patches := []patch{
				{
					Op:    "remove",
					Path:  "/metadata/labels/" + POLICY_LABEL_KEY,
					Value: POLICY_LABEL_VALUE,
				},
			}

			Eventually(func() error {
				err := updateClusterLabel(httpClient, patches, httpToken, string(managedCluster.UID))
				if err != nil {
					return err
				}
				return nil
			}, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())

			By("Check the policy is removed from the managedcluster")
			Eventually(func() error {
				status, err := getPolicyStatus(globalClient, httpClient, POLICY_NAME, POLICY_NAMESPACE, httpToken)
				if err != nil {
					return err
				}
				for _, policyInfo := range status.Status {
					if policyInfo.ClusterName == managedCluster.Name {
						return fmt.Errorf("the cluster %s policy(%s)should be removed", managedCluster.Name, POLICY_NAME)
					}
				}
				return nil
			}, 3*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())
		}	
	})

	It("verify the policy resource has been added the global cleanup finalizer", func() {
		By("Verify the policy has been added the global hub cleanup finalizer")
		Eventually(func() error {
			policy := &policiesv1.Policy{}
			err := globalClient.Get(context.TODO(), client.ObjectKey{
				Namespace: POLICY_NAMESPACE,
				Name:      POLICY_NAME,
			}, policy)
			if err != nil {
				return err
			}
			for _, finalizer := range policy.Finalizers {
				if finalizer == constants.GlobalHubCleanupFinalizer {
					return nil
				}
			}
			return fmt.Errorf("the policy(%s) hasn't been added the cleanup finalizer", policy.GetName())
		}, 1*time.Minute, 1*time.Second).Should(Succeed())

		By("Verify the placementbinding has been added the global hub cleanup finalizer")
		Eventually(func() error {
			placementbinding := &policiesv1.PlacementBinding{}
			err := globalClient.Get(context.TODO(), client.ObjectKey{
				Namespace: POLICY_NAMESPACE,
				Name:      PLACEMENTBINDING_NAME,
			}, placementbinding)
			if err != nil {
				return err
			}
			for _, finalizer := range placementbinding.Finalizers {
				if finalizer == constants.GlobalHubCleanupFinalizer {
					return nil
				}
			}
			return fmt.Errorf("the placementbinding(%s) hasn't been added the cleanup finalizer", placementbinding.GetName())
		}, 1*time.Minute, 1*time.Second).Should(Succeed())

		By("Verify the local placementrule has been added the global hub cleanup finalizer")
		Eventually(func() error {
			placementrule := &placementrulev1.PlacementRule{}
			err := globalClient.Get(context.TODO(), client.ObjectKey{
				Namespace: POLICY_NAMESPACE,
				Name:      PLACEMENT_RULE_NAME,
			}, placementrule)
			if err != nil {
				return err
			}
			for _, finalizer := range placementrule.Finalizers {
				if finalizer == constants.GlobalHubCleanupFinalizer {
					return nil
				}
			}
			return fmt.Errorf("the placementrule(%s) hasn't been added the cleanup finalizer", placementrule.GetName())
		}, 1*time.Minute, 1*time.Second).Should(Succeed())
	})

	AfterAll(func() {
		By("Delete the enforce policy from global hub")
		_, err := clients.Kubectl(clients.HubClusterName(), "delete", "-f", ENFORCE_POLICY_YAML)
		Expect(err).ShouldNot(HaveOccurred())

		By("Check the enforce policy is deleted from regional hub")
		Eventually(func() error {
			for i, regionalClient := range regionalClients {
				_, err := getRegionalPolicyStatus(regionalClient, POLICY_NAME, POLICY_NAMESPACE)
				if errors.IsNotFound(err) {
					return nil
				}
				if err != nil {
					return err
				}
				return fmt.Errorf("the policy should be removed from regional hub%d", i)
			}
			return nil
		}, 3*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())
		
		By("Delete the LimitRange CR from managedcluster")
		for _, managedCluster := range managedClusters{
			deleteInfo, err := clients.Kubectl(managedCluster.Name, "delete", "LimitRange", "container-mem-limit-range")
			Expect(err).ShouldNot(HaveOccurred())
			klog.V(5).Info(managedCluster, ": ", deleteInfo)
		}
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

	policyUID := string(policy.UID)
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
