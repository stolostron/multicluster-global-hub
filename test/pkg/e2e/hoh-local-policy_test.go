package tests

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"
	"time"
	"net/http"

	"github.com/jackc/pgx/v4"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
)

const (
	LOCAL_INFORM_POLICY_YAML  = "../../resources/policy/local-inform-limitrange-policy.yaml"
	LOCAL_ENFORCE_POLICY_YAML = "../../resources/policy/local-enforce-limitrange-policy.yaml"

	LOCAL_POLICY_LABEL_KEY   = "local-policy"
	LOCAL_POLICY_LABEL_VALUE = "test"
	LOCAL_POLICY_NAME        = "policy-limitrange"
	LOCAL_POLICY_NAMESPACE   = "local-policy-namespace"
)

var _ = Describe("Apply local policy to the managed clusters", Ordered,
	Label("e2e-tests-local-policy"), func() {
		var runtimeClient client.Client
		var leafhubClients []client.Client
		var managedClusterNames []string
		var managedClusterUIDs []string
		var leafhubNames []string
		var postgresConn *pgx.Conn
		var err error

		BeforeAll(func() {
			Eventually(func() error {
				By("Config request of the api")
				transport := &http.Transport{
					TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
				}
				httpClient = &http.Client{Timeout: time.Second * 20, Transport: transport}
				managedClusters, err := getManagedCluster(httpClient, httpToken)
				if err != nil {
					return err
				}
				for _, managedCluster := range managedClusters {
					managedClusterNames = append(managedClusterNames, managedCluster.Name)
					managedClusterUIDs = append(managedClusterUIDs, string(managedCluster.GetUID()))
				}
				return nil
			}, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())

			By("Create runtime client")
			scheme := runtime.NewScheme()
			v1.AddToScheme(scheme)
			policiesv1.AddToScheme(scheme)
			runtimeClient, err = clients.ControllerRuntimeClient(clients.HubClusterName(), scheme)
			Expect(err).Should(Succeed())

			// get multiple leafhubs
			leafhubNames = clients.GetLeafHubClusterNames()
			for _, leafhubName := range leafhubNames{
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
			databaseURI := strings.Split(testOptions.HubCluster.DatabaseURI, "?")[0]
			postgresConn, err = database.PostgresConnection(context.TODO(), databaseURI, nil)
			Expect(err).Should(Succeed())
		})

		It("add the label to a managedcluster for the local policy", func() {
			for i, managedClusterName := range managedClusterNames {
				patches := []patch{
					{
						Op:    "add",
						Path:  "/metadata/labels/" + LOCAL_POLICY_LABEL_KEY,
						Value: LOCAL_POLICY_LABEL_VALUE,
					},
				}
				Eventually(func() error {
					err := updateClusterLabel(httpClient, patches, httpToken, managedClusterUIDs[i])
					if err != nil {
						return err
					}
					return nil
				}, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())
	
				By("Check the label is added")
				Eventually(func() error {
					managedCluster, err := getManagedClusterByName(httpClient, httpToken, managedClusterName)
					if err != nil {
						return err
					}
					if val, ok := managedCluster.Labels[LOCAL_POLICY_LABEL_KEY]; ok {
						if val == LOCAL_POLICY_LABEL_VALUE {
							return nil
						}
					}
					return fmt.Errorf("the label %s: %s is not exist", POLICY_LABEL_KEY, POLICY_LABEL_VALUE)
				}, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())
			}
		})

		It("verify the multicluster global hub configmap", func() {
			By("Check the global hub config")
			globalConfig := &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.GHConfigCMName,
					Namespace: constants.GHSystemNamespace,
				},
			}
			err := runtimeClient.Get(context.TODO(), client.ObjectKeyFromObject(globalConfig),
				globalConfig, &client.GetOptions{})
			Expect(err).Should(Succeed())
			// Expect(globalConfig.Data["enableLocalPolicies"]).Should(Equal("true"))

			By("Disable the local policy")
			globalConfig.Data["enableLocalPolicies"] = "false"
			err = runtimeClient.Update(context.TODO(), globalConfig, &client.UpdateOptions{})
			Expect(err).Should(Succeed())

			By("Verify the leafhub config")
			leafhubConfigs := make([]*v1.ConfigMap, len(leafhubClients))
			Eventually(func() error {
				for i, leafhubClient := range leafhubClients {
					leafhubConfigs[i] = &v1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      constants.GHConfigCMName,
							Namespace: constants.GHSystemNamespace,
						},
					}
					err := leafhubClient.Get(context.TODO(), client.ObjectKeyFromObject(leafhubConfigs[i]),
					leafhubConfigs[i], &client.GetOptions{})
					if err != nil {
						return err
					}
					if globalConfig.Data["enableLocalPolicies"] != "false" {
						return fmt.Errorf("the global hub agent enableLocalPolicies should be false")
					}
				}
				return nil
			}, 30*time.Second, 1*time.Second).Should(Succeed())

			By("Rollback the enable local policy config")
			globalConfig.Data["enableLocalPolicies"] = "true"
			err = runtimeClient.Update(context.TODO(), globalConfig, &client.UpdateOptions{})
			Expect(err).Should(Succeed())

			Eventually(func() error {
				for i, leafhubClient := range leafhubClients {
					err := leafhubClient.Get(context.TODO(), client.ObjectKeyFromObject(leafhubConfigs[i]),
					leafhubConfigs[i], &client.GetOptions{})
					if err != nil {
						return err
					}
					if leafhubConfigs[i].Data["enableLocalPolicies"] != "true" {
						return fmt.Errorf("the global hub agent enableLocalPolicies should be true")
					}
				}
				return nil
			}, 30*time.Second, 1*time.Second).Should(Succeed())
		})

		It("deploy policy to the cluster with local policy label", func() {
			By("Deploy the policy to the leafhub")
			for _, leafhubName := range leafhubNames {
				output, err := clients.Kubectl(leafhubName, "apply", "-f", LOCAL_INFORM_POLICY_YAML)
				klog.V(5).Info(fmt.Sprintf("deploy inform local policy: %s", output))
				Expect(err).Should(Succeed())
			}

			By("Verify the local policy is synchronized to the global hub spec table")
			policies := []*policiesv1.Policy{}
			Eventually(func() error {
				policy := &policiesv1.Policy{}
				rows, err := postgresConn.Query(context.TODO(), "select payload from local_spec.policies")
				if err != nil {
					return err
				}
				defer rows.Close()
				for rows.Next() {
					if err := rows.Scan(policy); err != nil {
						return err
					}
					fmt.Printf("local_spec.policies: %s/%s \n", policy.Namespace, policy.Name)
					if policy.Name != LOCAL_POLICY_NAME || policy.Namespace != LOCAL_POLICY_NAMESPACE {
						return fmt.Errorf("expect policy [%s/%s] but got [%s/%s]", LOCAL_POLICY_NAMESPACE, LOCAL_POLICY_NAME, policy.Namespace, policy.Name)
					}
					policies = append(policies, policy)
				}
				return nil
			}, 1*time.Minute, 1*time.Second).Should(Succeed())

			By("Verify the local policy is synchronized to the global hub status table")
			Eventually(func() error {
				rows, err := postgresConn.Query(context.TODO(),
					"SELECT id,cluster_name,leaf_hub_name FROM local_status.compliance")
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
					// only for cases that one-to-one leafhub and manged cluster
					var foundpolicy bool
					for i, leafhubName := range leafhubNames {
						if policyId == string(policies[i].UID) && cluster == managedClusterNames[i] && leafhub == leafhubName {
							foundpolicy = true
							break
						}
					}
					if !foundpolicy {
						return fmt.Errorf("not get policy(%s) from local_status.compliance", policyId)
					}
				}
				return nil
			}, 1*time.Minute, 1*time.Second).Should(Succeed())
		})

		AfterAll(func() {
			By("Delete the policy from leafhub")
			for _, leafhubName := range leafhubNames {
				output, err := clients.Kubectl(leafhubName, "delete", "-f", LOCAL_INFORM_POLICY_YAML)
				fmt.Println(output)
				Expect(err).Should(Succeed())
	
				By("Delete the managed cluster")
				err = postgresConn.Close(context.Background())
				Expect(err).Should(Succeed())
			}
		})
	})
