package agent

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"

	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

// go test ./test/integration/operator/controllers/agent -ginkgo.focus "deploy hosted addon" -v
var _ = Describe("deploy hosted addon", func() {
	It("Should create hosted addon in OCP", func() {
		clusterName := fmt.Sprintf("hub-%s", rand.String(6))                // managed hub cluster -> enable local cluster
		hostingClusterName := fmt.Sprintf("hub-hosting-%s", rand.String(6)) // hosting cluster -> global hub
		hostingWorkName := fmt.Sprintf("addon-%s-deploy-hosting-%s-0",
			constants.GHManagedClusterAddonName, clusterName)
		By("By preparing clusters")
		prepareCluster(clusterName,
			map[string]string{
				"vendor": "OpenShift",
				operatorconstants.GHAgentDeployModeLabelKey: operatorconstants.GHAgentDeployModeHosted,
			},
			map[string]string{
				constants.AnnotationClusterDeployMode:                constants.ClusterDeployModeHosted,
				constants.AnnotationClusterKlusterletDeployNamespace: "open-cluster-management-hub1",
				constants.AnnotationClusterHostingClusterName:        hostingClusterName,
			},
			[]clusterv1.ManagedClusterClaim{},
			clusterAvailableCondition)
		prepareCluster(hostingClusterName,
			map[string]string{
				"vendor": "OpenShift",
				operatorconstants.GHAgentDeployModeLabelKey: operatorconstants.GHAgentDeployModeNone,
			},
			map[string]string{},
			[]clusterv1.ManagedClusterClaim{},
			clusterAvailableCondition)
		By("By checking the addon CR is is created in the cluster ns")
		addon := &addonv1alpha1.ManagedClusterAddOn{}
		Eventually(func() error {
			return runtimeClient.Get(ctx, types.NamespacedName{
				Name:      constants.GHManagedClusterAddonName,
				Namespace: clusterName,
			}, addon)
		}, timeout, interval).ShouldNot(HaveOccurred())

		Expect(addon.GetAnnotations()[constants.AnnotationAddonHostingClusterName]).Should(Equal(hostingClusterName))

		By("By checking the agent manifestworks are created for the newly created managed cluster")
		hostingWork := &workv1.ManifestWork{}
		Eventually(func() error {
			return runtimeClient.Get(ctx, types.NamespacedName{
				Name:      hostingWorkName,
				Namespace: hostingClusterName,
			}, hostingWork)
		}, timeout, interval).ShouldNot(HaveOccurred())

		Expect(len(hostingWork.Spec.Workload.Manifests)).Should(Equal(7))
	})

	It("Should create hosted addon and ACM in OCP", func() {
		clusterName := fmt.Sprintf("hub-%s", rand.String(6))
		hostingClusterName := fmt.Sprintf("hub-hosting-%s", rand.String(6))
		workName := fmt.Sprintf("addon-%s-deploy-0",
			constants.GHManagedClusterAddonName)
		hostingWorkName := fmt.Sprintf("addon-%s-deploy-hosting-%s-0",
			constants.GHManagedClusterAddonName, clusterName)
		By("By preparing clusters")
		prepareCluster(clusterName,
			map[string]string{
				"vendor": "OpenShift",
				operatorconstants.GHAgentDeployModeLabelKey:    operatorconstants.GHAgentDeployModeHosted,
				operatorconstants.GHAgentACMHubInstallLabelKey: "",
			},
			map[string]string{
				constants.AnnotationClusterDeployMode:                constants.ClusterDeployModeHosted,
				constants.AnnotationClusterKlusterletDeployNamespace: "open-cluster-management-hub1",
				constants.AnnotationClusterHostingClusterName:        hostingClusterName,
			},
			[]clusterv1.ManagedClusterClaim{
				{
					Name:  constants.HubClusterClaimName,
					Value: constants.HubNotInstalled,
				},
			},
			clusterAvailableCondition)
		prepareCluster(hostingClusterName,
			map[string]string{
				"vendor": "OpenShift",
				operatorconstants.GHAgentDeployModeLabelKey: operatorconstants.GHAgentDeployModeNone,
			},
			map[string]string{},
			[]clusterv1.ManagedClusterClaim{},
			clusterAvailableCondition)
		By("By checking the addon CR is is created in the cluster ns")
		addon := &addonv1alpha1.ManagedClusterAddOn{}
		Eventually(func() error {
			return runtimeClient.Get(ctx, types.NamespacedName{
				Name:      constants.GHManagedClusterAddonName,
				Namespace: clusterName,
			}, addon)
		}, timeout, interval).ShouldNot(HaveOccurred())

		Expect(addon.GetAnnotations()[constants.AnnotationAddonHostingClusterName]).Should(Equal(hostingClusterName))

		By("By checking the agent manifestworks are created for the newly created managed cluster")
		work := &workv1.ManifestWork{}
		Eventually(func() error {
			return runtimeClient.Get(ctx, types.NamespacedName{
				Name:      workName,
				Namespace: clusterName,
			}, work)
		}, timeout, interval).ShouldNot(HaveOccurred())

		Expect(len(work.Spec.Workload.Manifests)).Should(Equal(9))
		hostingWork := &workv1.ManifestWork{}
		Eventually(func() error {
			return runtimeClient.Get(ctx, types.NamespacedName{
				Name:      hostingWorkName,
				Namespace: hostingClusterName,
			}, hostingWork)
		}, timeout, interval).ShouldNot(HaveOccurred())
		Expect(len(hostingWork.Spec.Workload.Manifests)).Should(Equal(7))
	})
})
