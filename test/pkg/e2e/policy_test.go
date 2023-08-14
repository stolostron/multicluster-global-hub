package tests

import (
	"context"
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
	var globalClient client.Client
	var managedClient client.Client
	var managedClients []client.Client
	var err error

	BeforeAll(func() {
		By("Get the appsubreport client")
		scheme := runtime.NewScheme()
		policiesv1.AddToScheme(scheme)
		corev1.AddToScheme(scheme)
		placementrulev1.AddToScheme(scheme)
		globalClient, err = testClients.ControllerRuntimeClient(testOptions.GlobalHub.Name, scheme)
		Expect(err).ShouldNot(HaveOccurred())
		for _, leafhubName := range leafHubNames {
			managedClient, err = testClients.ControllerRuntimeClient(leafhubName, scheme)
			managedClients = append(managedClients, managedClient)
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
			err := updateClusterLabel(httpClient, patches, GetClusterID(managedClusters[0]))
			if err != nil {
				return err
			}
			managedClusterInfo, err := getManagedClusterByName(httpClient, managedClusters[0].Name)
			if err != nil {
				return err
			}
			if val, ok := managedClusterInfo.Labels[POLICY_LABEL_KEY]; ok {
				if val == POLICY_LABEL_VALUE && managedClusterInfo.Name == managedClusters[0].Name {
					return nil
				}
			}
			return fmt.Errorf("the label %s: %s is not exist", POLICY_LABEL_KEY, POLICY_LABEL_VALUE)
		}, 3*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())
	})

	It("create a inform policy for the labeled cluster", func() {
		By("Create the inform policy in global hub")
		Eventually(func() error {
			message, err := testClients.Kubectl(testOptions.GlobalHub.Name, "apply", "-f", INFORM_POLICY_YAML)
			if err != nil {
				klog.V(5).Info(fmt.Sprintf("apply inform policy error: %s", message))
				return err
			}
			return nil
		}, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())

		By("Check the inform policy in global hub")
		Eventually(func() error {
			status, err := getPolicyStatus(globalClient, httpClient, POLICY_NAME, POLICY_NAMESPACE)
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

		By("Check the inform policy in managed hub")
		Eventually(func() error {
			status, err := getManagedPolicyStatus(managedClients[0], POLICY_NAME, POLICY_NAMESPACE)
			if err != nil {
				return err
			}

			policyStatusStr, _ := json.MarshalIndent(status, "", "  ")
			klog.V(5).Info(fmt.Sprintf("get policy status: %s", policyStatusStr))

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
			status, err := getPolicyStatus(globalClient, httpClient, POLICY_NAME, POLICY_NAMESPACE)
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

	It("add the label to a managedcluster for the policy", func() {
		for i := 1; i < len(managedClusters); i++ {
			patches := []patch{
				{
					Op:    "add", // or remove
					Path:  "/metadata/labels/" + POLICY_LABEL_KEY,
					Value: POLICY_LABEL_VALUE,
				},
			}

			By("Check the label is added")
			Eventually(func() error {
				err := updateClusterLabel(httpClient, patches, GetClusterID(managedClusters[i]))
				if err != nil {
					return err
				}
				managedClusterInfo, err := getManagedClusterByName(httpClient, managedClusters[i].Name)
				if err != nil {
					return err
				}
				if val, ok := managedClusterInfo.Labels[POLICY_LABEL_KEY]; ok {
					if val == POLICY_LABEL_VALUE && managedClusterInfo.Name == managedClusters[i].Name {
						return nil
					}
				}
				return fmt.Errorf("the label %s: %s is not exist", POLICY_LABEL_KEY, POLICY_LABEL_VALUE)
			}, 3*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())

			By("Check the policy is created in global hub")
			Eventually(func() error {
				status, err := getPolicyStatus(globalClient, httpClient, POLICY_NAME, POLICY_NAMESPACE)
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

			By("Check the policy is created in managed hub")
			Eventually(func() error {
				status, err := getManagedPolicyStatus(managedClient, POLICY_NAME, POLICY_NAMESPACE)
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
	})

	It("remove managedcluster1 policy by deleting label", func() {
		By("Check the policy is created in managedcluster1")
		Eventually(func() error {
			status, err := getPolicyStatus(globalClient, httpClient, POLICY_NAME, POLICY_NAMESPACE)
			if err != nil {
				return err
			}
			for _, policyInfo := range status.Status {
				if policyInfo.ClusterName == managedClusters[0].Name {
					return nil
				}
			}
			return fmt.Errorf("the policy should be in the managedcluster1")
		}, 3*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())

		By("remove the label from the managedcluster1")
		patches := []patch{
			{
				Op:    "remove",
				Path:  "/metadata/labels/" + POLICY_LABEL_KEY,
				Value: POLICY_LABEL_VALUE,
			},
		}
		Eventually(func() error {
			err := updateClusterLabel(httpClient, patches, GetClusterID(managedClusters[0]))
			if err != nil {
				return err
			}
			return nil
		}, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())

		By("Check the policy is removed from the managedcluster1")
		Eventually(func() error {
			status, err := getPolicyStatus(globalClient, httpClient, POLICY_NAME, POLICY_NAMESPACE)
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
		_, err := testClients.Kubectl(testOptions.GlobalHub.Name, "delete", "-f", ENFORCE_POLICY_YAML)
		Expect(err).ShouldNot(HaveOccurred())

		By("Check the enforce policy is deleted from managed hub")
		Eventually(func() error {
			_, err := getManagedPolicyStatus(managedClient, POLICY_NAME, POLICY_NAMESPACE)
			if errors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			return fmt.Errorf("the policy should be removed from managed hub")
		}, 3*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())

		By("Delete the label from managedcluster2")
		patches := []patch{
			{
				Op:    "remove",
				Path:  "/metadata/labels/" + POLICY_LABEL_KEY,
				Value: POLICY_LABEL_VALUE,
			},
		}
		Eventually(func() error {
			for i := 1; i < len(managedClusters); i++ {
				err := updateClusterLabel(httpClient, patches, GetClusterID(managedClusters[i]))
				if err != nil {
					return err
				}
			}
			return nil
		}, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())

		// By("Delete the LimitRange CR from managedclusters")
		// for _, managedCluster := range managedClusters {
		// 	deleteInfo, err := clients.Kubectl(managedCluster.Name, "delete", "LimitRange", "container-mem-limit-range")
		// 	Expect(err).ShouldNot(HaveOccurred())
		// 	klog.V(5).Info(managedCluster.Name, ": ", deleteInfo)
		// }
	})
})

func getPolicyStatus(client client.Client, httpClient *http.Client, name, namespace string,
) (*policiesv1.PolicyStatus, error) {
	policy := &policiesv1.Policy{}
	err := client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: name}, policy)
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

	klog.V(5).Info(fmt.Sprintf("Get policy status response body from non-k8s-api: \n%s\n", body))

	err = json.Unmarshal(body, policy)
	if err != nil {
		return nil, err
	}

	return &policy.Status, nil
}

func getManagedPolicyStatus(client client.Client, name, namespace string) (*policiesv1.PolicyStatus, error) {
	policy := &policiesv1.Policy{}
	err := client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: name}, policy)
	if err != nil {
		return nil, err
	}

	return &policy.Status, nil
}
