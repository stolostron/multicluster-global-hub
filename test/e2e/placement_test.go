package tests

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

const (
	PLACEMENT_POLICY_YAML       = "./../manifest/policy/inform-limitrange-policy-placement.yaml"
	PLACEMENT_LOCAL_POLICY_YAML = "./../manifest/policy/local-inform-limitrange-policy-placement.yaml"
	CLUSTERSET_LABEL_KEY        = "cluster.open-cluster-management.io/clusterset"
)

var _ = Describe("Apply policy with placement on the global hub", Ordered, Label("e2e-test-placement"), func() {
	Context("Placement with Local Policy", func() {
		localPolicyName := "policy-limitrange" // mclset/mclsetbinding: default
		localPolicyNamespace := "local-placement"
		localPlacementName := "placement-policy-limitrange"
		localPolicyLabelStr := "local-policy-placement=test"
		localPolicyLabelRemoveStr := "local-policy-placement-"

		BeforeAll(func() {
			By("Deploy the placement policy to the leafhubs")
			for _, leafhubName := range managedHubNames {
				output, err := testClients.Kubectl(leafhubName, "apply", "-f", PLACEMENT_LOCAL_POLICY_YAML)
				fmt.Println("create local policy with placement", output)
				Expect(err).Should(Succeed(), string(output))
			}
		})

		It("deploy local policy on the managed hub", func() {
			for _, cluster := range managedClusters {
				Expect(updateClusterLabel(cluster.GetName(), localPolicyLabelStr)).Should(Succeed())
			}

			By("Verify the policy is scheduled to leaf hubs")
			Eventually(func() error {
				var localPolicies []models.LocalSpecPolicy
				err := database.GetGorm().Find(&localPolicies).Error
				if err != nil {
					return err
				}

				for _, leafHubName := range managedHubNames {
					found := false
					for _, p := range localPolicies {
						if p.LeafHubName == leafHubName &&
							p.PolicyName == localPolicyName {
							found = true
							break
						}
					}
					if !found {
						return fmt.Errorf("the local policy is not scheduled to the leafhub %s", leafHubName)
					}
				}
				return nil
			}, 2*time.Minute, 1*time.Second).Should(Succeed())
		})

		It("check the local policy(placement) resource isn't added the global cleanup finalizer", func() {
			By("Local Policy")
			Eventually(func() error {
				for _, hubClient := range hubClients {
					policy := &policiesv1.Policy{}
					err := hubClient.Get(ctx, client.ObjectKey{
						Namespace: localPolicyNamespace,
						Name:      localPolicyName,
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

			// placement is not be synchronized to the global hub database, so it doesn't need the finalizer
			By("Verify the local placement hasn't been added the global hub cleanup finalizer")
			Eventually(func() error {
				for _, leafhubClient := range hubClients {
					placement := &clusterv1beta1.Placement{}
					err := leafhubClient.Get(ctx, client.ObjectKey{
						Namespace: localPolicyNamespace,
						Name:      localPlacementName,
					}, placement)
					if err != nil {
						return err
					}
					for _, finalizer := range placement.Finalizers {
						if finalizer == constants.GlobalHubCleanupFinalizer {
							return fmt.Errorf("the local placement(%s) has been added the cleanup finalizer",
								placement.GetName())
						}
					}
				}
				return nil
			}, 1*time.Minute, 1*time.Second).Should(Succeed())
		})

		AfterAll(func() {
			for _, leafhubName := range managedHubNames {
				output, err := testClients.Kubectl(leafhubName, "delete", "-f", PLACEMENT_LOCAL_POLICY_YAML)
				fmt.Println("delete local policy with placement", output)
				Expect(err).To(Succeed(), string(output))
			}

			By("Verify the local policy(placement) is deleted from the database")
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
					if policy.Name == localPolicyName && policy.Namespace == localPolicyNamespace {
						return fmt.Errorf("the policy(%s) is not deleted from local_spec.policies", policy.GetName())
					}
				}
				return nil
			}, 3*time.Minute, 1*time.Second).Should(Succeed())

			By("Remove local policy test label")
			for _, cluster := range managedClusters {
				Expect(updateClusterLabel(cluster.GetName(), localPolicyLabelRemoveStr)).Should(Succeed())
			}
		})
	})
})
