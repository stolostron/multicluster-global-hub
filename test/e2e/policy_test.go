package tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	INFORM_POLICY_YAML  = "./../manifest/policy/inform-limitrange-policy.yaml"
	ENFORCE_POLICY_YAML = "./../manifest/policy/enforce-limitrange-policy.yaml"

	POLICY_LABEL_KEY      = "global-policy"
	POLICY_LABEL_VALUE    = "test"
	POLICY_NAME           = "policy-limitrange"
	POLICY_NAMESPACE      = "default"
	PLACEMENTBINDING_NAME = "binding-policy-limitrange"
	PLACEMENT_RULE_NAME   = "placementrule-policy-limitrange"
)

var _ = Describe("Apply policy to the managed clusters", Ordered, Label("e2e-test-policy"), func() {
	BeforeAll(func() {
		By("Create the inform policy in global hub")
		Eventually(func() error {
			message, err := testClients.Kubectl(testOptions.GlobalHub.Name, "apply", "-f", INFORM_POLICY_YAML)
			if err != nil {
				klog.V(5).Info(fmt.Sprintf("apply inform policy error: %s", message))
				return err
			}
			return nil
		}, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())
	})

	Context("Deploy and Scale Policy", func() {
		It("create a inform policy for the labeled cluster", func() {
			By("add the label to a managedcluster1 for the policy")
			assertAddLabel(managedClusters[0], POLICY_LABEL_KEY, POLICY_LABEL_VALUE)

			By("Check the inform policy in global hub")
			Eventually(func() error {
				status, err := getStatusFromGolbalHub(globalHubClient, httpClient, POLICY_NAME, POLICY_NAMESPACE)
				if err != nil {
					return err
				}
				for _, policyInfo := range status.Status {
					if policyInfo.ClusterName == managedClusters[0].Name {
						if policyInfo.ComplianceState == policiesv1.NonCompliant ||
							policyInfo.ComplianceState == policiesv1.Compliant {
							return nil
						}
					}
				}
				return fmt.Errorf("the policy have not applied to the managed cluster %s", managedClusters[0].Name)
			}, 3*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())
		})

		It("enforce the inform policy", func() {
			Eventually(func() error {
				_, err := testClients.Kubectl(testOptions.GlobalHub.Name, "apply", "-f", ENFORCE_POLICY_YAML)
				if err != nil {
					return err
				}
				return nil
			}, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())

			Eventually(func() error {
				status, err := getStatusFromGolbalHub(globalHubClient, httpClient, POLICY_NAME, POLICY_NAMESPACE)
				if err != nil {
					return err
				}
				for _, policyInfo := range status.Status {
					if policyInfo.ClusterName == managedClusters[0].Name && policyInfo.ComplianceState == policiesv1.Compliant {
						return nil
					}
				}
				return fmt.Errorf("the policy has not been applied to the managed cluster(%s) or it's not compliant", managedClusters[0].Name)
			}, 3*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())
		})

		It("scale the policy by label", func() {
			By("scale policy to other clusters")
			for i := 1; i < len(managedClusters); i++ {
				assertAddLabel(managedClusters[i], POLICY_LABEL_KEY, POLICY_LABEL_VALUE)

				By("Check the policy is created in global hub")
				Eventually(func() error {
					status, err := getStatusFromGolbalHub(globalHubClient, httpClient, POLICY_NAME, POLICY_NAMESPACE)
					if err != nil {
						return err
					}
					for _, policyInfo := range status.Status {
						if policyInfo.ClusterName == managedClusters[i].Name {
							if policyInfo.ComplianceState == policiesv1.Compliant {
								return nil
							}
						}
					}
					return fmt.Errorf("the policy have not applied to the managed cluster %s", managedClusters[i].Name)
				}, 3*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())
			}

			By("remove the label from the managedcluster1")
			assertRemoveLabel(managedClusters[0], POLICY_LABEL_KEY, POLICY_LABEL_VALUE)

			By("Check the policy is removed from the managedcluster1")
			Eventually(func() error {
				status, err := getStatusFromGolbalHub(globalHubClient, httpClient, POLICY_NAME, POLICY_NAMESPACE)
				if err != nil {
					return err
				}
				for _, policyInfo := range status.Status {
					if policyInfo.ClusterName == managedClusters[0].Name {
						return fmt.Errorf("the cluster %s policy(%s)should be removed", managedClusters[0].Name, POLICY_NAME)
					}
				}
				return nil
			}, 3*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())
		})
	})

	Context("Policy Finalizer", func() {
		It("verify the policy resource has been added the global cleanup finalizer", func() {
			Eventually(func() error {
				policy := &policiesv1.Policy{}
				err := globalHubClient.Get(ctx, client.ObjectKey{
					Namespace: POLICY_NAMESPACE,
					Name:      POLICY_NAME,
				}, policy)
				if err != nil {
					return err
				}
				for _, finalizer := range policy.Finalizers {
					if finalizer == "global-hub.open-cluster-management.io/cleanup" {
						return nil
					}
				}
				return fmt.Errorf("the policy(%s) hasn't been added the cleanup finalizer", policy.GetName())
			}, 1*time.Minute, 1*time.Second).Should(Succeed())

			By("Placementbinding")
			Eventually(func() error {
				placementbinding := &policiesv1.PlacementBinding{}
				err := globalHubClient.Get(ctx, client.ObjectKey{
					Namespace: POLICY_NAMESPACE,
					Name:      PLACEMENTBINDING_NAME,
				}, placementbinding)
				if err != nil {
					return err
				}
				for _, finalizer := range placementbinding.Finalizers {
					if finalizer == "global-hub.open-cluster-management.io/cleanup" {
						return nil
					}
				}
				return fmt.Errorf("the placementbinding(%s) hasn't been added the cleanup finalizer", placementbinding.GetName())
			}, 1*time.Minute, 1*time.Second).Should(Succeed())

			By("Placementrule")
			Eventually(func() error {
				placementrule := &placementrulev1.PlacementRule{}
				err := globalHubClient.Get(ctx, client.ObjectKey{
					Namespace: POLICY_NAMESPACE,
					Name:      PLACEMENT_RULE_NAME,
				}, placementrule)
				if err != nil {
					return err
				}
				for _, finalizer := range placementrule.Finalizers {
					if finalizer == "global-hub.open-cluster-management.io/cleanup" {
						return nil
					}
				}
				return fmt.Errorf("the placementrule(%s) hasn't been added the cleanup finalizer", placementrule.GetName())
			}, 1*time.Minute, 1*time.Second).Should(Succeed())
		})
	})

	AfterAll(func() {
		By("Delete the enforce policy from global hub")
		_, err := testClients.Kubectl(testOptions.GlobalHub.Name, "delete", "-f", ENFORCE_POLICY_YAML)
		Expect(err).ShouldNot(HaveOccurred())

		By("Check the enforce policy is deleted from managed hub")
		Eventually(func() error {
			for _, hubClient := range hubClients {
				_, err := getHubPolicyStatus(hubClient, POLICY_NAME, POLICY_NAMESPACE)
				if errors.IsNotFound(err) {
					continue
				}
				if err != nil {
					return err
				}
			}
			return nil
		}, 3*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())

		By("Delete the label from other managedclusters")
		for i := 1; i < len(managedClusters); i++ {
			assertRemoveLabel(managedClusters[i], POLICY_LABEL_KEY, POLICY_LABEL_VALUE)
		}

		// By("Delete the LimitRange CR from managedclusters")
		// for _, managedCluster := range managedClusters {
		// 	deleteInfo, err := testClients.Kubectl(managedCluster.Name, "delete", "LimitRange", "container-mem-limit-range")
		// 	Expect(err).ShouldNot(HaveOccurred(), deleteInfo)
		// }
	})
})

func getStatusFromGolbalHub(client client.Client, httpClient *http.Client, name, namespace string,
) (*policiesv1.PolicyStatus, error) {
	policy := &policiesv1.Policy{}
	err := client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, policy)
	if err != nil {
		return nil, err
	}

	policyUID := string(policy.UID)
	getPolicyStatusURL := fmt.Sprintf("%s/global-hub-api/v1/policy/%s/status",
		testOptions.GlobalHub.Nonk8sApiServer, policyUID)
	req, err := http.NewRequest("GET", getPolicyStatusURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(body, policy)
	if err != nil {
		return nil, err
	}

	return &policy.Status, nil
}

func getHubPolicyStatus(client client.Client, name, namespace string) (*policiesv1.PolicyStatus, error) {
	policy := &policiesv1.Policy{}
	err := client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, policy)
	if err != nil {
		return nil, err
	}

	return &policy.Status, nil
}
