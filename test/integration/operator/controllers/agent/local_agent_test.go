package agent

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/agent"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

// go test ./test/integration/operator/controllers/agent/local_agent -ginkgo.focus "local agent" -v
var _ = Describe("local agent", func() {
	It("Should create local agent ", func() {
		By("By checking the GH agent is created")
		agentDeployment := &appsv1.Deployment{}
		Eventually(func() error {
			return runtimeClient.Get(ctx, types.NamespacedName{
				Name:      "multicluster-global-hub-agent",
				Namespace: controllerOption.MulticlusterGlobalHub.Namespace,
			}, agentDeployment)
		}, time.Second*10, time.Second*1).ShouldNot(HaveOccurred())

		By("By checking the GH agent serviceaccount is created")
		agentSA := &corev1.ServiceAccount{}
		Eventually(func() error {
			return runtimeClient.Get(ctx, types.NamespacedName{
				Name:      "multicluster-global-hub-agent",
				Namespace: controllerOption.MulticlusterGlobalHub.Namespace,
			}, agentSA)
		}, time.Second*10, time.Second*1).ShouldNot(HaveOccurred())

		By("By checking the GH agent configmap is created")
		agentCM := &corev1.ConfigMap{}
		Eventually(func() error {
			return runtimeClient.Get(ctx, types.NamespacedName{
				Name:      "multicluster-global-hub-agent-config",
				Namespace: controllerOption.MulticlusterGlobalHub.Namespace,
			}, agentCM)
		}, time.Second*10, time.Second*1).ShouldNot(HaveOccurred())

		By("By checking the GH agent clusterrole is created")
		agentClusterRole := &rbacv1.ClusterRole{}
		Eventually(func() error {
			return runtimeClient.Get(ctx, types.NamespacedName{
				Name: "multicluster-global-hub:multicluster-global-hub-agent",
			}, agentClusterRole)
		}, time.Second*10, time.Second*1).ShouldNot(HaveOccurred())

		By("By checking the GH agent clusterrolebinding is created")
		agentClusterRoleBinding := &rbacv1.ClusterRoleBinding{}
		Eventually(func() error {
			return runtimeClient.Get(ctx, types.NamespacedName{
				Name: "multicluster-global-hub:multicluster-global-hub-agent",
			}, agentClusterRoleBinding)
		}, time.Second*10, time.Second*1).ShouldNot(HaveOccurred())

		By("By checking the transport config secret for local-cluster is created")
		transportSecret := &corev1.Secret{}
		Eventually(func() error {
			return runtimeClient.Get(ctx, types.NamespacedName{
				Name:      constants.GHTransportConfigSecret + "-" + constants.LocalClusterName,
				Namespace: controllerOption.MulticlusterGlobalHub.Namespace,
			}, transportSecret)
		}, time.Second*10, time.Second*1).ShouldNot(HaveOccurred())

		By("Removed the clusterrolebinding")
		originClusterRoleBindingId := agentClusterRoleBinding.GetUID()
		Expect(runtimeClient.Delete(ctx, agentClusterRoleBinding)).Should(Succeed())

		By("By checking the GH agent clusterrolebinding is re-created")
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

	It("Should create local agent with new local-cluster name", func() {
		newLocalClusterName := "local-cluster-new"
		err := runtimeClient.Create(ctx, &clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: newLocalClusterName,
				Labels: map[string]string{
					constants.LocalClusterName: "true",
				},
			},
			Spec: clusterv1.ManagedClusterSpec{
				HubAcceptsClient:     true,
				LeaseDurationSeconds: 60,
			},
		})
		Expect(err).ToNot(HaveOccurred())
		By("By checking the GH agent is created")
		agentDeployment := &appsv1.Deployment{}
		Eventually(func() error {
			err := runtimeClient.Get(ctx, types.NamespacedName{
				Name:      "multicluster-global-hub-agent",
				Namespace: controllerOption.MulticlusterGlobalHub.Namespace,
			}, agentDeployment)
			if err != nil {
				return err
			}
			name, err := agent.GetLocalClusterName(ctx, runtimeClient, controllerOption.MulticlusterGlobalHub.Namespace)
			if err != nil {
				return err
			}
			if name != newLocalClusterName {
				return fmt.Errorf("local cluster name is not updated, expected: %s, actual: %s", newLocalClusterName, name)
			}
			return nil
		}, time.Second*30, time.Second*1).ShouldNot(HaveOccurred())

		By("By checking the transport config secret for local-cluster is created")
		transportSecret := &corev1.Secret{}
		Eventually(func() error {
			return runtimeClient.Get(ctx, types.NamespacedName{
				Name:      constants.GHTransportConfigSecret + "-" + newLocalClusterName,
				Namespace: controllerOption.MulticlusterGlobalHub.Namespace,
			}, transportSecret)
		}, time.Second*30, time.Second*1).ShouldNot(HaveOccurred())
	})
	It("Should delete local agent when disable local agent", func() {
		Eventually(func() error {
			existingMGH := &globalhubv1alpha4.MulticlusterGlobalHub{}
			err := runtimeClient.Get(ctx, config.GetMGHNamespacedName(), existingMGH)
			if err != nil {
				return err
			}
			existingMGH.Spec.InstallAgentOnLocal = false
			return runtimeClient.Update(ctx, existingMGH)
		}, time.Second*10, time.Second*1).ShouldNot(HaveOccurred())

		By("By checking the GH agent is deleted")
		agentDeployment := &appsv1.Deployment{}
		Eventually(func() error {
			err := runtimeClient.Get(ctx, types.NamespacedName{
				Name:      "multicluster-global-hub-agent",
				Namespace: controllerOption.MulticlusterGlobalHub.Namespace,
			}, agentDeployment)
			if errors.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("GH agent deployment is not deleted, err:%v", err)
		}, time.Second*10, time.Second*1).ShouldNot(HaveOccurred())

		By("By checking the GH agent serviceaccount is deleted")
		agentSA := &corev1.ServiceAccount{}
		Eventually(func() error {
			err := runtimeClient.Get(ctx, types.NamespacedName{
				Name:      "multicluster-global-hub-agent",
				Namespace: controllerOption.MulticlusterGlobalHub.Namespace,
			}, agentSA)
			if errors.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("GH agent sa is not deleted, err:%v", err)
		}, time.Second*10, time.Second*1).ShouldNot(HaveOccurred())

		By("By checking the GH agent configmap is deleted")
		agentCM := &corev1.ConfigMap{}
		Eventually(func() error {
			err := runtimeClient.Get(ctx, types.NamespacedName{
				Name:      "multicluster-global-hub-agent-config",
				Namespace: controllerOption.MulticlusterGlobalHub.Namespace,
			}, agentCM)
			if errors.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("GH agent configmap is not deleted, err:%v", err)
		}, time.Second*10, time.Second*1).ShouldNot(HaveOccurred())

		By("By checking the GH agent clusterrole is deleted")
		agentClusterRole := &rbacv1.ClusterRole{}
		Eventually(func() error {
			err := runtimeClient.Get(ctx, types.NamespacedName{
				Name: "multicluster-global-hub:multicluster-global-hub-agent",
			}, agentClusterRole)
			if errors.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("GH agent ClusterRole is not deleted, err:%v", err)
		}, time.Second*10, time.Second*1).ShouldNot(HaveOccurred())

		By("By checking the GH agent clusterrolebinding is deleted")
		agentClusterRoleBinding := &rbacv1.ClusterRoleBinding{}
		Eventually(func() error {
			err := runtimeClient.Get(ctx, types.NamespacedName{
				Name: "multicluster-global-hub:multicluster-global-hub-agent",
			}, agentClusterRoleBinding)
			if errors.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("GH agent ClusterRole is not deleted, err:%v", err)
		}, time.Second*10, time.Second*1).ShouldNot(HaveOccurred())
	})
})
