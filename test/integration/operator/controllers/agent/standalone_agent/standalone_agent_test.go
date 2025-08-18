package agent

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	globalhubv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha1"
)

// go test ./test/integration/operator/controllers/agent/standalone_agent -ginkgo.focus "standalone agent" -v
var _ = Describe("standalone agent", func() {
	It("Should create standalone agent in the default namespace", func() {
		By("Creating multiclusterglobalhubagent CR")
		mgha := &globalhubv1alpha1.MulticlusterGlobalHubAgent{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "multiclusterglobalhubagent",
				Namespace: "default",
			},
			Spec: globalhubv1alpha1.MulticlusterGlobalHubAgentSpec{
				ImagePullSecret:           "test-pull-secret",
				TransportConfigSecretName: "transport-secret",
			},
		}
		Expect(runtimeClient.Create(ctx, mgha)).Should(Succeed())

		By("Creating OpenShift Infrastructure CR")
		infra := &configv1.Infrastructure{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cluster",
			},
		}
		Expect(runtimeClient.Create(ctx, infra)).Should(Succeed())

		By("By checking the GH agent is created in default namespace")
		agentDeployment := &appsv1.Deployment{}
		Eventually(func() error {
			return runtimeClient.Get(ctx, types.NamespacedName{
				Name:      "multicluster-global-hub-agent",
				Namespace: "default",
			}, agentDeployment)
		}, time.Second*10, time.Second*1).ShouldNot(HaveOccurred())

		By("By checking the GH agent serviceaccount is created in default namespace")
		agentSA := &corev1.ServiceAccount{}
		Eventually(func() error {
			return runtimeClient.Get(ctx, types.NamespacedName{
				Name:      "multicluster-global-hub-agent",
				Namespace: "default",
			}, agentSA)
		}, time.Second*10, time.Second*1).ShouldNot(HaveOccurred())

		By("By checking the GH agent configmap is created in default namespace")
		agentCM := &corev1.ConfigMap{}
		Eventually(func() error {
			return runtimeClient.Get(ctx, types.NamespacedName{
				Name:      "multicluster-global-hub-agent-config",
				Namespace: "default",
			}, agentCM)
		}, time.Second*10, time.Second*1).ShouldNot(HaveOccurred())

		By("By checking the GH agent clusterrole is created in default namespace")
		agentClusterRole := &rbacv1.ClusterRole{}
		Eventually(func() error {
			return runtimeClient.Get(ctx, types.NamespacedName{
				Name: "multicluster-global-hub:multicluster-global-hub-agent",
			}, agentClusterRole)
		}, time.Second*10, time.Second*1).ShouldNot(HaveOccurred())

		By("By checking the GH agent clusterrolebinding is created in default namespace")
		agentClusterRoleBinding := &rbacv1.ClusterRoleBinding{}
		Eventually(func() error {
			return runtimeClient.Get(ctx, types.NamespacedName{
				Name: "multicluster-global-hub:multicluster-global-hub-agent",
			}, agentClusterRoleBinding)
		}, time.Second*10, time.Second*1).ShouldNot(HaveOccurred())

		By("Removed the clusterrolebinding")
		originClusterRoleBindingId := agentClusterRoleBinding.GetUID()
		Expect(runtimeClient.Delete(ctx, agentClusterRoleBinding)).Should(Succeed())

		By("By checking the GH agent clusterrolebinding is re-created in default namespace")
		agentClusterRoleBinding = &rbacv1.ClusterRoleBinding{}
		Eventually(func() bool {
			err := runtimeClient.Get(ctx, types.NamespacedName{
				Name: "multicluster-global-hub:multicluster-global-hub-agent",
			}, agentClusterRoleBinding)
			if err != nil {
				return false
			}
			return agentClusterRoleBinding.GetUID() != originClusterRoleBindingId
		}, time.Second*10, time.Second*1).Should(BeTrue())
	})
})
