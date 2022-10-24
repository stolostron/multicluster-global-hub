package dbsyncer_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/confluentinc/confluent-kafka-go/kafka"
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

	managerscheme "github.com/stolostron/multicluster-global-hub/manager/pkg/scheme"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/db/postgresql"
	statusbundle "github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/bundle"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/db/workerpool"
	statussyncer "github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/syncer"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/helpers"
	"github.com/stolostron/multicluster-global-hub/pkg/compressor"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/statistics"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/consumer"
	"github.com/stolostron/multicluster-global-hub/test/pkg/testpostgres"
)

var (
	testenv             *envtest.Environment
	cfg                 *rest.Config
	ctx                 context.Context
	cancel              context.CancelFunc
	mgr                 ctrl.Manager
	kubeClient          client.Client
	testPostgres        *testpostgres.TestPostgres
	transportPostgreSQL *postgresql.PostgreSQL
	dbWorkerPool        *workerpool.DBWorkerPool
	statusTransport     transport.Transport
	kafkaMessageChan    chan *kafka.Message
	mockCluster         *kafka.MockCluster
)

func TestDbsyncer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Database syncer Suite")
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

	By("Create database work pool")
	stats, err := statistics.NewStatistics(ctrl.Log.WithName("statistics"), &statistics.StatisticsConfig{},
		[]string{
			helpers.GetBundleType(&statusbundle.ManagedClustersStatusBundle{}),
			helpers.GetBundleType(&statusbundle.ClustersPerPolicyBundle{}),
			helpers.GetBundleType(&statusbundle.CompleteComplianceStatusBundle{}),
			helpers.GetBundleType(&statusbundle.DeltaComplianceStatusBundle{}),
			helpers.GetBundleType(&statusbundle.MinimalComplianceStatusBundle{}),
			helpers.GetBundleType(&statusbundle.PlacementRulesBundle{}),
			helpers.GetBundleType(&statusbundle.PlacementsBundle{}),
			helpers.GetBundleType(&statusbundle.PlacementDecisionsBundle{}),
			helpers.GetBundleType(&statusbundle.SubscriptionStatusesBundle{}),
			helpers.GetBundleType(&statusbundle.SubscriptionReportsBundle{}),
			helpers.GetBundleType(&statusbundle.ControlInfoBundle{}),
			helpers.GetBundleType(&statusbundle.LocalPolicySpecBundle{}),
			helpers.GetBundleType(&statusbundle.LocalClustersPerPolicyBundle{}),
			helpers.GetBundleType(&statusbundle.LocalCompleteComplianceStatusBundle{}),
			helpers.GetBundleType(&statusbundle.LocalPlacementRulesBundle{}),
		})
	Expect(err).NotTo(HaveOccurred())
	dbWorkerPool, err = workerpool.NewDBWorkerPool(ctrl.Log.WithName("db-worker-pool"), testPostgres.URI, stats)
	Expect(err).NotTo(HaveOccurred())
	Expect(dbWorkerPool.Start()).Should(Succeed())

	By("Create conflationReadyQueue")
	conflationReadyQueue := conflator.NewConflationReadyQueue(stats)
	conflationManager := conflator.NewConflationManager(ctrl.Log.WithName("conflation"),
		conflationReadyQueue, false, stats) // manage all Conflation Units

	kafkaMessageChan = make(chan *kafka.Message)
	mockCluster, err = kafka.NewMockCluster(1)
	Expect(err).NotTo(HaveOccurred())

	kafkaConsumerConfig := &consumer.KafkaConsumerConfig{
		ConsumerTopic: "test-topic",
		ConsumerID:    "test-consumer",
	}
	kafkaConsumer, err := consumer.NewKafkaConsumer(
		mockCluster.BootstrapServers(), "", kafkaConsumerConfig,
		ctrl.Log.WithName("kafka-consumer"))
	Expect(err).NotTo(HaveOccurred())

	kafkaConsumer.SetCommitter(consumer.NewCommitter(
		1*time.Second, kafkaConsumerConfig.ConsumerTopic, kafkaConsumer.Consumer(),
		conflationManager.GetBundlesMetadata, ctrl.Log.WithName("kafka-consumer")),
	)
	kafkaConsumer.SetStatistics(stats)
	kafkaConsumer.Start()

	mgr, err = ctrl.NewManager(cfg, ctrl.Options{
		MetricsBindAddress: "0",
		Scheme:             scheme.Scheme,
	})
	Expect(err).NotTo(HaveOccurred())

	By("Get kubeClient")
	kubeClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(kubeClient).NotTo(BeNil())

	By("Add to Scheme")
	Expect(managerscheme.AddToScheme(mgr.GetScheme())).NotTo(HaveOccurred())

	By("Create the global hub ConfigMap with aggregationLevel=full and enableLocalPolicies=true")
	mghSystemNamespace := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: constants.HohSystemNamespace}}
	Expect(kubeClient.Create(ctx, mghSystemNamespace)).Should(Succeed())
	mghSystemConfigMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.HoHConfigName,
			Namespace: constants.HohSystemNamespace,
		},
		Data: map[string]string{"aggregationLevel": "full", "enableLocalPolicies": "true"},
	}
	Expect(kubeClient.Create(ctx, mghSystemConfigMap)).Should(Succeed())

	By("Add controllers to manager")
	err = statussyncer.AddTransport2DBSyncers(mgr, dbWorkerPool, conflationManager,
		conflationReadyQueue, statusTransport, stats)
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
	statusTransport.Stop()
	transportPostgreSQL.Stop()
	dbWorkerPool.Stop()
	mockCluster.Close()
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

func buildKafkaMessage(key, id string, payload []byte) (*kafka.Message, error) {
	transportMessage := &transport.Message{
		Key:     key,
		ID:      id, // entry.transportBundleKey
		MsgType: constants.StatusBundle,
		Version: "0.2", // entry.bundle.GetBundleVersion().String()
		Payload: payload,
	}
	transportMessageBytes, err := json.Marshal(transportMessage)
	if err != nil {
		return nil, err
	}

	compressor, err := compressor.NewCompressor(compressor.GZip)
	if err != nil {
		return nil, err
	}
	compressedTransportBytes, err := compressor.Compress(transportMessageBytes)
	if err != nil {
		return nil, err
	}

	topic := "status"
	kafkaMessage := &kafka.Message{
		Key: []byte(transportMessage.Key),
		TopicPartition: kafka.TopicPartition{
			Topic:     &topic,
			Partition: 0,
		},
		Headers: []kafka.Header{
			{Key: transport.CompressionType, Value: []byte(compressor.GetType())},
		},
		Value:         compressedTransportBytes,
		TimestampType: 1,
		Timestamp:     time.Now(),
	}
	return kafkaMessage, nil
}
