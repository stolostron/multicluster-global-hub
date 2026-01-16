/*
Copyright 2023.
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

package backup_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	postgresv1beta1 "github.com/crunchydata/postgres-operator/pkg/apis/postgres-operator.crunchydata.com/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	routev1 "github.com/openshift/api/route/v1"
	subv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	clusterv1beta2 "open-cluster-management.io/api/cluster/v1beta2"
	workv1 "open-cluster-management.io/api/work/v1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/backup"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	cfg           *rest.Config
	k8sClient     client.Client // You'll be using this client in your tests.
	kubeClient    *kubernetes.Clientset
	testEnv       *envtest.Environment
	ctx           context.Context
	cancel        context.CancelFunc
	testNamespace = "default"
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Integration Suite")
}

var _ = BeforeSuite(func() {
	Expect(os.Setenv("POD_NAMESPACE", testNamespace)).To(Succeed())
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDInstallOptions: envtest.CRDInstallOptions{
			Paths: []string{
				filepath.Join("..", "..", "..", "config", "crd", "bases"),
				filepath.Join("..", "..", "..", "..", "test", "manifest", "crd"),
			},
			MaxTime: 1 * time.Minute,
		},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	// add scheme
	Expect(routev1.AddToScheme(scheme.Scheme)).NotTo(HaveOccurred())
	Expect(clusterv1.AddToScheme(scheme.Scheme)).NotTo(HaveOccurred())
	Expect(clusterv1beta1.AddToScheme(scheme.Scheme)).NotTo(HaveOccurred())
	Expect(clusterv1beta2.AddToScheme(scheme.Scheme)).NotTo(HaveOccurred())
	Expect(workv1.AddToScheme(scheme.Scheme)).NotTo(HaveOccurred())
	Expect(addonv1alpha1.AddToScheme(scheme.Scheme)).NotTo(HaveOccurred())
	Expect(globalhubv1alpha4.AddToScheme(scheme.Scheme)).NotTo(HaveOccurred())
	Expect(policyv1.AddToScheme(scheme.Scheme)).NotTo(HaveOccurred())
	Expect(mchv1.AddToScheme(scheme.Scheme)).NotTo(HaveOccurred())
	Expect(promv1.AddToScheme(scheme.Scheme)).NotTo(HaveOccurred())
	Expect(subv1alpha1.AddToScheme(scheme.Scheme)).NotTo(HaveOccurred())
	Expect(postgresv1beta1.AddToScheme(scheme.Scheme)).NotTo(HaveOccurred())
	Expect(kafkav1beta2.AddToScheme(scheme.Scheme)).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	leaseDuration := 137 * time.Second
	renewDeadline := 126 * time.Second
	retryPeriod := 16 * time.Second
	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Metrics: metricsserver.Options{
			BindAddress: "0", // disable the metrics serving
		}, Scheme: scheme.Scheme,
		LeaderElection:          true,
		LeaderElectionNamespace: testNamespace,
		LeaderElectionID:        "549a8919.open-cluster-management.io",
		LeaseDuration:           &leaseDuration,
		RenewDeadline:           &renewDeadline,
		RetryPeriod:             &retryPeriod,
	})
	Expect(err).ToNot(HaveOccurred())

	kubeClient, err = kubernetes.NewForConfig(k8sManager.GetConfig())
	Expect(err).ToNot(HaveOccurred())
	option := config.ControllerOption{
		Manager: k8sManager,
	}
	config.SetACMResourceReady(true)
	_, err = backup.StartController(option)
	Expect(err).ToNot(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()
	Expect(k8sManager.GetCache().WaitForCacheSync(ctx)).To(BeTrue())
})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")
	err := testEnv.Stop()
	// https://github.com/kubernetes-sigs/controller-runtime/issues/1571
	// Set 4 with random
	if err != nil {
		time.Sleep(4 * time.Second)
	}
	err = testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
