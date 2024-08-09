package addon

import (
	"encoding/json"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stolostron/cluster-lifecycle-api/imageregistry/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

// go test ./test/integration/operator/addon -ginkgo.focus "addon registry" -v
var _ = Describe("addon registry", Ordered, func() {
	BeforeAll(func() {
	})
	It("Should update the image pull secret from the mgh cr", func() {
		clusterName := fmt.Sprintf("hub-%s", rand.String(6))
		workName := fmt.Sprintf("addon-%s-deploy-0", constants.GHManagedClusterAddonName)

		By("By preparing an OCP Managed Clusters")
		prepareCluster(clusterName,
			map[string]string{"vendor": "OpenShift"},
			map[string]string{},
			[]clusterv1.ManagedClusterClaim{},
			clusterAvailableCondition)

		By("By checking the addon CR is created in the cluster ns")
		addon := &addonv1alpha1.ManagedClusterAddOn{}
		Eventually(func() error {
			return runtimeClient.Get(ctx, types.NamespacedName{
				Name:      constants.GHManagedClusterAddonName,
				Namespace: clusterName,
			}, addon)
		}, timeout, interval).ShouldNot(HaveOccurred())

		Expect(len(addon.GetAnnotations())).Should(Equal(0))

		By("By checking the mgh image pull secret is created in the cluster's manifestworks")
		Eventually(func() error {
			work := &workv1.ManifestWork{}
			err := runtimeClient.Get(ctx, types.NamespacedName{
				Name:      workName,
				Namespace: clusterName,
			}, work)
			if err != nil {
				return err
			}
			fmt.Println("===", "manifests size", len(work.Spec.Workload.Manifests))
			for _, manifest := range work.Spec.Workload.Manifests {
				unstructuredObj := &unstructured.Unstructured{}
				err := json.Unmarshal(manifest.Raw, unstructuredObj)
				if err != nil {
					return err
				}
				fmt.Println("===", unstructuredObj.GetKind(),
					unstructuredObj.GetName(), unstructuredObj.GetNamespace())
				if unstructuredObj.GetKind() == "Secret" &&
					unstructuredObj.GetName() == mgh.Spec.ImagePullSecret {
					return nil
				}
			}
			return fmt.Errorf("image global hub pull secret is not created")
		}, timeout, interval).ShouldNot(HaveOccurred())
	})

	It("Should update the image registry and pull secret from ManagedClusterImageRegistry", func() {
		clusterName := fmt.Sprintf("hub-%s", rand.String(6))
		workName := fmt.Sprintf("addon-%s-deploy-0",
			constants.GHManagedClusterAddonName)

		By("By preparing the image registry pull secret")
		imageRegistrySecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "image-registry-pull-secret",
				Namespace: "default",
			},
			Data: map[string][]byte{
				".dockerconfigjson": []byte(`{"test":"secret by the managedclusterimageregistries"}`),
			},
			Type: corev1.SecretTypeDockerConfigJson,
		}
		Expect(runtimeClient.Create(ctx, imageRegistrySecret)).Should(Succeed())

		By("By preparing an OCP Managed Clusters")
		prepareCluster(clusterName,
			map[string]string{"vendor": "OpenShift"},
			map[string]string{
				v1alpha1.ClusterImageRegistriesAnnotation: `{"pullSecret":"default.image-registry-pull-secret","registries":[{"mirror":"quay.io/test","source":"quay.io/stolostron"}]}`,
			},
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

		By("By checking the mgh image pull secret is created in the cluster's manifestworks")
		Eventually(func() error {
			work := &workv1.ManifestWork{}
			err := runtimeClient.Get(ctx, types.NamespacedName{
				Name:      workName,
				Namespace: clusterName,
			}, work)
			if err != nil {
				return err
			}
			fmt.Println("+++", "manifests size", len(work.Spec.Workload.Manifests))

			// check registry is updated
			workStr, err := json.Marshal(work)
			if err != nil {
				return err
			}
			if !strings.Contains(string(workStr), "quay.io/test") {
				return fmt.Errorf("the image registry should be replaced with %s", "quay.io/test")
			}

			// pull secret is updated
			for _, manifest := range work.Spec.Workload.Manifests {
				unstructuredObj := &unstructured.Unstructured{}
				err := json.Unmarshal(manifest.Raw, unstructuredObj)
				if err != nil {
					return err
				}
				fmt.Println("+++", unstructuredObj.GetKind(),
					unstructuredObj.GetName(), unstructuredObj.GetNamespace())
				if unstructuredObj.GetKind() == "Secret" &&
					unstructuredObj.GetName() == imageRegistrySecret.GetName() {
					return nil
				}
			}
			return fmt.Errorf("image registry pull secret is not created")
		}, timeout, interval).ShouldNot(HaveOccurred())
	})
})
