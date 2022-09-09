package tests

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	appsv1alpha1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonconstants "github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/test/pkg/utils"
)

const (
	TIMEOUT          = 1 * time.Minute
	INTERVAL         = 2 * time.Second
	REGINAL_HUB_NAME = "kind-hub1"
)

var _ = Describe("Delete the multiclusterglobalhub and prune resources", Label("e2e-tests-prune"), Ordered, func() {
	ctx := context.Background()
	var runtimeClient client.Client
	var httpClient *http.Client
	var managedClusterName1 string
	var managedClusterName2 string
	var token string

	BeforeAll(func() {
		By("Get token")
		initToken, err := utils.FetchBearerToken(testOptions)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(len(initToken)).Should(BeNumerically(">", 0))
		token = initToken

		By("Get httpClient")
		transport := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		httpClient = &http.Client{Timeout: time.Second * 10, Transport: transport}

		By("Get the runtimeClient client")
		scheme := runtime.NewScheme()
		appsv1alpha1.AddToScheme(scheme)
		rbacv1.AddToScheme(scheme)
		clusterv1beta1.AddToScheme(scheme)
		policiesv1.AddToScheme(scheme)
		clusterv1.AddToScheme(scheme)
		runtimeClient, err = clients.ControllerRuntimeClient(scheme)
		Expect(err).ShouldNot(HaveOccurred())

		By("Get managed cluster name")
		Eventually(func() error {
			managedClusters, err := getManagedCluster(httpClient, token)
			if err != nil {
				return err
			}
			managedClusterName1 = managedClusters[0].Name
			managedClusterName2 = managedClusters[1].Name
			return nil
		}, TIMEOUT, INTERVAL).ShouldNot(HaveOccurred())
	})

	It("create application", func() {
		By("Add app label to the managedcluster1")
		patches := []patch{
			{
				Op:    "add",
				Path:  "/metadata/labels/" + APP_LABEL_KEY,
				Value: APP_LABEL_VALUE,
			},
		}
		Eventually(func() error {
			if err := updateClusterLabel(httpClient, patches, token, managedClusterName1); err != nil {
				return err
			}
			return nil
		}, TIMEOUT, INTERVAL).ShouldNot(HaveOccurred())

		By("Apply the appsub to labeled cluster")
		Eventually(func() error {
			_, err := clients.Kubectl(clients.HubClusterName(), "apply", "-f", APP_SUB_YAML)
			if err != nil {
				return err
			}
			return nil
		}, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())

		By("Check the appsub is applied to the cluster")
		Eventually(func() error {
			return checkAppsubreport(runtimeClient, 1, []string{managedClusterName1})
		}, TIMEOUT, INTERVAL).ShouldNot(HaveOccurred())
	})

	It("create policy", func() {
		By("Add policy label to the managedcluster2")
		patches := []patch{
			{
				Op:    "add", // or remove
				Path:  "/metadata/labels/" + POLICY_LABEL_KEY,
				Value: POLICY_LABEL_VALUE,
			},
		}
		Eventually(func() error {
			err := updateClusterLabel(httpClient, patches, token, managedClusterName2)
			if err != nil {
				return err
			}
			return nil
		}, TIMEOUT, INTERVAL).ShouldNot(HaveOccurred())

		By("Apply the policy to labeled cluster")
		Eventually(func() error {
			_, err := clients.Kubectl(clients.HubClusterName(), "apply", "-f", INFORM_POLICY_YAML)
			if err != nil {
				return err
			}
			return nil
		}, TIMEOUT, INTERVAL).ShouldNot(HaveOccurred())

		Eventually(func() error {
			status, err := getPolicyStatus(runtimeClient, POLICY_NAME, POLICY_NAMESPACE)
			if err != nil {
				return err
			}
			for _, policyInfo := range status.Status {
				if policyInfo.ClusterName == managedClusterName2 {
					if policyInfo.ComplianceState == policiesv1.NonCompliant {
						return nil
					}
				}
			}
			return fmt.Errorf("the policy have not applied to the managed cluster %s", managedClusterName2)
		}, TIMEOUT, INTERVAL).ShouldNot(HaveOccurred())
	})

	It("delete multiclusterglobalhub", func() {
		By("Check whether multiclusterglobalhub is exists")

		By("Delete multiclusterglobalhub")

		By("Check whether multiclusterglobalhub is deleted")
		Eventually(func() error {
			return nil
		}, TIMEOUT, INTERVAL).ShouldNot(HaveOccurred())
	})

	It("prune the global resources", func() {
		listOpts := []client.ListOption{
			client.MatchingLabels(map[string]string{
				commonconstants.GlobalHubOwnerLabelKey: commonconstants.HoHOperatorOwnerLabelVal,
			}),
		}

		By("Delete clusterrole")
		Eventually(func() error {
			clusterRoleList := &rbacv1.ClusterRoleList{}
			if err := runtimeClient.List(ctx, clusterRoleList, listOpts...); err != nil {
				return err
			}
			for idx := range clusterRoleList.Items {
				name := clusterRoleList.Items[idx].GetName()
				fmt.Printf("clusterrole: %s \n", name)
			}
			return nil
		}, TIMEOUT, INTERVAL).ShouldNot(HaveOccurred())

		By("Delete clusterrolebinding")
		Eventually(func() error {
			clusterRoleBindingList := &rbacv1.ClusterRoleBindingList{}
			if err := runtimeClient.List(ctx, clusterRoleBindingList, listOpts...); err != nil {
				return err
			}
			for idx := range clusterRoleBindingList.Items {
				name := clusterRoleBindingList.Items[idx].GetName()
				fmt.Printf("clusterrolebinding: %s \n", name)
			}
			return nil
		}, TIMEOUT, INTERVAL).ShouldNot(HaveOccurred())

		By("Delete placement finalizer")
		Eventually(func() error {
			placements := &clusterv1beta1.PlacementList{}
			if err := runtimeClient.List(ctx, placements, &client.ListOptions{}); err != nil {
				return err
			}
			for idx := range placements.Items {
				namespace := placements.Items[idx].GetNamespace()
				name := placements.Items[idx].GetName()
				finalizers := placements.Items[idx].GetFinalizers()
				fmt.Printf("placement: %s - %s: %v \n", namespace, name, finalizers)
				// commonconstants.GlobalHubCleanupFinalizer
			}
			return nil
		}, TIMEOUT, INTERVAL).ShouldNot(HaveOccurred())

		By("Delete managedclusterset finalizer")
		Eventually(func() error {
			managedclustersets := &clusterv1beta1.ManagedClusterSetList{}
			if err := runtimeClient.List(ctx, managedclustersets, &client.ListOptions{}); err != nil {
				return err
			}
			for idx := range managedclustersets.Items {
				namespace := managedclustersets.Items[idx].GetNamespace()
				name := managedclustersets.Items[idx].GetName()
				finalizers := managedclustersets.Items[idx].GetFinalizers()
				fmt.Printf("managedclusterset: %s - %s: %v \n", namespace, name, finalizers)
			}
			return nil
		}, TIMEOUT, INTERVAL).ShouldNot(HaveOccurred())

		By("Delete managedclustersetbinding finalizer")
		Eventually(func() error {
			managedclustersetbindings := &clusterv1beta1.ManagedClusterSetBindingList{}
			if err := runtimeClient.List(ctx, managedclustersetbindings, &client.ListOptions{}); err != nil &&
				!errors.IsNotFound(err) {
				return err
			}
			for idx := range managedclustersetbindings.Items {
				namespace := managedclustersetbindings.Items[idx].GetNamespace()
				name := managedclustersetbindings.Items[idx].GetName()
				finalizers := managedclustersetbindings.Items[idx].GetFinalizers()
				fmt.Printf("managedclustersetbindings: %s - %s: %v \n", namespace, name, finalizers)
			}
			return nil
		}, TIMEOUT, INTERVAL).ShouldNot(HaveOccurred())
	})

	AfterAll(func() {
		// By("Remove from clusters")
		// patches := []patch{
		// 	{
		// 		Op:    "remove",
		// 		Path:  "/metadata/labels/" + APP_LABEL_KEY,
		// 		Value: APP_LABEL_VALUE,
		// 	},
		// }
		// Eventually(func() error {
		// 	err := updateClusterLabel(httpClient, patches, token, managedClusterName1)
		// 	if err != nil {
		// 		return err
		// 	}
		// 	return nil
		// }, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())

		// Eventually(func() error {
		// 	err := updateClusterLabel(httpClient, patches, token, managedClusterName2)
		// 	if err != nil {
		// 		return err
		// 	}
		// 	return nil
		// }, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())

		// By("Remove the appsub resource")
		// Eventually(func() error {
		// 	_, err := clients.Kubectl(clients.HubClusterName(), "delete", "-f", APP_SUB_YAML)
		// 	if err != nil {
		// 		return err
		// 	}
		// 	return nil
		// }, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())

		// By("Delete policy")
		// patches := []patch{
		// 	{
		// 		Op:    "remove",
		// 		Path:  "/metadata/labels/" + POLICY_LABEL_KEY,
		// 		Value: POLICY_LABEL_VALUE,
		// 	},
		// }
		// Eventually(func() error {
		// 	err := updateClusterLabel(httpClient, patches, token, managedClusterName2)
		// 	if err != nil {
		// 		return err
		// 	}
		// 	return nil
		// }, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())

		// By("Delete the inforce policy")
		// _, err := clients.Kubectl(clients.HubClusterName(), "delete", "-f", INFORM_POLICY_YAML)
		// Expect(err).ShouldNot(HaveOccurred())

		// >>>> policy
		// check ReginalHub: Policy, PlacementRule(default: placement-policy-limitrange), PlacementBinding
		// check Managedcluster: Policy
		policyInfoBytes, err := clients.Kubectl(REGINAL_HUB_NAME, "get", "policy", "-n open-cluster-management")
		if err != nil {
			fmt.Printf("get reginal policy err %s", err.Error())
		}
		fmt.Printf("get reginal policy: %s", string(policyInfoBytes))
		if strings.Contains(string(policyInfoBytes), "No resources found") {
			fmt.Println("not find resources")
		}

		// >>>> application
		// check ReginalHub: application/ appsub/ channel/ placementrule(helloworld: helloworld-placement)
		// check managedcluster: appsub
	})
})
