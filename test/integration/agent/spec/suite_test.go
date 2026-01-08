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

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	speccontroller "github.com/stolostron/multicluster-global-hub/agent/pkg/spec"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	genericconsumer "github.com/stolostron/multicluster-global-hub/pkg/transport/consumer"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/controller"
	genericproducer "github.com/stolostron/multicluster-global-hub/pkg/transport/producer"
)

func TestSyncers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Spec Syncers Suite")
}

var (
	testenv         *envtest.Environment
	leafHubName     string
	agentConfig     *configs.AgentConfig
	ctx             context.Context
	cancel          context.CancelFunc
	runtimeClient   runtimeclient.Client
	genericProducer transport.Producer
	genericConsumer transport.Consumer
)

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.Background())

	leafHubName = "spec-hub"
	agentConfig = &configs.AgentConfig{
		TransportConfig: &transport.TransportInternalConfig{
			TransportType: string(transport.Chan),
			KafkaCredential: &transport.KafkaConfig{
				SpecTopic:       "spec",
				StatusTopic:     "status",
				ConsumerGroupID: "agent",
			},
		},
		SpecWorkPoolSize:     2,
		LeafHubName:          leafHubName,
		SpecEnforceHohRbac:   true,
		EnableGlobalResource: true,
	}
	configs.SetAgentConfig(agentConfig)

	testenv = &envtest.Environment{
		CRDInstallOptions: envtest.CRDInstallOptions{
			Paths: []string{
				filepath.Join("..", "..", "..", "manifest", "crd"),
			},
			MaxTime: 1 * time.Minute,
		},
	}

	cfg, err := testenv.Start()
	Expect(err).NotTo(HaveOccurred())

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Metrics: metricsserver.Options{
			BindAddress: "0", // disable the metrics serving
		},
		Scheme: configs.GetRuntimeScheme(),
	})
	Expect(err).NotTo(HaveOccurred())
	runtimeClient = mgr.GetClient()

	transportConfigChan := make(chan *transport.TransportInternalConfig)
	genericConsumer, err = genericconsumer.NewGenericConsumer(transportConfigChan, false, false)
	Expect(err).NotTo(HaveOccurred())

	go func() {
		if err := genericConsumer.Start(ctx); err != nil {
			logf.Log.Error(err, "error to start the chan consumer")
		}
	}()
	go func() {
		transportConfigChan <- agentConfig.TransportConfig
	}()

	genericProducer, err = genericproducer.NewGenericProducer(
		agentConfig.TransportConfig,
		agentConfig.TransportConfig.KafkaCredential.SpecTopic,
		nil,
	)
	Expect(err).NotTo(HaveOccurred())

	transportClient := controller.TransportClient{}
	transportClient.SetConsumer(genericConsumer)
	transportClient.SetProducer(genericProducer)

	err = speccontroller.AddToManager(ctx, mgr, &transportClient, agentConfig)
	Expect(err).NotTo(HaveOccurred())

	go func() {
		Expect(mgr.Start(ctx)).NotTo(HaveOccurred())
	}()

	By("Waiting for the manager to be ready")
	Expect(mgr.GetCache().WaitForCacheSync(ctx)).To(BeTrue())
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
