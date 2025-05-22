/*
Copyright 2022.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package webhook_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	addonapi "github.com/stolostron/klusterlet-addon-controller/pkg/apis"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	mgrwebhook "github.com/stolostron/multicluster-global-hub/operator/pkg/webhook"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	cfg              *rest.Config
	testEnv          *envtest.Environment
	ctx              context.Context
	cancel           context.CancelFunc
	c                client.Client
	localClusterName = "renamed-local-cluster"
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Integration Suite")
}

var _ = BeforeSuite(func() {
	Expect(os.Setenv("POD_NAMESPACE", "default")).To(Succeed())
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDInstallOptions: envtest.CRDInstallOptions{
			Paths: []string{
				filepath.Join("..", "..", "..", "manifest", "crd"),
			},
			MaxTime: 1 * time.Minute,
		},
		ErrorIfCRDPathMissing: true,
	}

	// we're initializing webhook here and not in webhook.go to also test the envtest install code via WebhookOptions
	initializeWebhookInEnvironment()

	var err error
	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	// add scheme
	err = clusterv1beta1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = addonapi.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	clusterv1.AddToScheme(scheme.Scheme)

	m, err := manager.New(testEnv.Config, manager.Options{
		WebhookServer: webhook.NewServer(webhook.Options{
			Host:    testEnv.WebhookInstallOptions.LocalServingHost,
			Port:    testEnv.WebhookInstallOptions.LocalServingPort,
			CertDir: testEnv.WebhookInstallOptions.LocalServingCertDir,
		}),
		Scheme: scheme.Scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0", // disable the metrics serving
		},
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

	// create the renamed local cluster
	renamedLocalcluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: localClusterName,
			Labels: map[string]string{
				constants.LocalClusterName: "true",
			},
		},
	}
	err = c.Create(ctx, renamedLocalcluster, &client.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())

	// set the hosted configuration into true: the webhook is only takes effect when the global hub enable hosted mode
	mgh := &v1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test"},
		Spec: v1alpha4.MulticlusterGlobalHubSpec{
			FeatureGates: []v1alpha4.FeatureGate{
				{
					Feature: v1alpha4.FeatureGateImportClusterInHosted,
					Mode:    v1alpha4.FeatureGateModeTypeEnable,
				},
			},
		},
	}
	config.SetImportClusterInHosted(mgh)
})

var _ = AfterSuite(func() {
	err := c.Delete(ctx, &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: localClusterName,
		},
	})
	Expect(err).NotTo(HaveOccurred())

	cancel()
	Expect(testEnv.Stop()).NotTo(HaveOccurred())
})

func initializeWebhookInEnvironment() {
	namespacedScopeV1 := admissionv1.NamespacedScope
	clusterScope := admissionv1.ClusterScope
	failedTypeV1 := admissionv1.Fail
	equivalentTypeV1 := admissionv1.Equivalent
	noSideEffectsV1 := admissionv1.SideEffectClassNone
	webhookPathV1 := "/mutating"
	testEnv.WebhookInstallOptions = envtest.WebhookInstallOptions{
		MutatingWebhooks: []*admissionv1.MutatingWebhookConfiguration{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "multicluster-global-hub-mutating",
				},
				Webhooks: []admissionv1.MutatingWebhook{
					{
						Name: "global-hub.open-cluster-management.io",
						Rules: []admissionv1.RuleWithOperations{
							{
								Operations: []admissionv1.OperationType{"CREATE", "UPDATE"},
								Rule: admissionv1.Rule{
									APIGroups:   []string{"agent.open-cluster-management.io"},
									APIVersions: []string{"v1"},
									Resources:   []string{"klusterletaddonconfigs"},
									Scope:       &namespacedScopeV1,
								},
							},
							{
								Operations: []admissionv1.OperationType{"CREATE", "UPDATE"},
								Rule: admissionv1.Rule{
									APIGroups:   []string{"cluster.open-cluster-management.io"},
									APIVersions: []string{"v1"},
									Resources:   []string{"managedclusters"},
									Scope:       &clusterScope,
								},
							},
						},
						FailurePolicy: &failedTypeV1,
						MatchPolicy:   &equivalentTypeV1,
						SideEffects:   &noSideEffectsV1,
						ClientConfig: admissionv1.WebhookClientConfig{
							Service: &admissionv1.ServiceReference{
								Name:      "multicluster-global-hub-mutator",
								Namespace: utils.GetDefaultNamespace(),
								Path:      &webhookPathV1,
							},
						},
						AdmissionReviewVersions: []string{"v1", "v1beta1"},
					},
				},
			},
		},
	}
}
