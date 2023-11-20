package tests

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v4"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
)

const (
	LOCAL_INFORM_POLICY_YAML  = "../../resources/policy/local-inform-limitrange-policy.yaml"
	LOCAL_ENFORCE_POLICY_YAML = "../../resources/policy/local-enforce-limitrange-policy.yaml"

	LOCAL_POLICY_LABEL_KEY      = "local-policy"
	LOCAL_POLICY_LABEL_VALUE    = "test"
	LOCAL_POLICY_NAME           = "policy-limitrange"
	LOCAL_POLICY_NAMESPACE      = "local-policy-namespace"
	LOCAL_PLACEMENTBINDING_NAME = "binding-policy-limitrange"
	LOCAL_PLACEMENT_RULE_NAME   = "placementrule-policy-limitrange"
)

var _ = Describe("Apply local policy to the managed clusters", Ordered,
	Label("e2e-tests-local-policy"), func() {
		var leafhubClients []client.Client
		var postgresConn *pgx.Conn
		var err error

		BeforeAll(func() {
			By("Create runtime client")
			scheme := runtime.NewScheme()
			v1.AddToScheme(scheme)
			policiesv1.AddToScheme(scheme)
			placementrulev1.AddToScheme(scheme)

			// get multiple leafhubs
			for _, leafhubName := range leafHubNames {
				leafhubClient, err := testClients.ControllerRuntimeClient(leafhubName, scheme)
				Expect(err).Should(Succeed())
				// create local namespace on each leafhub
				err = leafhubClient.Create(context.TODO(), &v1.Namespace{ObjectMeta: metav1.ObjectMeta{
					Name: LOCAL_POLICY_NAMESPACE,
				}}, &client.CreateOptions{})
				if err != nil && !errors.IsAlreadyExists(err) {
					Expect(err).Should(Succeed())
				}
				leafhubClients = append(leafhubClients, leafhubClient)
			}

			By("Get postgres master pod name")
			databaseURI := strings.Split(testOptions.GlobalHub.DatabaseURI, "?")[0]
			postgresConn, err = database.PostgresConnection(context.TODO(), databaseURI, nil)
			Expect(err).Should(Succeed())
		})

		It("add the label to a managedcluster for the local policy", func() {
			By("Add local label to the managed cluster")
			for _, managedCluster := range managedClusters {
				Expect(updateClusterLabel(managedCluster.GetName(), LOCAL_POLICY_LABEL_KEY+"="+LOCAL_POLICY_LABEL_VALUE)).Should(Succeed())
			}
			Eventually(func() error {
				for _, managedCluster := range managedClusters {
					managedClusterInfo, err := getManagedClusterByName(httpClient, managedCluster.Name)
					if err != nil {
						return err
					}
					val, ok := managedClusterInfo.Labels[LOCAL_POLICY_LABEL_KEY]
					if !ok {
						return fmt.Errorf("the label [%s] is not exist", LOCAL_POLICY_LABEL_KEY)
					}
					if val != LOCAL_POLICY_LABEL_VALUE {
						return fmt.Errorf("the label [%s: %s] is not exist", LOCAL_POLICY_LABEL_KEY, LOCAL_POLICY_LABEL_VALUE)
					}
				}
				return nil
			}, 3*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())
		})

		Context("When deploy local policy to the leafhub", func() {
			It("deploy policy to the cluster to the leafhub", func() {
				By("Deploy the policy to the leafhub")
				for _, leafhubName := range leafHubNames {
					output, err := testClients.Kubectl(leafhubName, "apply", "-f", LOCAL_INFORM_POLICY_YAML)
					klog.V(10).Info(fmt.Sprintf("deploy inform local policy: %s", output))
					Expect(err).Should(Succeed())
				}

				By("Verify the local policy is directly synchronized to the global hub spec table")
				policies := make(map[string]*policiesv1.Policy)
				policyIds := sets.NewString()

				Eventually(func() error {
					rows, err := postgresConn.Query(context.TODO(), `select leaf_hub_name,payload from 
					local_spec.policies where deleted_at is null`)
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
						for _, leafhubName := range leafHubNames {
							if leafhub == leafhubName && policy.Name == LOCAL_POLICY_NAME && policy.Namespace == LOCAL_POLICY_NAMESPACE {
								policies[leafhub] = policy
								policyIds.Insert(string(policy.UID))
							}
						}
					}
					if len(policies) != len(leafHubNames) {
						return fmt.Errorf("expect policy has not synchronized")
					}
					return nil
				}, 3*time.Minute, 1*time.Second).Should(Succeed())

				By("Verify the local policy is synchronized to the global hub status table")
				Eventually(func() error {
					rows, err := postgresConn.Query(context.TODO(),
						"SELECT policy_id,cluster_name,cluster_id,leaf_hub_name FROM local_status.compliance")
					if err != nil {
						return err
					}
					managedClusterIds := sets.NewString()
					defer rows.Close()
					currentPolicyIds := sets.NewString()
					// policies, if leahfubname check remove the kv
					for rows.Next() {
						columnValues, _ := rows.Values()
						if len(columnValues) < 4 {
							return fmt.Errorf("the compliance record is not correct, expected 5 but got %d", len(columnValues))
						}
						policyId, cluster, cluster_id, leafhub := "", "", "", ""
						if err := rows.Scan(&policyId, &cluster, &cluster_id, &leafhub); err != nil {
							return err
						}
						if string(policies[leafhub].UID) == policyId {
							managedClusterIds.Insert(cluster_id)
							currentPolicyIds.Insert(policyId)
						}
					}
					if managedClusterIds.Len() != len(managedClusters) {
						return fmt.Errorf("not get all cluster status from local_status.compliance, current number:%v, expect:%v", managedClusterIds.Len(), len(managedClusters))
					}
					if len(policyIds) != currentPolicyIds.Len() {
						return fmt.Errorf("not get all policy from local_status.compliance, current number:%v, expect:%v", currentPolicyIds.Len(), policyIds.Len())
					}
					return nil
				}, 1*time.Minute, 10*time.Second).Should(Succeed())

				By("Verify the local policy events is synchronized to the global hub event.local_policies table")
				Eventually(func() error {
					rows, err := postgresConn.Query(context.TODO(),
						"select policy_id,cluster_id,leaf_hub_name from event.local_policies")
					if err != nil {
						return err
					}
					defer rows.Close()
					currentPolicyIds := sets.NewString()
					managedClusterIds := sets.NewString()
					for rows.Next() {
						columnValues, _ := rows.Values()
						if len(columnValues) < 3 {
							return fmt.Errorf("the event.local_policies record is not correct, expected 3 but got %d", len(columnValues))
						}
						policyId, cluster_id, leafhub := "", "", ""
						if err := rows.Scan(&policyId, &cluster_id, &leafhub); err != nil {
							return err
						}
						if policyIds.Has(policyId) {
							managedClusterIds.Insert(cluster_id)
							currentPolicyIds.Insert(policyId)
						}
					}
					if managedClusterIds.Len() != len(managedClusters) {
						return fmt.Errorf("not get all cluster status from local_status.compliance, current number:%v, expect:%v", managedClusterIds.Len(), len(managedClusters))
					}
					if len(policyIds) != currentPolicyIds.Len() {
						return fmt.Errorf("not get all policy from local_status.compliance, current number:%v, expect:%v", currentPolicyIds.Len(), policyIds.Len())
					}
					return nil
				}, 3*time.Minute, 1*time.Second).Should(Succeed())

				By("Verify the local policy events is synchronized to the global hub event.local_root_policies table")
				Eventually(func() error {
					rows, err := postgresConn.Query(context.TODO(),
						"select policy_id,leaf_hub_name from event.local_root_policies")
					if err != nil {
						return err
					}
					defer rows.Close()

					currentPolicyIds := sets.NewString()

					for rows.Next() {
						columnValues, _ := rows.Values()
						if len(columnValues) < 2 {
							return fmt.Errorf("the event.local_root_policies record is not correct, expected 2 but got %d", len(columnValues))
						}
						policyId, leafhub := "", ""
						if err := rows.Scan(&policyId, &leafhub); err != nil {
							return err
						}
						if policyIds.Has(policyId) {
							currentPolicyIds.Insert(policyId)
						}
					}
					if len(policyIds) != currentPolicyIds.Len() {
						return fmt.Errorf("not get all policy from local_status.compliance, current number:%v, expect:%v", currentPolicyIds.Len(), policyIds.Len())
					}
					return nil
				}, 3*time.Minute, 1*time.Second).Should(Succeed())
			})

			// no need to add the finalizer to the local policy
			// delete from bundle -> transport -> database
			It("check the local policy resource isn't added the global cleanup finalizer", func() {
				By("Verify the local policy hasn't been added the global hub cleanup finalizer")
				Eventually(func() error {
					for _, leafhubClient := range leafhubClients {
						policy := &policiesv1.Policy{}
						err := leafhubClient.Get(context.TODO(), client.ObjectKey{
							Namespace: LOCAL_POLICY_NAMESPACE,
							Name:      LOCAL_POLICY_NAME,
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

				// placementbinding is not be synchronized to the global hub database, so it doesn't need the finalizer
				By("Verify the local placementbinding hasn't been added the global hub cleanup finalizer")
				Eventually(func() error {
					for _, leafhubClient := range leafhubClients {
						placementbinding := &policiesv1.PlacementBinding{}
						err := leafhubClient.Get(context.TODO(), client.ObjectKey{
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

				// placementrule will be synced to the local_spec table, so it needs the finalizer
				By("Verify the local placementrule hasn't been added the global hub cleanup finalizer")
				Eventually(func() error {
					for _, leafhubClient := range leafhubClients {
						placementrule := &placementrulev1.PlacementRule{}
						err := leafhubClient.Get(context.TODO(), client.ObjectKey{
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

		Context("When delete the local policy from the leafhub", func() {
			It("delete the local policy from the leafhub", func() {
				By("Delete the policy from leafhub")
				for _, leafhubName := range leafHubNames {
					output, err := testClients.Kubectl(leafhubName, "delete", "-f", LOCAL_INFORM_POLICY_YAML)
					fmt.Println(output)
					Expect(err).Should(Succeed())
				}

				By("Verify the policy is delete from the leafhub")
				Eventually(func() error {
					notFoundCount := 0
					for _, leafhubClient := range leafhubClients {
						policy := &policiesv1.Policy{}
						err := leafhubClient.Get(context.TODO(), client.ObjectKey{
							Namespace: LOCAL_POLICY_NAMESPACE,
							Name:      LOCAL_POLICY_NAME,
						}, policy)
						if err != nil {
							if errors.IsNotFound(err) {
								notFoundCount++
								continue
							}
							return err
						}
						return fmt.Errorf("the policy(%s) is not deleted", policy.GetName())
					}
					if notFoundCount == len(leafhubClients) {
						return nil
					}
					return fmt.Errorf("the policy(%s) is not deleted from some leafhubClients", LOCAL_POLICY_NAME)
				}, 1*time.Minute, 1*time.Second).Should(Succeed())
			})

			It("check the local policy resource is deleted from database", func() {
				By("Verify the local policy is deleted from the spec table")
				Eventually(func() error {
					rows, err := postgresConn.Query(context.TODO(), `select payload from local_spec.policies where 
					deleted_at is null`)
					if err != nil {
						return err
					}
					defer rows.Close()
					policy := &policiesv1.Policy{}
					for rows.Next() {
						if err := rows.Scan(policy); err != nil {
							return err
						}
						if policy.Name == LOCAL_POLICY_NAME && policy.Namespace == LOCAL_POLICY_NAMESPACE {
							return fmt.Errorf("the policy(%s) is not deleted from local_spec.policies", policy.GetName())
						}
					}
					return nil
				}, 1*time.Minute, 1*time.Second).Should(Succeed())

				By("Verify the local policy is deleted from the global hub status table")
				Eventually(func() error {
					rows, err := postgresConn.Query(context.TODO(),
						"SELECT policy_id,cluster_name,leaf_hub_name FROM local_status.compliance")
					if err != nil {
						return err
					}
					defer rows.Close()

					for rows.Next() {
						columnValues, _ := rows.Values()
						if len(columnValues) < 3 {
							return fmt.Errorf("the compliance record is not correct, expected 5 but got %d", len(columnValues))
						}
						policyId, cluster, leafhub := "", "", ""
						if err := rows.Scan(&policyId, &cluster, &leafhub); err != nil {
							return err
						}
						for i, leafhubName := range leafHubNames {
							if cluster == managedClusters[i].Name && leafhub == leafhubName {
								return fmt.Errorf("the policy(%s) is not deleted from local_status.compliance", policyId)
							}
						}
					}
					return nil
				}, 1*time.Minute, 1*time.Second).Should(Succeed())
			})
		})

		AfterAll(func() {
			By("Close the postgresql connection")
			Expect(postgresConn.Close(context.Background())).Should(Succeed())
		})
	})
