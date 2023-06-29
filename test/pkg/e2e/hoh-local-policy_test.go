package tests

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v4"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
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
		var managedClusters []clusterv1.ManagedCluster
		var postgresConn *pgx.Conn
		var err error

		BeforeAll(func() {
			Eventually(func() error {
				By("Config request of the api")
				transport := &http.Transport{
					TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
				}
				httpClient = &http.Client{Timeout: time.Second * 60, Transport: transport}
				managedClusters, err = getManagedCluster(httpClient, httpToken)
				if err != nil {
					return err
				}
				if len(managedClusters) != ExpectedManagedClusterNum {
					return fmt.Errorf("managed cluster number error")
				}
				return nil
			}, 5*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())

			By("Create runtime client")
			scheme := runtime.NewScheme()
			v1.AddToScheme(scheme)
			policiesv1.AddToScheme(scheme)
			placementrulev1.AddToScheme(scheme)

			// get multiple leafhubs
			for _, leafhubName := range LeafHubNames {
				leafhubClient, err := clients.ControllerRuntimeClient(leafhubName, scheme)
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
			fmt.Printf("\n localOptions.LocalHubCluster.DatabaseURI: \n %v\n", localOptions.LocalHubCluster.DatabaseURI)
			databaseURI := strings.Split(localOptions.LocalHubCluster.DatabaseURI, "?")[0]
			fmt.Printf("\n databaseURI: \n %v\n", databaseURI)
			postgresConn, err = database.PostgresConnection(context.TODO(), databaseURI, nil)
			Expect(err).Should(Succeed())
		})

		It("add the label to a managedcluster for the local policy", func() {
			By("Add local label to the managed cluster")
			patches := []patch{
				{
					Op:    "add",
					Path:  "/metadata/labels/" + LOCAL_POLICY_LABEL_KEY,
					Value: LOCAL_POLICY_LABEL_VALUE,
				},
			}
			for _, managedCluster := range managedClusters {
				Expect(updateClusterLabel(httpClient, patches, httpToken, GetClusterID(managedCluster))).Should(Succeed())
			}
			Eventually(func() error {
				for _, managedCluster := range managedClusters {
					managedClusterInfo, err := getManagedClusterByName(httpClient, httpToken, managedCluster.Name)
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
			}, 3*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())
		})

		Context("When deploy local policy to the leafhub", func() {
			It("deploy policy to the cluster to the leafhub", func() {
				By("Deploy the policy to the leafhub")
				for _, leafhubName := range LeafHubNames {
					fmt.Printf("\n leafhubName: \n %v\n", leafhubName)
					output, err := clients.Kubectl(leafhubName, "apply", "-f", LOCAL_INFORM_POLICY_YAML)
					fmt.Printf("\n Deploy the policy to the leafhub err: \n %v\n", err)
					fmt.Printf("\n Deploy the policy to the leafhub output: \n %v\n", output)
					klog.V(10).Info(fmt.Sprintf("deploy inform local policy: %s", output))
					Expect(err).Should(Succeed())
				}

				By("Verify the local policy is directly synchronized to the global hub spec table")
				policies := make(map[string]*policiesv1.Policy)
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
						fmt.Printf("local_spec.policies: %s/%s \n", policy.Namespace, policy.Name)
						for _, leafhubName := range LeafHubNames {
							if leafhub == leafhubName && policy.Name == LOCAL_POLICY_NAME && policy.Namespace == LOCAL_POLICY_NAMESPACE {
								policies[leafhub] = policy
							}
						}
					}
					if len(policies) != len(LeafHubNames) {
						return fmt.Errorf("expect policy has not synchronized")
					}
					return nil
				}, 5*time.Minute, 5*time.Second).Should(Succeed())

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
				for _, leafhubName := range LeafHubNames {
					output, err := clients.Kubectl(leafhubName, "delete", "-f", LOCAL_INFORM_POLICY_YAML)
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

				By("Verify the local placementrule is deleted from the spec table")
				Eventually(func() error {
					rows, err := postgresConn.Query(context.TODO(), "select payload from local_spec.placementrules")
					if err != nil {
						return err
					}
					defer rows.Close()
					placementrule := &placementrulev1.PlacementRule{}
					for rows.Next() {
						if err := rows.Scan(placementrule); err != nil {
							return err
						}
						if placementrule.Name == LOCAL_PLACEMENT_RULE_NAME && placementrule.Namespace == LOCAL_POLICY_NAMESPACE {
							return fmt.Errorf("the placementrule(%s) is not deleted from local_spec.policies", placementrule.Name)
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
						for i, leafhubName := range LeafHubNames {
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