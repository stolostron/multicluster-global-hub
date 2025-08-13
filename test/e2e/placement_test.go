package tests

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	appsv1alpha1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

const (
	PLACEMENT_POLICY_YAML       = "./../manifest/policy/inform-limitrange-policy-placement.yaml"
	PLACEMENT_APP_SUB_YAML      = "./../manifest/app/app-helloworld-appsub-placement.yaml"
	PLACEMENT_LOCAL_POLICY_YAML = "./../manifest/policy/local-inform-limitrange-policy-placement.yaml"
	CLUSTERSET_LABEL_KEY        = "cluster.open-cluster-management.io/clusterset"
)

var _ = Describe("Apply policy/app with placement on the global hub", Ordered, Label("e2e-test-placement"), func() {
	clusterSet1 := "clusterset1"

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

	Context("Placement with Global Policy", Label("e2e-test-global-resource"), func() {
		policyName := "policy-limitrange"
		policyNamespace := "global-placement"
		BeforeAll(func() {
			By("Add managedCluster2 to the clusterset1")
			assertAddLabel(managedClusters[1], CLUSTERSET_LABEL_KEY, clusterSet1)

			By("Deploy the policy to the global hub")
			output, err := testClients.Kubectl(testOptions.GlobalHub.Name, "apply", "-f", PLACEMENT_POLICY_YAML)
			klog.V(5).Info(fmt.Sprintf("deploy inform policy with placement: %s", output))
			Expect(err).Should(Succeed())
		})
		It("deploy and scale policy by placement", func() {
			By("Check the inform policy managedCluster2")
			Eventually(func() error {
				status, err := getStatusFromGolbalHub(globalHubClient, httpClient, policyName, policyNamespace)
				if err != nil {
					return err
				}
				if len(status.Status) == 1 && status.Status[0].ClusterName == managedClusters[1].Name {
					return nil
				}
				return fmt.Errorf("the policy have not applied to the managed cluster %s", managedClusters[1].Name)
			}, 1*time.Minute, 1*time.Second).Should(Succeed())

			By("Remove managedCluster2 from clusterset1")
			assertRemoveLabel(managedClusters[1], CLUSTERSET_LABEL_KEY, clusterSet1)

			By("Check the inform policy is removed from managedCluster2")
			Eventually(func() error {
				status, err := getStatusFromGolbalHub(globalHubClient, httpClient, policyName, policyNamespace)
				if err != nil {
					return err
				}
				fmt.Println(status.Status)
				if len(status.Status) == 0 {
					return nil
				}
				return fmt.Errorf("the policy should removed from managed cluster %s", managedClusters[1].Name)
			}, 1*time.Minute, 1*time.Second).Should(Succeed())
		})

		AfterAll(func() {
			By("Delete the policy in the global hub")
			output, err := testClients.Kubectl(testOptions.GlobalHub.Name, "delete", "-f", PLACEMENT_POLICY_YAML)
			klog.V(5).Info(fmt.Sprintf("delete inform policy with placement: %s", output))
			Expect(err).Should(Succeed())

			By("Check the inform policy in global hub")
			Eventually(func() error {
				_, err := getStatusFromGolbalHub(globalHubClient, httpClient, policyName, policyNamespace)
				if errors.IsNotFound(err) {
					return nil
				}
				if err != nil {
					return err
				}
				return fmt.Errorf("the policy should be removed from global hub")
			}, 1*time.Minute, 1*time.Second).Should(Succeed())
		})
	})

	Context("Placement with Application", Label("e2e-test-global-resource"), func() {
		BeforeAll(func() {
			// it will scheduled to default clusterset with label app: test
			output, err := testClients.Kubectl(testOptions.GlobalHub.Name, "apply", "-f", PLACEMENT_APP_SUB_YAML)
			klog.V(5).Info("create application with placement", "output", output)
			Expect(err).To(Succeed(), output)
		})
		It("deploy application with placement", func() {
			By("Add app label to the managedClusters")
			for _, cluster := range managedClusters {
				assertAddLabel(cluster, APP_LABEL_KEY, APP_LABEL_VALUE)
			}

			By("Check the appsub is applied to the cluster")
			Eventually(func() error {
				return checkAppsubreport(httpClient, APP_SUB_NAME, APP_SUB_NAMESPACE, 2,
					[]string{managedClusters[0].Name, managedClusters[1].Name})
			}, 3*time.Minute, 1*time.Second).Should(Succeed())
		})

		It("scale application with placement", func() {
			By("Move managedCluster2 to the clusterset1")
			assertAddLabel(managedClusters[1], CLUSTERSET_LABEL_KEY, clusterSet1)

			By("Check the appsub is applied to the cluster")
			Eventually(func() error {
				return checkAppsubreport(httpClient, APP_SUB_NAME, APP_SUB_NAMESPACE, 1,
					[]string{managedClusters[0].Name})
			}, 3*time.Minute, 1*time.Second).Should(Succeed())
		})

		AfterAll(func() {
			By("Delete the appsub")
			output, err := testClients.Kubectl(testOptions.GlobalHub.Name, "delete", "-f", PLACEMENT_APP_SUB_YAML)
			klog.V(5).Info("delete application with placement", "output", output)
			Expect(err).Should(Succeed())

			By("Move managedCluster2 to the default clusterset")
			assertRemoveLabel(managedClusters[1], CLUSTERSET_LABEL_KEY, clusterSet1)

			By("Remove the app label")
			for _, cluster := range managedClusters {
				assertRemoveLabel(cluster, APP_LABEL_KEY, APP_LABEL_VALUE)
			}

			By("manually remove the appsubreport on the managed hub") // TODO: remove this step after the issue is fixed
			for i, hubClient := range hubClients {
				// current each managed hub with 1 cluster
				appsubreport := &appsv1alpha1.SubscriptionReport{
					ObjectMeta: metav1.ObjectMeta{
						Name:      managedClusters[i].Name,
						Namespace: managedClusters[i].Name,
					},
				}
				Expect(hubClient.Delete(ctx, appsubreport, &client.DeleteOptions{})).Should(Succeed())
			}

			By("Check the appsub is deleted")
			Eventually(func() error {
				var reports []models.SubscriptionReport
				err := database.GetGorm().Find(&reports).Error
				if err != nil {
					return err
				}
				for _, r := range reports {
					appsubreport := &appsv1alpha1.SubscriptionReport{}
					if err = json.Unmarshal(r.Payload, appsubreport); err != nil {
						return err
					}
					fmt.Printf("status.subscription_reports: %s/%s ", appsubreport.Namespace, appsubreport.Name)
					if appsubreport.Name == APP_SUB_NAME && appsubreport.Namespace == APP_SUB_NAMESPACE {
						return fmt.Errorf("the appsub is not deleted from managed hub")
					}

				}
				return nil
			}, 2*time.Minute, 1*time.Second).Should(Succeed())
		})
	})
})
