package status

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	http "github.com/go-kratos/kratos/v2/transport/http"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/project-kessel/inventory-api/api/kessel/inventory/v1beta1/relationships"
	kessel "github.com/project-kessel/inventory-api/api/kessel/inventory/v1beta1/resources"
	"github.com/project-kessel/inventory-client-go/v1beta1"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/status"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/statistics"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	genericconsumer "github.com/stolostron/multicluster-global-hub/pkg/transport/consumer"
	genericproducer "github.com/stolostron/multicluster-global-hub/pkg/transport/producer"
	"github.com/stolostron/multicluster-global-hub/test/integration/utils/testpostgres"
)

var (
	testenv      *envtest.Environment
	cfg          *rest.Config
	ctx          context.Context
	cancel       context.CancelFunc
	testPostgres *testpostgres.TestPostgres
	kubeClient   client.Client
	producer     transport.Producer
)

func TestDbsyncer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Status dbsyncer Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	Expect(os.Setenv("POD_NAMESPACE", "default")).To(Succeed())

	ctx, cancel = context.WithCancel(context.Background())

	By("Prepare envtest environment")
	var err error
	testenv = &envtest.Environment{
		CRDInstallOptions: envtest.CRDInstallOptions{
			Paths: []string{
				filepath.Join("..", "..", "..", "manifest", "crd"),
			},
			MaxTime: 1 * time.Minute,
		},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err = testenv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	By("Prepare postgres database")
	testPostgres, err = testpostgres.NewTestPostgres()
	Expect(err).NotTo(HaveOccurred())
	err = testpostgres.InitDatabase(testPostgres.URI)
	Expect(err).NotTo(HaveOccurred())

	// start manager
	By("Start manager")
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Metrics: metricsserver.Options{
			BindAddress: "0", // disable the metrics serving
		},
		Scheme: configs.GetRuntimeScheme(),
	})
	Expect(err).NotTo(HaveOccurred())

	By("Get kubeClient")
	kubeClient, err = client.New(cfg, client.Options{Scheme: configs.GetRuntimeScheme()})
	Expect(err).NotTo(HaveOccurred())
	Expect(kubeClient).NotTo(BeNil())

	By("Create test postgres")
	managerConfig := &configs.ManagerConfig{
		TransportConfig: &transport.TransportInternalConfig{
			TransportType: string(transport.Chan),
			KafkaCredential: &transport.KafkaConfig{
				SpecTopic:   "spec",
				StatusTopic: "event",
			},
		},
		StatisticsConfig: &statistics.StatisticsConfig{
			LogInterval: "10s",
		},
		EnableGlobalResource: true,
		EnableInventoryAPI:   true,
	}

	By("Start cloudevents producer and consumer")
	producer, err = genericproducer.NewGenericProducer(managerConfig.TransportConfig,
		managerConfig.TransportConfig.KafkaCredential.StatusTopic, nil)
	Expect(err).NotTo(HaveOccurred())

	signalChan := make(chan struct{}, 1)
	// isManager=true automatically sets TopicMetadataRefreshInterval
	consumer, err := genericconsumer.NewGenericConsumer(signalChan, managerConfig.TransportConfig, true, false)
	Expect(err).NotTo(HaveOccurred())
	go func() {
		signalChan <- struct{}{}
	}()
	Expect(mgr.Add(consumer)).Should(Succeed())

	By("Add controllers to manager")
	requester := &FakeRequester{
		&v1beta1.InventoryHttpClient{
			PolicyServiceClient: &FakeKesselK8SPolicyServiceHTTPClientImpl{},
			K8sClusterService:   &FakeKesselK8SClusterServiceHTTPClientImpl{},
			K8SPolicyIsPropagatedToK8SClusterServiceHTTPClient: &FakeKesselK8SPolicyIsPropagatedToK8SClusterServiceHTTPClientImpl{},
		},
	}
	configs.SetEnableInventoryAPI(true)
	err = status.AddStatusSyncers(mgr, consumer, requester, managerConfig)
	Expect(err).ToNot(HaveOccurred())

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
	Expect(testPostgres.Stop()).NotTo(HaveOccurred())
	database.CloseGorm(database.GetSqlDb())

	By("Tearing down the test environment")
	err := testenv.Stop()
	// https://github.com/kubernetes-sigs/controller-runtime/issues/1571
	// Set 4 with random
	if err != nil {
		time.Sleep(4 * time.Second)
	}
	Expect(testenv.Stop()).NotTo(HaveOccurred())
})

// FakeRequester is a mock implementation of the Requester interface.
type FakeRequester struct {
	HttpClient *v1beta1.InventoryHttpClient
}

// RefreshClient is a mock implementation that simulates refreshing the client.
func (f *FakeRequester) RefreshClient(ctx context.Context, restConfig *transport.RestfulConfig) error {
	// Simulate a successful refresh operation
	return nil
}

// GetHttpClient returns a mock InventoryHttpClient.
func (f *FakeRequester) GetHttpClient() *v1beta1.InventoryHttpClient {
	// Return the fake HTTP client
	return f.HttpClient
}

type FakeKesselK8SPolicyServiceHTTPClientImpl struct{}

func (c *FakeKesselK8SPolicyServiceHTTPClientImpl) CreateK8SPolicy(ctx context.Context, in *kessel.CreateK8SPolicyRequest, opts ...http.CallOption) (*kessel.CreateK8SPolicyResponse, error) {
	return nil, nil
}

func (c *FakeKesselK8SPolicyServiceHTTPClientImpl) DeleteK8SPolicy(ctx context.Context, in *kessel.DeleteK8SPolicyRequest, opts ...http.CallOption) (*kessel.DeleteK8SPolicyResponse, error) {
	return nil, nil
}

func (c *FakeKesselK8SPolicyServiceHTTPClientImpl) UpdateK8SPolicy(ctx context.Context, in *kessel.UpdateK8SPolicyRequest, opts ...http.CallOption) (*kessel.UpdateK8SPolicyResponse, error) {
	return nil, nil
}

type FakeKesselK8SClusterServiceHTTPClientImpl struct{}

func (c *FakeKesselK8SClusterServiceHTTPClientImpl) CreateK8SCluster(ctx context.Context, in *kessel.CreateK8SClusterRequest, opts ...http.CallOption) (*kessel.CreateK8SClusterResponse, error) {
	return nil, nil
}

func (c *FakeKesselK8SClusterServiceHTTPClientImpl) DeleteK8SCluster(ctx context.Context, in *kessel.DeleteK8SClusterRequest, opts ...http.CallOption) (*kessel.DeleteK8SClusterResponse, error) {
	return nil, nil
}

func (c *FakeKesselK8SClusterServiceHTTPClientImpl) UpdateK8SCluster(ctx context.Context, in *kessel.UpdateK8SClusterRequest, opts ...http.CallOption) (*kessel.UpdateK8SClusterResponse, error) {
	return nil, nil
}

type FakeKesselK8SPolicyIsPropagatedToK8SClusterServiceHTTPClientImpl struct{}

func (c *FakeKesselK8SPolicyIsPropagatedToK8SClusterServiceHTTPClientImpl) CreateK8SPolicyIsPropagatedToK8SCluster(ctx context.Context, in *relationships.CreateK8SPolicyIsPropagatedToK8SClusterRequest, opts ...http.CallOption) (*relationships.CreateK8SPolicyIsPropagatedToK8SClusterResponse, error) {
	return nil, nil
}

func (c *FakeKesselK8SPolicyIsPropagatedToK8SClusterServiceHTTPClientImpl) DeleteK8SPolicyIsPropagatedToK8SCluster(ctx context.Context, in *relationships.DeleteK8SPolicyIsPropagatedToK8SClusterRequest, opts ...http.CallOption) (*relationships.DeleteK8SPolicyIsPropagatedToK8SClusterResponse, error) {
	return nil, nil
}

func (c *FakeKesselK8SPolicyIsPropagatedToK8SClusterServiceHTTPClientImpl) UpdateK8SPolicyIsPropagatedToK8SCluster(ctx context.Context, in *relationships.UpdateK8SPolicyIsPropagatedToK8SClusterRequest, opts ...http.CallOption) (*relationships.UpdateK8SPolicyIsPropagatedToK8SClusterResponse, error) {
	return nil, nil
}
