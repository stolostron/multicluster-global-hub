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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	cfg        *rest.Config
	k8sClient  client.Client // You'll be using this client in your tests.
	kubeClient *kubernetes.Clientset
	testEnv    *envtest.Environment
	ctx        context.Context
	cancel     context.CancelFunc
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
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "pkg", "testdata", "crds"),
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
})

var _ = AfterSuite(func() {
	Expect(testEnv.Stop()).NotTo(HaveOccurred())
})

func initializeWebhookInEnvironment() {
	namespacedScopeV1 := admissionv1.NamespacedScope
	failedTypeV1 := admissionv1.Fail
	equivalentTypeV1 := admissionv1.Equivalent
	noSideEffectsV1 := admissionv1.SideEffectClassNone
	webhookPathV1 := "/mutating"
	testEnv.WebhookInstallOptions = envtest.WebhookInstallOptions{
		MutatingWebhooks: []*admissionv1.MutatingWebhookConfiguration{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "multicluster-global-hub-manager",
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "MutatingWebhookConfiguration",
					APIVersion: "admissionregistration.k8s.io/v1",
				},
				Webhooks: []admissionv1.MutatingWebhook{
					{
						Name: "global-hub.open-cluster-management.io",
						Rules: []admissionv1.RuleWithOperations{
							{
								Operations: []admissionv1.OperationType{"CREATE", "UPDATE"},
								Rule: admissionv1.Rule{
									APIGroups:   []string{"apps.open-cluster-management.io"},
									APIVersions: []string{"v1"},
									Resources:   []string{"placementrules"},
									Scope:       &namespacedScopeV1,
								},
							},
							{
								Operations: []admissionv1.OperationType{"CREATE", "UPDATE"},
								Rule: admissionv1.Rule{
									APIGroups:   []string{"cluster.open-cluster-management.io"},
									APIVersions: []string{"v1beta1"},
									Resources:   []string{"placements"},
									Scope:       &namespacedScopeV1,
								},
							},
						},
						FailurePolicy: &failedTypeV1,
						MatchPolicy:   &equivalentTypeV1,
						SideEffects:   &noSideEffectsV1,
						ClientConfig: admissionv1.WebhookClientConfig{
							Service: &admissionv1.ServiceReference{
								Name:      "multicluster-global-hub-manager",
								Namespace: config.GetDefaultNamespace(),
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
