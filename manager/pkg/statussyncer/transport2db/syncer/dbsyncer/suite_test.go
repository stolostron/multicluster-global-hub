package dbsyncer_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/config"
	managerscheme "github.com/stolostron/multicluster-global-hub/manager/pkg/scheme"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/db/postgresql"
	statussyncer "github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/syncer"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/syncer/dispatcher"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/statistics"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	genericproducer "github.com/stolostron/multicluster-global-hub/pkg/transport/producer"
	"github.com/stolostron/multicluster-global-hub/test/pkg/testpostgres"
)

var (
	testenv             *envtest.Environment
	cfg                 *rest.Config
	ctx                 context.Context
	cancel              context.CancelFunc
	testPostgres        *testpostgres.TestPostgres
	transportPostgreSQL *postgresql.PostgreSQL
	kubeClient          client.Client
	producer            transport.Producer
	transportDispatcher *dispatcher.TransportDispatcher
	// dbWorkerPool *workerpool.DBWorkerPool
	// kafkaConsumer       *consumer.KafkaConsumer
	// kafkaProducer       *producer.KafkaProducer
	// mockCluster         *kafka.MockCluster
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
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "..", "..", "..", "pkg", "testdata", "crds"),
		},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err = testenv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	By("Create test postgres")
	testPostgres, err = testpostgres.NewTestPostgres()
	Expect(err).NotTo(HaveOccurred())
	transportPostgreSQL, err = postgresql.NewPostgreSQL(testPostgres.URI)
	Expect(err).NotTo(HaveOccurred())

	// By("Create database work pool")
	// stats := statistics.NewStatistics(ctrl.Log.WithName("statistics"), &statistics.StatisticsConfig{},
	// 	[]string{
	// 		helpers.GetBundleType(&statusbundle.ManagedClustersStatusBundle{}),
	// 		helpers.GetBundleType(&statusbundle.ClustersPerPolicyBundle{}),
	// 		helpers.GetBundleType(&statusbundle.CompleteComplianceStatusBundle{}),
	// 		helpers.GetBundleType(&statusbundle.DeltaComplianceStatusBundle{}),
	// 		helpers.GetBundleType(&statusbundle.MinimalComplianceStatusBundle{}),
	// 		helpers.GetBundleType(&statusbundle.PlacementRulesBundle{}),
	// 		helpers.GetBundleType(&statusbundle.PlacementsBundle{}),
	// 		helpers.GetBundleType(&statusbundle.PlacementDecisionsBundle{}),
	// 		helpers.GetBundleType(&statusbundle.SubscriptionStatusesBundle{}),
	// 		helpers.GetBundleType(&statusbundle.SubscriptionReportsBundle{}),
	// 		helpers.GetBundleType(&statusbundle.ControlInfoBundle{}),
	// 		helpers.GetBundleType(&statusbundle.LocalPolicySpecBundle{}),
	// 		helpers.GetBundleType(&statusbundle.LocalClustersPerPolicyBundle{}),
	// 		helpers.GetBundleType(&statusbundle.LocalCompleteComplianceStatusBundle{}),
	// 		helpers.GetBundleType(&statusbundle.LocalPlacementRulesBundle{}),
	// 	})

	// dbWorkerPool, err = workerpool.NewDBWorkerPool(ctrl.Log.WithName("db-worker-pool"), testPostgres.URI, stats)
	// Expect(err).NotTo(HaveOccurred())

	// By("Create conflationReadyQueue")
	// conflationReadyQueue := conflator.NewConflationReadyQueue(stats)
	// conflationManager := conflator.NewConflationManager(ctrl.Log.WithName("conflation"),
	// 	conflationReadyQueue, false, stats) // manage all Conflation Units

	// By("Create mock kafka cluster")
	// mockCluster, err = kafka.NewMockCluster(1)
	// Expect(err).NotTo(HaveOccurred())
	// fmt.Fprintf(GinkgoWriter, "mock kafka bootstrap server address: %s\n", mockCluster.BootstrapServers())

	managerConfig := &config.ManagerConfig{
		DatabaseConfig: &config.DatabaseConfig{
			TransportBridgeDatabaseURL: testPostgres.URI,
		},
		TransportConfig: &transport.TransportConfig{
			TransportType: string(transport.Chan),
		},
		StatisticsConfig: &statistics.StatisticsConfig{},
	}

	By("Start cloudevents producer")
	producer, err = genericproducer.NewGenericProducer(managerConfig.TransportConfig)
	// kafkaProducerConfig := &protocol.KafkaProducerConfig{
	// 	ProducerTopic:      "status",
	// 	ProducerID:         "status-producer",
	// 	MessageSizeLimitKB: 1,
	// }
	// kafkaProducer, err = producer.NewKafkaProducer(&compressor.CompressorGZip{},
	// 	mockCluster.BootstrapServers(), "", kafkaProducerConfig,
	// 	ctrl.Log.WithName("kafka-producer"))
	// Expect(err).NotTo(HaveOccurred())

	// By("Start kafka consumer")
	// kafkaConsumerConfig := &protocol.KafkaConsumerConfig{
	// 	ConsumerTopic: "status",
	// 	ConsumerID:    "status-consumer",
	// }
	// kafkaConsumer, err = consumer.NewKafkaConsumer(
	// 	mockCluster.BootstrapServers(), "", kafkaConsumerConfig,
	// 	ctrl.Log.WithName("kafka-consumer"))
	// Expect(err).NotTo(HaveOccurred())

	// kafkaConsumer.SetCommitter(consumer.NewCommitter(
	// 	1*time.Second, kafkaConsumerConfig.ConsumerTopic, kafkaConsumer.Consumer(),
	// 	conflationManager.GetBundlesMetadata, ctrl.Log.WithName("kafka-consumer")),
	// )
	// kafkaConsumer.SetStatistics(stats)
	// kafkaConsumer.SetConflationManager(conflationManager)

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		MetricsBindAddress: "0",
		Scheme:             scheme.Scheme,
	})
	Expect(err).NotTo(HaveOccurred())

	By("Add to Scheme")
	Expect(managerscheme.AddToScheme(mgr.GetScheme())).NotTo(HaveOccurred())

	By("Get kubeClient")
	kubeClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(kubeClient).NotTo(BeNil())

	By("Create the global hub ConfigMap with aggregationLevel=full and enableLocalPolicies=true")
	mghSystemNamespace := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: constants.GHSystemNamespace}}
	Expect(kubeClient.Create(ctx, mghSystemNamespace)).Should(Succeed())
	mghSystemConfigMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.GHConfigCMName,
			Namespace: constants.GHSystemNamespace,
		},
		Data: map[string]string{"aggregationLevel": "full", "enableLocalPolicies": "true"},
	}
	Expect(kubeClient.Create(ctx, mghSystemConfigMap)).Should(Succeed())

	By("Add controllers to manager")
	transportDispatcher, err = statussyncer.AddTransport2DBSyncers(mgr, managerConfig)
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
	transportPostgreSQL.Stop()
	// mockCluster.Close()
	Expect(testPostgres.Stop()).NotTo(HaveOccurred())

	By("Tearing down the test environment")
	err := testenv.Stop()
	// https://github.com/kubernetes-sigs/controller-runtime/issues/1571
	// Set 4 with random
	if err != nil {
		time.Sleep(4 * time.Second)
	}
	Expect(testenv.Stop()).NotTo(HaveOccurred())
})
