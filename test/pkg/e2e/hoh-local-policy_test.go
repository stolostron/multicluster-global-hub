package tests

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/db/postgresql"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
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
		var leafhubClient client.Client
		var managedClusterName1 string
		var managedClusterUID1 string
		var postgresSQL *postgresql.PostgreSQL

		BeforeAll(func() {
			By("Get managed cluster name")
			Eventually(func() error {
				managedClusters, err := getManagedCluster(httpClient, httpToken)
				if err != nil {
					return err
				}
				managedClusterUID1 = string(managedClusters[0].GetUID())
				managedClusterName1 = managedClusters[0].Name
				return nil
			}, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())

			By("Create runtime client")
			scheme := runtime.NewScheme()
			v1.AddToScheme(scheme)
			policiesv1.AddToScheme(scheme)
			var err error
			runtimeClient, err = clients.ControllerRuntimeClient(clients.HubClusterName(), scheme)
			Expect(err).Should(Succeed())
			leafhubClient, err = clients.ControllerRuntimeClient(clients.LeafHubClusterName(), scheme)
			Expect(err).Should(Succeed())

			By("Create local policy namespace on leafhub")
			err = leafhubClient.Create(context.TODO(), &v1.Namespace{ObjectMeta: metav1.ObjectMeta{
				Name: LOCAL_POLICY_NAMESPACE,
			}}, &client.CreateOptions{})
			if !errors.IsAlreadyExists(err) {
				Expect(err).Should(Succeed())
			}

			By("Get postgres master pod name")
			postgresSQL, err = postgresql.NewPostgreSQL(testOptions.HubCluster.DatabaseURI)
			Expect(err).Should(Succeed())

			By("Add local label to the managed cluster")
			patches := []patch{
				{
					Op:    "add",
					Path:  "/metadata/labels/" + LOCAL_POLICY_LABEL_KEY,
					Value: LOCAL_POLICY_LABEL_VALUE,
				},
			}
			Eventually(func() error {
				err := updateClusterLabel(httpClient, patches, httpToken, managedClusterUID1)
				if err != nil {
					return err
				}

				managedCluster, err := getManagedClusterByName(httpClient, httpToken, managedClusterName1)
				if err != nil {
					return err
				}
				if val, ok := managedCluster.Labels[LOCAL_POLICY_LABEL_KEY]; ok {
					if val == LOCAL_POLICY_LABEL_VALUE {
						return nil
					}
				}
				return fmt.Errorf("the label [%s: %s] is not exist", LOCAL_POLICY_LABEL_KEY, LOCAL_POLICY_LABEL_VALUE)
			}, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())
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
			Expect(globalConfig.Data["enableLocalPolicies"]).Should(Equal("true"))

			By("Disable the local policy")
			globalConfig.Data["enableLocalPolicies"] = "false"
			err = runtimeClient.Update(context.TODO(), globalConfig, &client.UpdateOptions{})
			Expect(err).Should(Succeed())

			By("Verify the leafhub config")
			leafhubConfig := &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.GHConfigCMName,
					Namespace: constants.GHSystemNamespace,
				},
			}
			Eventually(func() error {
				err := leafhubClient.Get(context.TODO(), client.ObjectKeyFromObject(leafhubConfig),
					leafhubConfig, &client.GetOptions{})
				if err != nil {
					return err
				}
				if globalConfig.Data["enableLocalPolicies"] != "false" {
					return fmt.Errorf("the global hub agent enableLocalPolicies should be false")
				}
				return nil
			}, 30*time.Second, 1*time.Second).Should(Succeed())

			By("Rollback the enable local policy config")
			globalConfig.Data["enableLocalPolicies"] = "true"
			err = runtimeClient.Update(context.TODO(), globalConfig, &client.UpdateOptions{})
			Expect(err).Should(Succeed())
			Eventually(func() error {
				err := leafhubClient.Get(context.TODO(), client.ObjectKeyFromObject(leafhubConfig),
					leafhubConfig, &client.GetOptions{})
				if err != nil {
					return err
				}
				if globalConfig.Data["enableLocalPolicies"] != "true" {
					return fmt.Errorf("the global hub agent enableLocalPolicies should be true")
				}
				return nil
			}, 30*time.Second, 1*time.Second).Should(Succeed())
		})

		It("deploy policy to the cluster with local policy label", func() {
			By("Deploy the policy to the leafhub")
			output, err := clients.Kubectl(clients.LeafHubClusterName(), "apply", "-f", LOCAL_INFORM_POLICY_YAML)
			klog.V(5).Info(fmt.Sprintf("deploy inform local policy: %s", output))
			Expect(err).Should(Succeed())

			By("Verify the local policy is synchronized to the global hub spec table")
			policy := &policiesv1.Policy{}
			Eventually(func() error {
				rows, err := postgresSQL.GetConn().Query(context.TODO(), "select payload from local_spec.policies")
				if err != nil {
					return err
				}
				defer rows.Close()
				for rows.Next() {
					if err := rows.Scan(policy); err != nil {
						return err
					}
					fmt.Printf("local_spec.policies: %s/%s \n", policy.Namespace, policy.Name)
					if policy.Name == LOCAL_POLICY_NAME && policy.Namespace == LOCAL_POLICY_NAMESPACE {
						return nil
					}
				}
				return fmt.Errorf("expect policy [%s/%s] but got [%s/%s]", LOCAL_POLICY_NAMESPACE, LOCAL_POLICY_NAME,
					policy.Namespace, policy.Name)
			}, 1*time.Minute, 1*time.Second).Should(Succeed())

			By("Verify the local policy is synchronized to the global hub status table")
			Eventually(func() error {
				rows, err := postgresSQL.GetConn().Query(context.TODO(),
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
					if policyId == string(policy.UID) && cluster == managedClusterName1 &&
						leafhub == clients.LeafHubClusterName() {
						return nil
					}
				}
				return fmt.Errorf("not get policy(%s) from local_status.compliance", policy.UID)
			}, 1*time.Minute, 1*time.Second).Should(Succeed())
		})

		AfterAll(func() {
			By("Delete the policy from leafhub")
			output, err := clients.Kubectl(clients.LeafHubClusterName(), "delete", "-f", LOCAL_INFORM_POLICY_YAML)
			fmt.Println(output)
			Expect(err).Should(Succeed())

			By("Delete the managed cluster")
			postgresSQL.Stop()
		})
	})
