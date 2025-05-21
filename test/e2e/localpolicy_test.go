package tests

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

const (
	LOCAL_INFORM_POLICY_YAML  = "./../manifest/policy/local-inform-limitrange-policy.yaml"
	LOCAL_ENFORCE_POLICY_YAML = "./../manifest/policy/local-enforce-limitrange-policy.yaml"

	LOCAL_POLICY_LABEL_KEY      = "local-policy"
	LOCAL_POLICY_LABEL_VALUE    = "test"
	LOCAL_POLICY_NAME           = "policy-limitrange"
	LOCAL_POLICY_NAMESPACE      = "local-policy-namespace"
	LOCAL_PLACEMENTBINDING_NAME = "binding-policy-limitrange"
	LOCAL_PLACEMENT_RULE_NAME   = "placementrule-policy-limitrange"
)

var _ = Describe("Local Policy", Ordered, Label("e2e-test-localpolicy"), func() {
	hubToPolicyMap := make(map[string]*policiesv1.Policy)
	policyIds := sets.NewString()

	BeforeAll(func() {
		By("Add local policy label to the managed cluster")
		for _, managedCluster := range managedClusters {
			assertAddLabel(managedCluster, LOCAL_POLICY_LABEL_KEY, LOCAL_POLICY_LABEL_VALUE)
		}

		By("Deploy the policy to the leafhub")
		for _, leafhubName := range managedHubNames {
			output, err := testClients.Kubectl(leafhubName, "apply", "-f", LOCAL_INFORM_POLICY_YAML)
			Expect(err).To(Succeed(), output)
		}
	})

	Context("Policy Spec and Compliance", Ordered, func() {
		It("policy spec", func() {
			Eventually(func() error {
				var localPolicies []models.LocalSpecPolicy
				err := database.GetGorm().Find(&localPolicies).Error
				if err != nil {
					return err
				}
				for _, p := range localPolicies {
					policy := &policiesv1.Policy{}
					err = json.Unmarshal(p.Payload, policy)
					if err != nil {
						return err
					}
					if policy.Name == LOCAL_POLICY_NAME && policy.Namespace == LOCAL_POLICY_NAMESPACE {
						hubToPolicyMap[p.LeafHubName] = policy
						policyIds.Insert(string(policy.UID))
					}
				}
				if len(hubToPolicyMap) != len(managedHubNames) {
					return fmt.Errorf("expect policy num %d, but got %d, policys: %v", len(managedHubNames), len(hubToPolicyMap), hubToPolicyMap)
				}
				return nil
			}, 3*time.Minute, 1*time.Second).Should(Succeed())
		})

		It("policy compliance", func() {
			Eventually(func() error {
				var localCompliances []models.LocalStatusCompliance
				err := database.GetGorm().Find(&localCompliances).Error
				if err != nil {
					return err
				}

				count := 0
				for _, c := range localCompliances {
					if string(hubToPolicyMap[c.LeafHubName].UID) == c.PolicyID {
						count++
					}
				}
				if count != len(managedClusters) {
					return fmt.Errorf("compliance num want %d, but got %d", len(managedClusters), count)
				}
				return nil
			}, 1*time.Minute, 1*time.Second).Should(Succeed())
		})

		It("compliance history", func() {
			Eventually(func() error {
				var localHistories []models.LocalComplianceHistory
				err := database.GetGorm().Find(&localHistories).Error
				if err != nil {
					return err
				}

				count := 0
				for _, c := range localHistories {
					if string(hubToPolicyMap[c.LeafHubName].UID) == c.PolicyID {
						count++
					}
				}
				if count != len(managedClusters) {
					return fmt.Errorf("history compliance num want %d, but got %d", len(managedClusters), count)
				}
				return nil
			}, 3*time.Minute, 10*time.Second).Should(Succeed())
		})

		It("replicated policy event", func() {
			Eventually(func() error {
				var policyEvents []models.LocalReplicatedPolicyEvent
				err := database.GetGorm().Find(&policyEvents).Error
				if err != nil {
					return err
				}

				count := 0
				eventPolicyIds := sets.NewString()
				fmt.Printf("local policy events: %v\n", policyEvents)
				for _, e := range policyEvents {
					fmt.Println(e.LeafHubName, e.ClusterName, e.EventName, e.Message, e.Compliance)
					if string(hubToPolicyMap[e.LeafHubName].UID) == e.PolicyID {
						count++
						eventPolicyIds.Insert(e.PolicyID)
					}
				}
				if count < len(managedClusters) {
					return fmt.Errorf("replicated policy event num at least %d, but got %d", len(managedClusters), count)
				}
				if eventPolicyIds.Len() < len(policyIds) {
					return fmt.Errorf("policy num(policy event) at least %d, but got %d", len(policyIds), eventPolicyIds.Len())
				}
				return nil
			}, 3*time.Minute, 1*time.Second).Should(Succeed())
		})

		It("root policy event", func() {
			Eventually(func() error {
				var policyEvents []models.LocalRootPolicyEvent
				err := database.GetGorm().Find(&policyEvents).Error
				if err != nil {
					return err
				}

				eventPolicyIds := sets.NewString()
				count := 0
				fmt.Println("local root policy events: ")
				for _, e := range policyEvents {
					fmt.Println(e.LeafHubName, e.EventName, e.Message, e.Compliance)
					if string(hubToPolicyMap[e.LeafHubName].UID) == e.PolicyID {
						count++
						eventPolicyIds.Insert(e.PolicyID)
					}
				}
				if count != len(managedClusters) {
					return fmt.Errorf("root policy event num want %d, but got %d", len(managedClusters), count)
				}

				if len(policyIds) != eventPolicyIds.Len() {
					return fmt.Errorf("policy num(root policy event): want %d, but got %d", len(policyIds), eventPolicyIds.Len())
				}
				return nil
			}, 3*time.Minute, 1*time.Second).Should(Succeed())
		})
	})

	// no need to add the finalizer to the local policy
	Context("Global Hub Cleanup Finalizer", func() {
		It("check the policy hasn't been added the finalizer", func() {
			Eventually(func() error {
				for _, hubClient := range hubClients {
					policy := &policiesv1.Policy{}
					err := hubClient.Get(ctx, client.ObjectKey{
						Namespace: LOCAL_POLICY_NAMESPACE, Name: LOCAL_POLICY_NAME,
					}, policy)
					if err != nil {
						return err
					}
					for _, finalizer := range policy.Finalizers {
						if finalizer == constants.GlobalHubCleanupFinalizer {
							return fmt.Errorf("the local policy(%s) has been added the cleanup finalizer", policy.GetName())
						}
					}
				}
				return nil
			}, 1*time.Minute, 1*time.Second).Should(Succeed())
		})
		It("check the placementbinding", func() {
			Eventually(func() error {
				for _, hubClient := range hubClients {
					placementbinding := &policiesv1.PlacementBinding{}
					err := hubClient.Get(ctx, client.ObjectKey{
						Namespace: LOCAL_POLICY_NAMESPACE,
						Name:      LOCAL_PLACEMENTBINDING_NAME,
					}, placementbinding)
					if err != nil {
						return err
					}
					for _, finalizer := range placementbinding.Finalizers {
						if finalizer == constants.GlobalHubCleanupFinalizer {
							return fmt.Errorf("the local placementbinding(%s) has been added the cleanup finalizer",
								placementbinding.GetName())
						}
					}
				}
				return nil
			}, 1*time.Minute, 1*time.Second).Should(Succeed())
		})

		It("check the placementrule", func() {
			Eventually(func() error {
				for _, hubClient := range hubClients {
					placementrule := &placementrulev1.PlacementRule{}
					err := hubClient.Get(ctx, client.ObjectKey{
						Namespace: LOCAL_POLICY_NAMESPACE,
						Name:      LOCAL_PLACEMENT_RULE_NAME,
					}, placementrule)
					if err != nil {
						return err
					}
					for _, finalizer := range placementrule.Finalizers {
						if finalizer == constants.GlobalHubCleanupFinalizer {
							return fmt.Errorf("the local placementrule(%s) has been added the cleanup finalizer", placementrule.GetName())
						}
					}
				}
				return nil
			}, 1*time.Minute, 1*time.Second).Should(Succeed())
		})
	})

	AfterAll(func() {
		By("Delete the policy from leafhub")
		for _, leafhubName := range managedHubNames {
			output, err := testClients.Kubectl(leafhubName, "delete", "-f", LOCAL_INFORM_POLICY_YAML)
			fmt.Println(output)
			Expect(err).Should(Succeed())
		}

		By("Verify the policy is delete from the leafhub")
		Eventually(func() error {
			notFoundCount := 0
			for _, hubClient := range hubClients {
				_, err := getHubPolicyStatus(hubClient, LOCAL_POLICY_NAME, LOCAL_POLICY_NAMESPACE)
				if errors.IsNotFound(err) {
					notFoundCount++
					continue
				}
				if err != nil {
					return err
				}
				return fmt.Errorf("the policy(%s) is not deleted", LOCAL_POLICY_NAME)
			}
			if notFoundCount == len(hubClients) {
				return nil
			}
			return fmt.Errorf("the policy(%s) is not deleted from some hubs", LOCAL_POLICY_NAME)
		}, 1*time.Minute, 1*time.Second).Should(Succeed())

		By("Verify the policy is delete from the spec table")
		Eventually(func() error {
			var localPolicies []models.LocalSpecPolicy
			err := database.GetGorm().Find(&localPolicies).Error
			if err != nil {
				return err
			}
			for _, p := range localPolicies {
				// since we have local agent enabled, so that localPolicies has local-cluster.
				// but hubToPolicyMap does not have local-cluster
				if _, found := hubToPolicyMap[p.LeafHubName]; !found {
					continue
				}
				if string(hubToPolicyMap[p.LeafHubName].UID) == p.PolicyID {
					return fmt.Errorf("the policy(%s) is not deleted from local_spec.policies", p.PolicyName)
				}
			}
			return nil
		}, 1*time.Minute, 1*time.Second).Should(Succeed())

		By("Verify the policy is delete from the compliance table")
		Eventually(func() error {
			var localCompliances []models.LocalStatusCompliance
			err := database.GetGorm().Find(&localCompliances).Error
			if err != nil {
				return err
			}
			for _, c := range localCompliances {
				if string(hubToPolicyMap[c.LeafHubName].UID) == c.PolicyID {
					return fmt.Errorf("the compliance(%s: %s) is not deleted from local_spec.policies",
						hubToPolicyMap[c.LeafHubName].Name, c.ClusterName)
				}
			}
			return nil
		}, 1*time.Minute, 1*time.Second).Should(Succeed())
	})
})
