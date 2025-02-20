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

package operator

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/transporter/protocol"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/test/integration/utils/testpostgres"
)

var (
	cfg            *rest.Config
	runtimeClient  client.Client
	runtimeManager ctrl.Manager
	kubeClient     *kubernetes.Clientset
	testPostgres   *testpostgres.TestPostgres
	testEnv        *envtest.Environment
	ctx            context.Context
	cancel         context.CancelFunc
	operatorConfig *config.OperatorConfig

	testNamespace = "default"
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Integration dd Suite")
}

var _ = BeforeSuite(func() {
	Expect(os.Setenv("POD_NAMESPACE", testNamespace)).To(Succeed())
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		ControlPlane: envtest.ControlPlane{},
		CRDInstallOptions: envtest.CRDInstallOptions{
			Paths: []string{
				filepath.Join("..", "..", "..", "operator", "config", "crd", "bases"),
				filepath.Join("..", "..", "manifest", "crd"),
			},
			MaxTime: 1 * time.Minute,
		},
		ErrorIfCRDPathMissing: true,
	}
	config.SetKafkaResourceReady(true)
	config.SetACMResourceReady(true)
	operatorConfig = &config.OperatorConfig{
		GlobalResourceEnabled: false,
	}
	testEnv.ControlPlane.GetAPIServer().Configure().Set("disable-admission-plugins",
		"ServiceAccount,MutatingAdmissionWebhook,ValidatingAdmissionWebhook")

	var err error
	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	// create test postgres
	testPostgres, err = testpostgres.NewTestPostgres()
	Expect(err).NotTo(HaveOccurred())

	// add scheme
	runtimeScheme := config.GetRuntimeScheme()

	runtimeClient, err = client.New(cfg, client.Options{Scheme: runtimeScheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(runtimeClient).NotTo(BeNil())

	leaseDuration := 137 * time.Second
	renewDeadline := 126 * time.Second
	retryPeriod := 16 * time.Second
	runtimeManager, err = ctrl.NewManager(cfg, ctrl.Options{
		Metrics: metricsserver.Options{
			BindAddress: "0", // disable the metrics serving
		}, Scheme: runtimeScheme,
		LeaderElection:          true,
		LeaderElectionNamespace: testNamespace,
		LeaderElectionID:        "549a8919.open-cluster-management.io",
		LeaseDuration:           &leaseDuration,
		RenewDeadline:           &renewDeadline,
		RetryPeriod:             &retryPeriod,
	})
	Expect(err).ToNot(HaveOccurred())

	kubeClient, err = kubernetes.NewForConfig(runtimeManager.GetConfig())
	Expect(err).ToNot(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		err = runtimeManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()

	Expect(runtimeManager.GetCache().WaitForCacheSync(ctx)).To(BeTrue())
})

var _ = AfterSuite(func() {
	cancel()
	Expect(testPostgres.Stop()).To(Succeed())
	By("tearing down the test environment")
	err := testEnv.Stop()
	// https://github.com/kubernetes-sigs/controller-runtime/issues/1571
	// Set 4 with random
	if err != nil {
		time.Sleep(4 * time.Second)
		Expect(testEnv.Stop()).To(Succeed())
	}
})

func CreateTestSecretTransport(c client.Client, namespace string) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.GHTransportSecretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"bootstrap_server": []byte("localhost:test"),
			"ca.crt":           []byte("ca.crt"),
			"client.crt":       []byte("client.crt"),
			"client.key":       []byte("client.key"),
		},
		Type: corev1.SecretTypeOpaque,
	}
	err := c.Create(context.Background(), secret)
	if err != nil {
		return err
	}
	trans := protocol.NewBYOTransporter(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      constants.GHTransportSecretName,
	}, runtimeClient)
	config.SetTransporter(trans)
	conn, err := trans.GetConnCredential("")
	if err != nil {
		return err
	}
	config.SetTransporterConn(conn)
	_, _ = trans.EnsureTopic("")
	return nil
}
