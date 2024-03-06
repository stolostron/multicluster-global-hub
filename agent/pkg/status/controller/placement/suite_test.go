package placement

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/config"
	agentscheme "github.com/stolostron/multicluster-global-hub/agent/pkg/scheme"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	genericconsumer "github.com/stolostron/multicluster-global-hub/pkg/transport/consumer"
	genericproducer "github.com/stolostron/multicluster-global-hub/pkg/transport/producer"
)

var (
	leafHubName = "hub1"
	testenv     *envtest.Environment
	cfg         *rest.Config
	ctx         context.Context
	cancel      context.CancelFunc
	kubeClient  client.Client
	consumer    transport.Consumer
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Status Controller Integration Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	ctx, cancel = context.WithCancel(context.TODO())

	By("Prepare envtest environment")
	var err error
	testenv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "..", "..", "pkg", "testdata", "crds"),
		},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err = testenv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	topic := "status"
	agentConfig := &config.AgentConfig{
		LeafHubName: leafHubName,
		TransportConfig: &transport.TransportConfig{
			CommitterInterval: 1 * time.Second,
			TransportType:     string(transport.Chan),
			KafkaConfig: &transport.KafkaConfig{
				Topics: &transport.ClusterTopic{
					StatusTopic: topic,
				},
			},
		},
		EnableGlobalResource: true,
	}

	By("Add to Scheme")
	agentscheme.AddToScheme(scheme.Scheme)

	By("Create controller-runtime manager")
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		MetricsBindAddress: "0",
		Scheme:             scheme.Scheme,
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(mgr).NotTo(BeNil())

	By("Get kubeClient")
	kubeClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(kubeClient).NotTo(BeNil())

	By("Create global hub system namespace")
	mghSystemNamespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: constants.GHAgentNamespace}}
	Expect(kubeClient.Create(ctx, mghSystemNamespace)).Should(Succeed())

	By("create the configmap to disable the heartbeat on the suite test")
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: constants.GHAgentNamespace,
			Name:      constants.GHAgentConfigCMName,
		},
		Data: map[string]string{
			"hubClusterHeartbeat": "5m",
		},
	}
	Expect(kubeClient.Create(ctx, configMap)).Should(Succeed())

	By("Create cloudevents consumer")
	consumer, err = genericconsumer.NewGenericConsumer(agentConfig.TransportConfig, []string{topic})
	Expect(err).NotTo(HaveOccurred())
	err = mgr.Add(consumer)
	Expect(err).NotTo(HaveOccurred())

	producer, err := genericproducer.NewGenericProducer(agentConfig.TransportConfig, topic)

	By("Add controllers to manager")
	err = LaunchPlacementRuleSyncer(ctx, mgr, agentConfig, producer)
	Expect(err).Should(Succeed())
	err = LaunchPlacementSyncer(ctx, mgr, agentConfig, producer)
	Expect(err).Should(Succeed())
	err = LaunchPlacementDecisionSyncer(ctx, mgr, agentConfig, producer)
	Expect(err).Should(Succeed())

	By("Start the manager")
	go func() {
		defer GinkgoRecover()
		Expect(mgr.Start(ctx)).ToNot(HaveOccurred(), "failed to run manager")
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
		Expect(testenv.Stop()).NotTo(HaveOccurred())
	}
})
