// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package webhook_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	placementrulesv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	mgrwebhook "github.com/stolostron/multicluster-global-hub/manager/pkg/webhook"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var _ = Describe("Multicluster hub manager webhook", func() {
	var cancel context.CancelFunc
	var c client.Client

	Context("Test Placement and placementrule are handled by the global hub manager webhook", Ordered, func() {
		BeforeAll(func() {
			// add scheme
			err := placementrulesv1.AddToScheme(scheme.Scheme)
			Expect(err).NotTo(HaveOccurred())
			err = clusterv1beta1.AddToScheme(scheme.Scheme)
			Expect(err).NotTo(HaveOccurred())

			m, err := manager.New(testEnv.Config, manager.Options{
				Port:               testEnv.WebhookInstallOptions.LocalServingPort,
				Host:               testEnv.WebhookInstallOptions.LocalServingHost,
				CertDir:            testEnv.WebhookInstallOptions.LocalServingCertDir,
				Scheme:             scheme.Scheme,
				MetricsBindAddress: "0",
			}) // we need manager here just to leverage manager.SetFields
			Expect(err).NotTo(HaveOccurred())

			c, err = client.New(testEnv.Config, client.Options{Scheme: scheme.Scheme})
			Expect(err).NotTo(HaveOccurred())

			server := m.GetWebhookServer()
			server.Register("/mutating", &webhook.Admission{
				Handler: mgrwebhook.NewAdmissionHandler(m.GetClient(), m.GetScheme()),
			})

			ctx, cancel = context.WithCancel(context.Background())
			go func() {
				_ = m.Start(ctx)
			}()
		})
		It("Should add cluster.open-cluster-management.io/experimental-scheduling-disable annotation to placement", func() {
			testPlacement := &clusterv1beta1.Placement{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-placement-",
					Namespace:    utils.GetDefaultNamespace(),
					Labels:       map[string]string{constants.GlobalHubGlobalResourceLabel: ""},
				},
				Spec: clusterv1beta1.PlacementSpec{},
			}

			Eventually(func() bool {
				if err := c.Create(ctx, testPlacement, &client.CreateOptions{}); err != nil {
					return false
				}
				placement := &clusterv1beta1.Placement{}
				if err := c.Get(ctx, client.ObjectKeyFromObject(testPlacement), placement); err != nil {
					return false
				}
				return placement.Annotations[clusterv1beta1.PlacementDisableAnnotation] == "true"
			}, 5*time.Second).Should(BeTrue())
		})

		It("Should not add cluster.open-cluster-management.io/experimental-scheduling-disable annotation to placement", func() {
			testPlacement := &clusterv1beta1.Placement{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-placement-",
					Namespace:    utils.GetDefaultNamespace(),
				},
				Spec: clusterv1beta1.PlacementSpec{},
			}

			Eventually(func() bool {
				if err := c.Create(ctx, testPlacement, &client.CreateOptions{}); err != nil {
					return false
				}
				placement := &clusterv1beta1.Placement{}
				if err := c.Get(ctx, client.ObjectKeyFromObject(testPlacement), placement); err != nil {
					return false
				}
				return placement.Annotations == nil
			}, 1*time.Second, 5*time.Second).Should(BeTrue())
		})

		It("Should set global-hub as scheduler name for the placementrule", func() {
			testPlacementRule := &placementrulesv1.PlacementRule{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-placementrule-",
					Namespace:    utils.GetDefaultNamespace(),
					Labels:       map[string]string{constants.GlobalHubGlobalResourceLabel: ""},
				},
				Spec: placementrulesv1.PlacementRuleSpec{},
			}

			Eventually(func() bool {
				if err := c.Create(ctx, testPlacementRule, &client.CreateOptions{}); err != nil {
					return false
				}
				placementrule := &placementrulesv1.PlacementRule{}
				if err := c.Get(ctx, client.ObjectKeyFromObject(testPlacementRule), placementrule); err != nil {
					return false
				}
				return placementrule.Spec.SchedulerName == constants.GlobalHubSchedulerName
			}, 1*time.Second, 5*time.Second).Should(BeTrue())
		})

		It("Should not set global-hub as scheduler name for the placementrule", func() {
			testPlacementRule := &placementrulesv1.PlacementRule{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-placementrule-",
					Namespace:    utils.GetDefaultNamespace(),
					Labels:       map[string]string{},
				},
				Spec: placementrulesv1.PlacementRuleSpec{},
			}

			Eventually(func() bool {
				if err := c.Create(ctx, testPlacementRule, &client.CreateOptions{}); err != nil {
					return false
				}
				placementrule := &placementrulesv1.PlacementRule{}
				if err := c.Get(ctx, client.ObjectKeyFromObject(testPlacementRule), placementrule); err != nil {
					return false
				}
				return placementrule.Spec.SchedulerName != constants.GlobalHubSchedulerName
			}, 1*time.Second, 5*time.Second).Should(BeTrue())
		})

		AfterAll(func() {
			cancel()
		})
	})
})
