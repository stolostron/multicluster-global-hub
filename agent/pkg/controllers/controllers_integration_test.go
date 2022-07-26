// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controllers_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/hub-of-hubs/agent/pkg/controllers"
)

var _ = Describe("controller", Ordered, func() {

	ctx, cancel := context.WithCancel(context.Background())
	var mgr ctrl.Manager

	BeforeEach(func() {
		Expect(cfg).NotTo(BeNil())
	})

	It("create a manager", func() {
		By("Creating the Manager")
		var err error
		mgr, err = ctrl.NewManager(cfg, ctrl.Options{})
		Expect(err).NotTo(HaveOccurred())

		By("Adding the controllers to the manager")
		Expect(controllers.AddControllers(mgr)).NotTo(HaveOccurred())

		go func() {
			defer GinkgoRecover()
			Expect(mgr.Start(ctx)).NotTo(HaveOccurred())
		}()

		By("Waiting for the manager to be ready")
		Expect(mgr.GetCache().WaitForCacheSync(ctx)).To(BeTrue())

		By("Executing the PostStart function")
		Expect(controllers.PostStart(mgr)).NotTo(HaveOccurred())
	})

	It("clusterrole testing", func() {
		By("Having the ClusterRole created")
		clusterrole := &rbacv1.ClusterRole{}
		Expect(mgr.GetClient().Get(ctx, client.ObjectKey{Name: controllers.HubOfHubsClusterRoleName},
			clusterrole)).NotTo(HaveOccurred())
		Expect(clusterrole.GetName()).To(Equal(controllers.HubOfHubsClusterRoleName))

		By("Expect the ClusterRole re-created")
		Expect(mgr.GetClient().Delete(ctx, clusterrole)).NotTo(HaveOccurred())
		time.Sleep(1 * time.Second)
		newClusterrole := &rbacv1.ClusterRole{}
		Expect(mgr.GetClient().Get(ctx, client.ObjectKey{Name: controllers.HubOfHubsClusterRoleName},
			newClusterrole)).NotTo(HaveOccurred())
		Expect(newClusterrole.GetName()).To(Equal(controllers.HubOfHubsClusterRoleName))
		Expect(clusterrole.GetResourceVersion()).NotTo(Equal(newClusterrole.GetResourceVersion()))

		By("Expect the ClusterRole no change")
		newClusterrole.Rules = append(newClusterrole.Rules, rbacv1.PolicyRule{
			Resources: []string{
				"configmaps",
			},
			Verbs: []string{
				"create",
			},
			APIGroups: []string{""},
		})
		Expect(mgr.GetClient().Update(ctx, newClusterrole)).NotTo(HaveOccurred())
		time.Sleep(1 * time.Second)
		finalClusterrole := &rbacv1.ClusterRole{}
		Expect(mgr.GetClient().Get(ctx, client.ObjectKey{Name: controllers.HubOfHubsClusterRoleName},
			finalClusterrole)).NotTo(HaveOccurred())
		Expect(equality.Semantic.DeepEqual(clusterrole.Rules, finalClusterrole.Rules)).To(BeTrue())
		Expect(finalClusterrole.GetResourceVersion()).NotTo(Equal(newClusterrole.GetResourceVersion()))
	})

	It("clusterrolebinding testing", func() {

		By("Having the ClusterRoleBing created")
		clusterrolebinding := &rbacv1.ClusterRoleBinding{}
		Expect(mgr.GetClient().Get(ctx, client.ObjectKey{Name: controllers.HubOfHubsClusterRoleName},
			clusterrolebinding)).NotTo(HaveOccurred())
		Expect(clusterrolebinding.GetName()).To(Equal(controllers.HubOfHubsClusterRoleName))

		By("Expect the ClusterRoleBing re-created")
		Expect(mgr.GetClient().Delete(ctx, clusterrolebinding)).NotTo(HaveOccurred())
		time.Sleep(1 * time.Second)
		newClusterroleBinding := &rbacv1.ClusterRoleBinding{}
		Expect(mgr.GetClient().Get(ctx, client.ObjectKey{Name: controllers.HubOfHubsClusterRoleName},
			newClusterroleBinding)).NotTo(HaveOccurred())
		Expect(newClusterroleBinding.GetName()).To(Equal(controllers.HubOfHubsClusterRoleName))
		Expect(clusterrolebinding.GetResourceVersion()).NotTo(Equal(newClusterroleBinding.GetResourceVersion()))

		By("Expect the ClusterRoleBinding no change")
		newClusterroleBinding.Subjects = append(newClusterroleBinding.Subjects, rbacv1.Subject{
			Kind:      "ServiceAccount",
			Name:      "klusterlet-sa",
			Namespace: "open-cluster-management-agent",
		})
		Expect(mgr.GetClient().Update(ctx, newClusterroleBinding)).NotTo(HaveOccurred())
		time.Sleep(1 * time.Second)
		finalClusterroleBinding := &rbacv1.ClusterRoleBinding{}
		Expect(mgr.GetClient().Get(ctx, client.ObjectKey{Name: controllers.HubOfHubsClusterRoleName},
			finalClusterroleBinding)).NotTo(HaveOccurred())
		Expect(equality.Semantic.DeepEqual(clusterrolebinding.Subjects, finalClusterroleBinding.Subjects)).To(BeTrue())
		Expect(finalClusterroleBinding.GetResourceVersion()).NotTo(Equal(newClusterroleBinding.GetResourceVersion()))

	})

	AfterAll(func() {
		defer cancel()
	})

})
