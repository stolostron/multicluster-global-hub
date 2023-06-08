package tests

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	clusterv1beta2 "open-cluster-management.io/api/cluster/v1beta2"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	chnv1 "open-cluster-management.io/multicloud-operators-channel/pkg/apis/apps/v1"
	placementrulesv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	appsubv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1"
	appsv1alpha1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1alpha1"
	applicationv1beta1 "sigs.k8s.io/application/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	mghv1alpha3 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha3"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

const (
	TIMEOUT          = 2 * time.Minute
	INTERVAL         = 5 * time.Second
	REGINAL_HUB_NAME = "kind-hub1"
)

var _ = Describe("Delete the multiclusterglobalhub and prune resources", Label("e2e-tests-prune"), Ordered, func() {
	ctx := context.Background()
	var runtimeClient client.Client
	var managedClusterName1 string
	var managedClusterName2 string
	var managedClusterUID1 string
	var managedClusterUID2 string

	BeforeAll(func() {
		By("Get the runtimeClient client")
		scheme := runtime.NewScheme()
		appsv1alpha1.AddToScheme(scheme)
		rbacv1.AddToScheme(scheme)
		clusterv1beta1.AddToScheme(scheme)
		clusterv1beta2.AddToScheme(scheme)
		policiesv1.AddToScheme(scheme)
		clusterv1.AddToScheme(scheme)
		corev1.AddToScheme(scheme)
		applicationv1beta1.AddToScheme(scheme)
		appsubv1.SchemeBuilder.AddToScheme(scheme)
		chnv1.AddToScheme(scheme)
		placementrulesv1.AddToScheme(scheme)
		mghv1alpha3.AddToScheme(scheme)
		var err error
		runtimeClient, err = clients.ControllerRuntimeClient(GlobalHubName, scheme)
		Expect(err).ShouldNot(HaveOccurred())

		By("Get managed cluster name")
		Eventually(func() error {
			managedClusters, err := getManagedCluster(httpClient, httpToken)
			if err != nil {
				return err
			}
			managedClusterName1 = managedClusters[0].Name
			managedClusterName2 = managedClusters[1].Name
			managedClusterUID1 = string(managedClusters[0].GetUID())
			managedClusterUID2 = string(managedClusters[1].GetUID())
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
			if err := updateClusterLabel(httpClient, patches, httpToken, managedClusterUID1); err != nil {
				return err
			}
			return nil
		}, TIMEOUT, INTERVAL).ShouldNot(HaveOccurred())

		By("Apply the appsub to labeled cluster")
		Eventually(func() error {
			_, err := clients.Kubectl(GlobalHubName, "apply", "-f", APP_SUB_YAML)
			if err != nil {
				return err
			}
			return nil
		}, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())

		By("Check the appsub is applied to the cluster")
		Eventually(func() error {
			return checkAppsubreport(runtimeClient, httpClient, APP_SUB_NAME, APP_SUB_NAMESPACE, httpToken, 1,
				[]string{managedClusterName1})
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
			err := updateClusterLabel(httpClient, patches, httpToken, managedClusterUID2)
			if err != nil {
				return err
			}
			return nil
		}, TIMEOUT, INTERVAL).ShouldNot(HaveOccurred())

		By("Apply the policy to labeled cluster")
		Eventually(func() error {
			_, err := clients.Kubectl(GlobalHubName, "apply", "-f", INFORM_POLICY_YAML)
			if err != nil {
				return err
			}
			return nil
		}, TIMEOUT, INTERVAL).ShouldNot(HaveOccurred())

		Eventually(func() error {
			status, err := getPolicyStatus(runtimeClient, httpClient, POLICY_NAME, POLICY_NAMESPACE, httpToken)
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
		mgh := &mghv1alpha3.MulticlusterGlobalHub{}
		err := runtimeClient.Get(ctx, types.NamespacedName{
			Namespace: "open-cluster-management",
			Name:      "multiclusterglobalhub",
		}, mgh)
		Expect(err).NotTo(HaveOccurred())

		By("Delete multiclusterglobalhub")
		err = runtimeClient.Delete(ctx, mgh, &client.DeleteOptions{})
		Expect(err).NotTo(HaveOccurred())

		By("Check whether multiclusterglobalhub is deleted")
		Eventually(func() error {
			err = runtimeClient.Get(ctx, types.NamespacedName{
				Namespace: "open-cluster-management",
				Name:      "multiclusterglobalhub",
			}, mgh)
			if errors.IsNotFound(err) {
				return nil
			}
			if err == nil {
				return fmt.Errorf("multiclusterglobalhub should be deleted")
			}
			return err
		}, TIMEOUT, INTERVAL).ShouldNot(HaveOccurred())
	})

	It("prune the global resources", func() {
		listOpts := []client.ListOption{
			client.MatchingLabels(map[string]string{
				constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
			}),
		}

		By("Delete clusterrole")
		Eventually(func() error {
			clusterRoleList := &rbacv1.ClusterRoleList{}
			if err := runtimeClient.List(ctx, clusterRoleList, listOpts...); err != nil {
				return err
			}
			if len(clusterRoleList.Items) > 0 {
				return fmt.Errorf("clusterroles has not been deleted")
			}
			return nil
		}, TIMEOUT, INTERVAL).ShouldNot(HaveOccurred())

		By("Delete clusterrolebinding")
		Eventually(func() error {
			clusterRoleBindingList := &rbacv1.ClusterRoleBindingList{}
			if err := runtimeClient.List(ctx, clusterRoleBindingList, listOpts...); err != nil {
				return err
			}
			if len(clusterRoleBindingList.Items) > 0 {
				return fmt.Errorf("clusterrolebindings has not been deleted")
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
				if controllerutil.ContainsFinalizer(&placements.Items[idx],
					constants.GlobalHubCleanupFinalizer) {
					return fmt.Errorf("finalizer hasn't be deleted from obj: %s", placements.Items[idx].GetName())
				}
			}
			return nil
		}, TIMEOUT, INTERVAL).ShouldNot(HaveOccurred())

		By("Delete managedclusterset finalizer")
		Eventually(func() error {
			managedclustersets := &clusterv1beta2.ManagedClusterSetList{}
			if err := runtimeClient.List(ctx, managedclustersets, &client.ListOptions{}); err != nil {
				return err
			}
			for idx := range managedclustersets.Items {
				if controllerutil.ContainsFinalizer(&managedclustersets.Items[idx],
					constants.GlobalHubCleanupFinalizer) {
					return fmt.Errorf("finalizer hasn't be deleted from obj: %s",
						managedclustersets.Items[idx].GetName())
				}
			}
			return nil
		}, TIMEOUT, INTERVAL).ShouldNot(HaveOccurred())

		By("Delete managedclustersetbinding finalizer")
		Eventually(func() error {
			managedclustersetbindings := &clusterv1beta2.ManagedClusterSetBindingList{}
			if err := runtimeClient.List(ctx, managedclustersetbindings, &client.ListOptions{}); err != nil &&
				!errors.IsNotFound(err) {
				return err
			}
			for idx := range managedclustersetbindings.Items {
				if controllerutil.ContainsFinalizer(&managedclustersetbindings.Items[idx],
					constants.GlobalHubCleanupFinalizer) {
					return fmt.Errorf("finalizer hasn't be deleted from obj: %s",
						managedclustersetbindings.Items[idx].GetName())
				}
			}
			return nil
		}, TIMEOUT, INTERVAL).ShouldNot(HaveOccurred())
	})

	It("prune the namespaced resources", func() {
		By("Delete the mgh configmap")
		Eventually(func() error {
			existingMghConfigMap := &corev1.ConfigMap{}
			err := runtimeClient.Get(ctx, types.NamespacedName{
				Namespace: constants.GHSystemNamespace,
				Name:      constants.GHConfigCMName,
			}, existingMghConfigMap)
			if errors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			return fmt.Errorf("configmap should be deleted: %s - %s",
				constants.GHSystemNamespace, constants.GHConfigCMName)
		}, TIMEOUT, INTERVAL).ShouldNot(HaveOccurred())

		By("Delete the mgh configmap namespace")
		Eventually(func() error {
			err := runtimeClient.Get(ctx, types.NamespacedName{
				Name: constants.GHSystemNamespace,
			}, &corev1.Namespace{})
			if errors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			return fmt.Errorf("namespace should be deleted: %s", constants.GHSystemNamespace)
		}, TIMEOUT, INTERVAL).ShouldNot(HaveOccurred())
	})

	It("prune application", func() {
		By("Delete the application finalizer")
		Eventually(func() error {
			applications := &applicationv1beta1.ApplicationList{}
			if err := runtimeClient.List(ctx, applications, &client.ListOptions{}); err != nil {
				return err
			}
			for idx := range applications.Items {
				if controllerutil.ContainsFinalizer(&applications.Items[idx],
					constants.GlobalHubCleanupFinalizer) {
					return fmt.Errorf("finalizer hasn't be deleted from app: %s", applications.Items[idx].GetName())
				}
			}
			return nil
		}, TIMEOUT, INTERVAL).ShouldNot(HaveOccurred())

		By("Delete the appsub finalizer")
		Eventually(func() error {
			appsubs := &appsubv1.SubscriptionList{}
			if err := runtimeClient.List(ctx, appsubs, &client.ListOptions{}); err != nil {
				return err
			}
			for idx := range appsubs.Items {
				if controllerutil.ContainsFinalizer(&appsubs.Items[idx],
					constants.GlobalHubCleanupFinalizer) {
					return fmt.Errorf("finalizer hasn't be deleted from appsub: %s", appsubs.Items[idx].GetName())
				}
			}
			return nil
		}, TIMEOUT, INTERVAL).ShouldNot(HaveOccurred())

		By("Delete the channel finalizer")
		Eventually(func() error {
			channels := &chnv1.ChannelList{}
			if err := runtimeClient.List(ctx, channels, &client.ListOptions{}); err != nil {
				return err
			}
			for idx := range channels.Items {
				if controllerutil.ContainsFinalizer(&channels.Items[idx],
					constants.GlobalHubCleanupFinalizer) {
					return fmt.Errorf("finalizer hasn't be deleted from channels obj: %s", channels.Items[idx].GetName())
				}
			}
			return nil
		}, TIMEOUT, INTERVAL).ShouldNot(HaveOccurred())

		By("Delete the palcementrules(policy and app) finalizer")
		Eventually(func() error {
			palcementrules := &placementrulesv1.PlacementRuleList{}
			if err := runtimeClient.List(ctx, palcementrules, &client.ListOptions{}); err != nil && errors.IsNotFound(err) {
				return err
			}
			for idx := range palcementrules.Items {
				if controllerutil.ContainsFinalizer(&palcementrules.Items[idx],
					constants.GlobalHubCleanupFinalizer) {
					return fmt.Errorf("finalizer hasn't be deleted from palcementrules obj: %s",
						palcementrules.Items[idx].GetName())
				}
			}
			return nil
		}, TIMEOUT, INTERVAL).ShouldNot(HaveOccurred())
	})

	It("prune policy", func() {
		By("Delete the policies finalizer")
		Eventually(func() error {
			policies := &policiesv1.PolicyList{}
			if err := runtimeClient.List(ctx, policies, &client.ListOptions{}); err != nil {
				return err
			}
			for idx := range policies.Items {
				if controllerutil.ContainsFinalizer(&policies.Items[idx],
					constants.GlobalHubCleanupFinalizer) {
					return fmt.Errorf("finalizer hasn't be deleted from policies obj: %s", policies.Items[idx].GetName())
				}
			}
			return nil
		}, TIMEOUT, INTERVAL).ShouldNot(HaveOccurred())

		By("Delete the placementbindings finalizer")
		Eventually(func() error {
			placementbindings := &policiesv1.PlacementBindingList{}
			if err := runtimeClient.List(ctx, placementbindings, &client.ListOptions{}); err != nil {
				return err
			}
			for idx := range placementbindings.Items {
				if controllerutil.ContainsFinalizer(&placementbindings.Items[idx],
					constants.GlobalHubCleanupFinalizer) {
					return fmt.Errorf("finalizer hasn't be deleted from placementbindings obj: %s",
						placementbindings.Items[idx].GetName())
				}
			}
			return nil
		}, TIMEOUT, INTERVAL).ShouldNot(HaveOccurred())
	})
})
