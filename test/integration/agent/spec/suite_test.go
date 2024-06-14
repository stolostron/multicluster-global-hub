package spec

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ctrl "sigs.k8s.io/controller-runtime"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/config"
	speccontroller "github.com/stolostron/multicluster-global-hub/agent/pkg/spec/controller"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	genericproducer "github.com/stolostron/multicluster-global-hub/pkg/transport/producer"
)

func TestSyncers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Spec Syncers Suite")
}

var (
	testenv         *envtest.Environment
	leafHubName     string
	agentConfig     *config.AgentConfig
	ctx             context.Context
	cancel          context.CancelFunc
	runtimeClient   runtimeclient.Client
	genericProducer transport.Producer
)

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.Background())

	leafHubName = "spec-hub"
	agentConfig = &config.AgentConfig{
		TransportConfig: &transport.TransportConfig{
			TransportType: string(transport.Chan),
			KafkaConfig: &transport.KafkaConfig{
				Topics: &transport.ClusterTopic{
					SpecTopic: "spec",
				},
				ConsumerConfig: &transport.KafkaConsumerConfig{},
			},
		},
		SpecWorkPoolSize:     2,
		LeafHubName:          leafHubName,
		SpecEnforceHohRbac:   true,
		EnableGlobalResource: true,
	}

	testenv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "..", "pkg", "testdata", "crds"),
		},
	}

	cfg, err := testenv.Start()
	Expect(err).NotTo(HaveOccurred())

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Metrics: metricsserver.Options{
			BindAddress: "0", // disable the metrics serving
		},
		Scheme: config.GetRuntimeScheme(),
	})
	Expect(err).NotTo(HaveOccurred())

	err = speccontroller.AddToManager(mgr, agentConfig)
	Expect(err).NotTo(HaveOccurred())

	go func() {
		Expect(mgr.Start(ctx)).NotTo(HaveOccurred())
	}()

	By("Waiting for the manager to be ready")
	Expect(mgr.GetCache().WaitForCacheSync(ctx)).To(BeTrue())

	runtimeClient = mgr.GetClient()
	genericProducer, err = genericproducer.NewGenericProducer(agentConfig.TransportConfig, "spec")
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	cancel()

	By("Tearing down the test environment")
	err := testenv.Stop()
	// https://github.com/kubernetes-sigs/controller-runtime/issues/1571
	// Set 4 with random
	if err != nil {
		time.Sleep(4 * time.Second)
	}
	Expect(testenv.Stop()).NotTo(HaveOccurred())
})
