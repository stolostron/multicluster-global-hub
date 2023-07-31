package tests

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v4"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	clusterv1beta2 "open-cluster-management.io/api/cluster/v1beta2"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	appsv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1"
	appsv1alpha1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	PLACEMENT_POLICY_YAML       = "../../resources/policy/inform-limitrange-policy-placement.yaml"
	PLACEMENT_APP_SUB_YAML      = "../../resources/app/app-helloworld-appsub-placement.yaml"
	PLACEMENT_LOCAL_POLICY_YAML = "../../resources/policy/local-inform-limitrange-policy-placement.yaml"

	PLACEMENT_APP        = "../../resources/policy/enforce-limitrange-policy.yaml"
	CLUSTERSET_LABEL_KEY = "cluster.open-cluster-management.io/clusterset"
)

var _ = Describe("Apply policy/app with placement on the global hub", Ordered, Label("e2e-tests-placement"), func() {
	var globalClient client.Client
	var leafhubClients []client.Client
	var policyName, policyNamespace, policyClusterset string
	var localPolicyName, localPolicyNamespace, localPlacementName, localPolicyLabelKey, localPolicyLabelVal string
	var postgresConn *pgx.Conn

	BeforeAll(func() {
		By("Initialize the variables")
		policyName = "policy-limitrange"
		policyNamespace = "global-placement"

		localPolicyName = "policy-limitrange" // mclset/mclsetbinding: default
		localPolicyNamespace = "local-placement"
		localPlacementName = "placement-policy-limitrange"
		localPolicyLabelKey = "local-policy-placement"
		localPolicyLabelVal = "test"
		var err error

		policyClusterset = "clusterset1"

		By("Init the client")
		scheme := runtime.NewScheme()
		Expect(policiesv1.AddToScheme(scheme))
		Expect(corev1.AddToScheme(scheme))
		Expect(placementrulev1.AddToScheme(scheme))
		Expect(clusterv1beta1.AddToScheme(scheme))
		Expect(clusterv1beta2.AddToScheme(scheme))
		Expect(clusterv1.AddToScheme(scheme))
		Expect(appsv1.SchemeBuilder.AddToScheme(scheme))
		Expect(appsv1alpha1.AddToScheme(scheme))
		globalClient, err = testClients.ControllerRuntimeClient(testOptions.HubCluster.Name, scheme)
		Expect(err).ShouldNot(HaveOccurred())

		for _, leafhubName := range leafHubNames {
			leafhubClient, err := testClients.ControllerRuntimeClient(leafhubName, scheme)
			Expect(err).ShouldNot(HaveOccurred())
			// create local namespace on each leafhub
			leafhubClients = append(leafhubClients, leafhubClient)
		}

		By("Create Postgres connection")
		databaseURI := strings.Split(testOptions.HubCluster.DatabaseURI, "?")[0]
		postgresConn, err = database.PostgresConnection(context.TODO(), databaseURI, nil)
		Expect(err).Should(Succeed())
	})

	Context("When apply local policy with placement on the managed hub", func() {
		It("deploy local policy on the managed hub", func() {
			By("Add local policy test label")
			patches := []patch{
				{
					Op:    "add", // or remove
					Path:  "/metadata/labels/" + localPolicyLabelKey,
					Value: localPolicyLabelVal,
				},
			}
			Expect(updateClusterLabel(httpClient, patches,
				GetClusterID(managedClusters[0]))).Should(Succeed())
			Expect(updateClusterLabel(httpClient, patches, GetClusterID(managedClusters[1]))).Should(Succeed())

			By("Deploy the placement policy to the leafhub")
			for _, leafhubName := range leafHubNames {
				output, err := testClients.Kubectl(leafhubName, "apply", "-f", PLACEMENT_LOCAL_POLICY_YAML)
				fmt.Printf("deploy inform local policy:\n %s \n", output)
				Expect(err).Should(Succeed())
			}

			By("Verify the local policy is directly synchronized to the global hub spec table")
			policies := make(map[string]*policiesv1.Policy)
			Eventually(func() error {
				rows, err := postgresConn.Query(context.TODO(), `select leaf_hub_name,payload from local_spec.policies 
				where deleted_at is null`)
				if err != nil {
					return err
				}
				defer rows.Close()
				for rows.Next() {
					policy := &policiesv1.Policy{}
					leafhub := ""
					if err := rows.Scan(&leafhub, policy); err != nil {
						return err
					}
					fmt.Printf("local_spec.policies: %s/%s \n", policy.Namespace, policy.Name)
					for _, leafhubName := range leafHubNames {
						if leafhub == leafhubName && policy.Name == localPolicyName && policy.Namespace == localPolicyNamespace {
							policies[leafhub] = policy
							fmt.Println(len(policies))
						}
					}
				}
				if len(policies) != len(leafHubNames) {
					return fmt.Errorf("expect policy has not synchronized")
				}
				return nil
			}, 1*time.Minute, 1*time.Second).Should(Succeed())

			By("Verify the local policy is synchronized to the global hub status table")
			Eventually(func() error {
				rows, err := postgresConn.Query(context.TODO(),
					"SELECT policy_id,cluster_name,leaf_hub_name FROM local_status.compliance")
				if err != nil {
					return err
				}
				defer rows.Close()

				// policies, if leahfubname check remove the kv
				for rows.Next() {
					columnValues, _ := rows.Values()
					if len(columnValues) < 3 {
						return fmt.Errorf("the compliance record is not correct, expected 5 but got %d", len(columnValues))
					}
					policyId, cluster, leafhub := "", "", ""
					if err := rows.Scan(&policyId, &cluster, &leafhub); err != nil {
						return err
					}
					if string(policies[leafhub].UID) == policyId {
						delete(policies, leafhub)
					}
				}
				if len(policies) == ExpectedLeafHubNum {
					return fmt.Errorf("not get policy from local_status.compliance")
				}
				return nil
			}, 1*time.Minute, 1*time.Second).Should(Succeed())
		})

		// to use the finalizer achieves deleting local resource from database:
		// finalizer(deprecated) -> delete from bundle -> transport -> database
		It("check the local policy(placement) resource isn't added the global cleanup finalizer", func() {
			By("Verify the local policy(placement) hasn't been added the global hub cleanup finalizer")
			Eventually(func() error {
				for _, leafhubClient := range leafhubClients {
					policy := &policiesv1.Policy{}
					err := leafhubClient.Get(context.TODO(), client.ObjectKey{
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
				for _, leafhubClient := range leafhubClients {
					placement := &clusterv1beta1.Placement{}
					err := leafhubClient.Get(context.TODO(), client.ObjectKey{
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

		It("delete the local policy(placement) from the leafhub", func() {
			By("Delete the local policy from leafhub")
			output, err := testClients.Kubectl(leafHubNames[0], "delete", "-f", PLACEMENT_LOCAL_POLICY_YAML)
			fmt.Println(output)
			Expect(err).Should(Succeed())

			output, err = testClients.Kubectl(leafHubNames[1], "delete", "-f", PLACEMENT_LOCAL_POLICY_YAML)
			fmt.Println(output)
			Expect(err).Should(Succeed())

			By("Verify the local policy(placement) is deleted from the spec table")
			Eventually(func() error {
				rows, err := postgresConn.Query(context.TODO(), `select payload from local_spec.policies where 
				deleted_at is null`)
				if err != nil {
					return err
				}
				fmt.Println("Verify the local policy(placement) is deleted from the spec tabl")
				defer rows.Close()
				for rows.Next() {
					policy := &policiesv1.Policy{}
					if err := rows.Scan(policy); err != nil {
						return err
					}
					if policy.Name == localPolicyName && policy.Namespace == localPolicyNamespace {
						return fmt.Errorf("the policy(%s) is not deleted from local_spec.policies", policy.GetName())
					}
				}
				return nil
			}, 3*time.Minute, 5*time.Second).Should(Succeed())

			By("Verify the local policy(placement) is deleted from the global hub status table")
			Eventually(func() error {
				rows, err := postgresConn.Query(context.TODO(), "SELECT policy_id,cluster_name,leaf_hub_name FROM local_status.compliance")
				if err != nil {
					fmt.Println(err)
					return err
				}
				fmt.Println("Verify the local policy(placement) is deleted from the global hub status table")
				defer rows.Close()
				for rows.Next() {
					columnValues, _ := rows.Values()
					return fmt.Errorf("the policy(%s) is not deleted from local_status.compliance", columnValues)
				}
				return nil
			}, 1*time.Minute, 1*time.Second).Should(Succeed())

			By("Remove local policy test label")
			patches := []patch{
				{
					Op:    "remove",
					Path:  "/metadata/labels/" + localPolicyLabelKey,
					Value: localPolicyLabelVal,
				},
			}
			Expect(updateClusterLabel(httpClient, patches, GetClusterID(managedClusters[0]))).Should(Succeed())
			Expect(updateClusterLabel(httpClient, patches, GetClusterID(managedClusters[1]))).Should(Succeed())
		})
	})

	Context("When apply global policy with placement on the global hub", func() {
		It("add managedCluster2 to the clusterset1", func() {
			patches := []patch{
				{
					Op:    "add", // or remove
					Path:  "/metadata/labels/" + CLUSTERSET_LABEL_KEY,
					Value: policyClusterset,
				},
			}
			Expect(updateClusterLabel(httpClient, patches, GetClusterID(managedClusters[1]))).Should(Succeed())
			Eventually(func() error {
				managedCluster, err := getManagedClusterByName(httpClient, managedClusters[1].Name)
				if err != nil {
					return err
				}
				if val, ok := managedCluster.Labels[CLUSTERSET_LABEL_KEY]; ok && val == policyClusterset {
					return nil
				}
				return fmt.Errorf("the label %s: %s is not exist on mcl2", CLUSTERSET_LABEL_KEY, policyClusterset)
			}, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())
		})

		It("apply policy with placement", func() {
			By("Deploy the policy to the global hub")
			output, err := testClients.Kubectl(testOptions.HubCluster.Name, "apply", "-f", PLACEMENT_POLICY_YAML)
			klog.V(5).Info(fmt.Sprintf("deploy inform policy with placement: %s", output))
			Expect(err).Should(Succeed())

			By("Check the inform policy in global hub")
			Eventually(func() error {
				status, err := getPolicyStatus(globalClient, httpClient, policyName, policyNamespace)
				if err != nil {
					return err
				}
				if len(status.Status) == 1 &&
					status.Status[0].ClusterName == managedClusters[1].Name {
					return nil
				}
				return fmt.Errorf("the policy have not applied to the managed cluster %s", managedClusters[1].Name)
			}, 1*time.Minute, 1*time.Second).Should(Succeed())
		})

		It("scale policy with placement", func() {
			patches := []patch{
				{
					Op:    "remove", // or remove
					Path:  "/metadata/labels/" + CLUSTERSET_LABEL_KEY,
					Value: policyClusterset,
				},
			}
			Expect(updateClusterLabel(httpClient, patches, GetClusterID(managedClusters[1]))).Should(Succeed())

			By("Check the inform policy in global hub")
			Eventually(func() error {
				status, err := getPolicyStatus(globalClient, httpClient, policyName, policyNamespace)
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

		It("delete policy with placement", func() {
			By("Delete the policy in the global hub")
			output, err := testClients.Kubectl(testOptions.HubCluster.Name, "delete", "-f", PLACEMENT_POLICY_YAML)
			klog.V(5).Info(fmt.Sprintf("delete inform policy with placement: %s", output))
			Expect(err).Should(Succeed())

			By("Check the inform policy in global hub")
			Eventually(func() error {
				_, err := getPolicyStatus(globalClient, httpClient, policyName, policyNamespace)
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

	Context("When apply global application with placement on the global hub", func() {
		It("deploy application with placement", func() {
			By("Add app label to the managedClusters")
			patches := []patch{
				{
					Op:    "add",
					Path:  "/metadata/labels/" + APP_LABEL_KEY,
					Value: APP_LABEL_VALUE,
				},
			}
			Expect(updateClusterLabel(httpClient, patches, GetClusterID(managedClusters[0]))).Should(Succeed())
			Expect(updateClusterLabel(httpClient, patches, GetClusterID(managedClusters[1]))).Should(Succeed())

			By("Apply the appsub to labeled clusters")
			Eventually(func() error {
				_, err := testClients.Kubectl(testOptions.HubCluster.Name, "apply", "-f", PLACEMENT_APP_SUB_YAML)
				if err != nil {
					return err
				}
				return nil
			}, 1*time.Minute, 1*time.Second).Should(Succeed())

			By("Check the appsub is applied to the cluster")
			Eventually(func() error {
				return checkAppsubreport(globalClient, httpClient, APP_SUB_NAME, APP_SUB_NAMESPACE, 2,
					[]string{managedClusters[0].Name, managedClusters[1].Name})
			}, 3*time.Minute, 1*time.Second).Should(Succeed())
		})

		It("scale application with placement", func() {
			By("Move managedCluster2 to the clusterset1")
			patches := []patch{
				{
					Op:    "add",
					Path:  "/metadata/labels/" + CLUSTERSET_LABEL_KEY,
					Value: policyClusterset,
				},
			}
			Expect(updateClusterLabel(httpClient, patches, GetClusterID(managedClusters[1]))).Should(Succeed())

			By("Check the appsub is applied to the cluster")
			Eventually(func() error {
				return checkAppsubreport(globalClient, httpClient, APP_SUB_NAME, APP_SUB_NAMESPACE, 1,
					[]string{managedClusters[0].Name})
			}, 3*time.Minute, 1*time.Second).Should(Succeed())
		})

		It("delete application with placement", func() {
			By("Delete the appsub")
			_, err := testClients.Kubectl(testOptions.HubCluster.Name, "delete", "-f", PLACEMENT_APP_SUB_YAML)
			Expect(err).Should(Succeed())

			By("Move managedCluster2 to the default clusterset")
			patches := []patch{
				{
					Op:    "remove",
					Path:  "/metadata/labels/" + CLUSTERSET_LABEL_KEY,
					Value: policyClusterset,
				},
			}
			Expect(updateClusterLabel(httpClient, patches, GetClusterID(managedClusters[1]))).Should(Succeed())

			By("Remove app label")
			patches = []patch{
				{
					Op:    "remove",
					Path:  "/metadata/labels/" + APP_LABEL_KEY,
					Value: APP_LABEL_VALUE,
				},
			}
			Expect(updateClusterLabel(httpClient, patches, GetClusterID(managedClusters[0]))).Should(Succeed())
			Expect(updateClusterLabel(httpClient, patches, GetClusterID(managedClusters[1]))).Should(Succeed())

			By("manually remove the appsubreport on the managed hub") // TODO: remove this step after the issue is fixed
			appsubreport := &appsv1alpha1.SubscriptionReport{
				ObjectMeta: metav1.ObjectMeta{
					Name:      managedClusters[0].Name,
					Namespace: managedClusters[0].Name,
				},
			}
			Expect(leafhubClients[0].Delete(context.TODO(), appsubreport, &client.DeleteOptions{})).Should(Succeed())
			appsubreport.SetName(managedClusters[1].Name)
			appsubreport.SetNamespace(managedClusters[1].Name)
			Expect(leafhubClients[1].Delete(context.TODO(), appsubreport, &client.DeleteOptions{})).Should(Succeed())

			By("Check the appsub is deleted")
			Eventually(func() error {
				appsub := &appsv1.Subscription{}
				err := globalClient.Get(context.TODO(), types.NamespacedName{Namespace: APP_SUB_NAMESPACE, Name: APP_SUB_NAME},
					appsub)
				if err != nil && !errors.IsNotFound(err) {
					return err
				} else if err == nil {
					return fmt.Errorf("the appsub is not deleted from global hub")
				}

				rows, err := postgresConn.Query(context.TODO(), "select leaf_hub_name,payload from status.subscription_reports")
				if err != nil {
					return err
				}
				defer rows.Close()
				appsubreport := &appsv1alpha1.SubscriptionReport{}
				leafhub := ""
				for rows.Next() {
					if err := rows.Scan(&leafhub, appsubreport); err != nil {
						return err
					}
					fmt.Printf("status.subscription_reports: %s/%s \n", appsubreport.Namespace, appsubreport.Name)
					if appsubreport.Name == APP_SUB_NAME && appsubreport.Namespace == APP_SUB_NAMESPACE {
						return fmt.Errorf("the appsub is not deleted from managed hub")
					}
				}
				return nil
			}, 2*time.Minute, 1*time.Second).Should(Succeed())
		})
	})

	AfterAll(func() {
		By("Close the postgresql connection")
		Expect(postgresConn.Close(context.Background())).Should(Succeed())
	})
})
