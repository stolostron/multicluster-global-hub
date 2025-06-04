package agent

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"

	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

// go test ./test/integration/operator/controllers/agent -ginkgo.focus "deploy default addon" -v
var _ = Describe("deploy default addon", func() {
	It("Should skip to create agent when importing an bare OCP in brownfield(enable local cluster)", func() {
		clusterName := fmt.Sprintf("hub-%s", rand.String(6))

		By("By preparing an OCP Managed Clusters")
		prepareCluster(clusterName,
			map[string]string{"vendor": "OpenShift"}, // without the label of agent-deploy-mode
			map[string]string{},
			[]clusterv1.ManagedClusterClaim{},
			clusterAvailableCondition)

		By("By checking the addon CR is not created in the cluster ns")
		addon := &addonv1alpha1.ManagedClusterAddOn{}
		Eventually(func() error {
			err := runtimeClient.Get(ctx, types.NamespacedName{
				Name:      constants.GHManagedClusterAddonName,
				Namespace: clusterName,
			}, addon)
			if err != nil && errors.IsNotFound(err) {
				return nil
			} else if err != nil {
				return err
			}
			return fmt.Errorf("should not create addon in cluster %s under brownfield mode", clusterName)
		}, timeout, interval).ShouldNot(HaveOccurred())

		// Expect(len(addon.GetAnnotations())).Should(Equal(0))
		// By("By checking the agent manifestworks are created for the newly created managed cluster")
		// work := &workv1.ManifestWork{}
		// workName := fmt.Sprintf("addon-%s-deploy-0",constants.GHManagedClusterAddonName)
		// Eventually(func() error {
		// 	return runtimeClient.Get(ctx, types.NamespacedName{
		// 		Name:      workName,
		// 		Namespace: clusterName,
		// 	}, work)
		// }, timeout, interval).ShouldNot(HaveOccurred())

		// Expect(len(work.Spec.Workload.Manifests)).Should(Equal(8))
	})

	It("Should create default addon with deploy label = Default under brownfield mode", func() {
		clusterName := fmt.Sprintf("hub-%s", rand.String(6))
		workName := fmt.Sprintf("addon-%s-deploy-0",
			constants.GHManagedClusterAddonName)

		By("By preparing clusters")
		prepareCluster(clusterName,
			map[string]string{
				"vendor":                            "OpenShift",
				constants.GHAgentDeployModeLabelKey: constants.GHAgentDeployModeDefault,
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

		Expect(len(addon.GetAnnotations())).Should(Equal(0))

		By("By checking the agent manifestworks are created for the newly created managed cluster")
		work := &workv1.ManifestWork{}
		Eventually(func() error {
			return runtimeClient.Get(ctx, types.NamespacedName{
				Name:      workName,
				Namespace: clusterName,
			}, work)
		}, timeout, interval).ShouldNot(HaveOccurred())

		Expect(len(work.Spec.Workload.Manifests)).Should(Equal(8))
	})

	It("Should create default addon and ACM with deploy label = Default under brownfield mode", func() {
		clusterName := fmt.Sprintf("hub-%s", rand.String(6))
		workName := fmt.Sprintf("addon-%s-deploy-0",
			constants.GHManagedClusterAddonName)

		By("By preparing an OCP Managed Clusters")
		prepareCluster(clusterName,
			map[string]string{
				"vendor": "OpenShift",
				operatorconstants.GHAgentACMHubInstallLabelKey: "", // with label hub-cluster-install
				constants.GHAgentDeployModeLabelKey:            constants.GHAgentDeployModeDefault,
			},
			map[string]string{},
			[]clusterv1.ManagedClusterClaim{
				{
					Name:  constants.HubClusterClaimName,
					Value: constants.HubNotInstalled,
				},
			},
			clusterAvailableCondition)

		By("By checking the addon CR is is created in the cluster ns")
		addon := &addonv1alpha1.ManagedClusterAddOn{}
		Eventually(func() error {
			return runtimeClient.Get(ctx, types.NamespacedName{
				Name:      constants.GHManagedClusterAddonName,
				Namespace: clusterName,
			}, addon)
		}, timeout, interval).ShouldNot(HaveOccurred())

		Expect(len(addon.GetAnnotations())).Should(Equal(0))

		By("By checking the agent manifestworks are created for the newly created managed cluster")
		work := &workv1.ManifestWork{}
		Eventually(func() error {
			return runtimeClient.Get(ctx, types.NamespacedName{
				Name:      workName,
				Namespace: clusterName,
			}, work)
		}, timeout, interval).ShouldNot(HaveOccurred())

		// contains both the ACM and the Global Hub manifests
		Expect(len(work.Spec.Workload.Manifests)).Should(Equal(17))
	})
})
