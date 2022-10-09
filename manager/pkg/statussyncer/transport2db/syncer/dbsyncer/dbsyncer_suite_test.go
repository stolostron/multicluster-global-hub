package dbsyncer_test

import (
	"context"
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
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statistics"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/conflator"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/db/workerpool"
	statussyncer "github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/syncer"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/test/pkg/testkafka"
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
	stats, err := statistics.NewStatistics(ctrl.Log.WithName("statistics"), &statistics.StatisticsConfig{})
	Expect(err).NotTo(HaveOccurred())
	dbWorkerPool, err = workerpool.NewDBWorkerPool(ctrl.Log.WithName("db-worker-pool"), testPostgres.URI, stats)
	Expect(dbWorkerPool.Start()).Should(Succeed())

	By("Create conflationReadyQueue")
	conflationReadyQueue := conflator.NewConflationReadyQueue(stats)
	conflationManager := conflator.NewConflationManager(ctrl.Log.WithName("conflation"), conflationReadyQueue,
		false, stats) // manage all Conflation Units

	kafkaMessageChan = make(chan *kafka.Message)
	statusTransport, err = testkafka.NewKafkaTestConsumer(kafkaMessageChan, conflationManager, stats,
		ctrl.Log.WithName("kafka-consumer"))
	Expect(err).NotTo(HaveOccurred())
	statusTransport.Start()

	mgr, err = ctrl.NewManager(cfg, ctrl.Options{
		MetricsBindAddress: "0",
		Scheme:             scheme.Scheme,
	})
	Expect(err).NotTo(HaveOccurred())

	By("Get kubeClient")
	kubeClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(kubeClient).NotTo(BeNil())

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

	By("Add to Scheme")
	Expect(managerscheme.AddToScheme(mgr.GetScheme())).NotTo(HaveOccurred())

	By("Add transport to database controller")
	err = statussyncer.AddTransport2DBSyncers(mgr, dbWorkerPool, conflationManager,
		conflationReadyQueue, statusTransport, stats)
	Expect(err).NotTo(HaveOccurred())
	go func() {
		defer GinkgoRecover()
		err = mgr.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()

	By("Waiting for the manager to be ready")
	Expect(mgr.GetCache().WaitForCacheSync(ctx)).To(BeTrue())
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")

	cancel()
	statusTransport.Stop()
	transportPostgreSQL.Stop()
	dbWorkerPool.Stop()
	Expect(testPostgres.Stop()).NotTo(HaveOccurred())

	err := testenv.Stop()
	// https://github.com/kubernetes-sigs/controller-runtime/issues/1571
	// Set 4 with random
	if err != nil {
		time.Sleep(4 * time.Second)
	}
	Expect(testenv.Stop()).NotTo(HaveOccurred())
})
