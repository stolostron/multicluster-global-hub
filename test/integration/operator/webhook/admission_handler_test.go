// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package webhook_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	addonv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var _ = Describe("Multicluster hub webhook", func() {
	Context("Test managedclusters are handled by the global hub manager webhook", Ordered, func() {
		It("managedcluster should be added the hosted annotations", func() {
			testmanagedcluster := &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-mc-",
					Namespace:    utils.GetDefaultNamespace(),
					Labels: map[string]string{
						// global hub featuregate for klusterlet hosted, this label for addon hosted
						operatorconstants.GHAgentDeployModeLabelKey: operatorconstants.GHAgentDeployModeHosted,
					},
					Annotations: map[string]string{},
				},
			}

			Eventually(func() bool {
				if err := c.Create(ctx, testmanagedcluster, &client.CreateOptions{}); err != nil {
					return false
				}
				mc := &clusterv1.ManagedCluster{}
				if err := c.Get(ctx, client.ObjectKeyFromObject(testmanagedcluster), mc); err != nil {
					return false
				}
				if mc.Annotations[constants.AnnotationClusterDeployMode] != constants.ClusterDeployModeHosted {
					return false
				}
				if mc.Annotations[constants.AnnotationClusterHostingClusterName] != localClusterName {
					return false
				}
				return true
			}, 1*time.Second, 5*time.Second).Should(BeTrue())
		})
	})

	Context("Test klusterletaddonconfig are handled by the global hub manager webhook", Ordered, func() {
		It("klusterletaddonconfig should be added the hosted annotations", func() {
			klusterletConfig := &addonv1.KlusterletAddonConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
				},
				Spec: addonv1.KlusterletAddonConfigSpec{
					ApplicationManagerConfig: addonv1.KlusterletAddonAgentConfigSpec{
						Enabled: true,
					},
					PolicyController: addonv1.KlusterletAddonAgentConfigSpec{
						Enabled: true,
					},
					CertPolicyControllerConfig: addonv1.KlusterletAddonAgentConfigSpec{
						Enabled: true,
					},
				},
			}

			Eventually(func() bool {
				if err := c.Create(ctx, klusterletConfig, &client.CreateOptions{}); err != nil {
					klog.Errorf("Failed to create klusterletAddonConfig, err:%v", err)
					return false
				}
				kac := &addonv1.KlusterletAddonConfig{}
				if err := c.Get(ctx, client.ObjectKeyFromObject(klusterletConfig), kac); err != nil {
					klog.Errorf("Failed to get klusterletAddonConfig, err:%v", err)
					return false
				}
				if kac.Spec.PolicyController.Enabled == true {
					return false
				}
				if kac.Spec.ApplicationManagerConfig.Enabled == true {
					return false
				}
				if kac.Spec.CertPolicyControllerConfig.Enabled == true {
					return false
				}
				return true
			}, 1*time.Second, 5*time.Second).Should(BeTrue())
		})
	})
})
