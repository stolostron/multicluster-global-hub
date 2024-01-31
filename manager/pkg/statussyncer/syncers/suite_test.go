package dbsyncer_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer"
	dbsyncer "github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/syncers"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
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
	transportDispatcher dbsyncer.BundleRegisterable
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
			filepath.Join("..", "..", "..", "..", "pkg", "testdata", "crds"),
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

	By("Create test postgres")
	managerConfig := &config.ManagerConfig{
		DatabaseConfig: &config.DatabaseConfig{
			ProcessDatabaseURL:         testPostgres.URI,
			TransportBridgeDatabaseURL: testPostgres.URI,
			CACertPath:                 "ca-test-path",
		},
		TransportConfig: &transport.TransportConfig{
			TransportType: string(transport.Chan),
		},
		StatisticsConfig: &statistics.StatisticsConfig{
			LogInterval: "10s",
		},
		EnableGlobalResource: true,
	}
	Expect(err).NotTo(HaveOccurred())
	transportPostgreSQL, err = postgresql.NewSpecPostgreSQL(ctx, managerConfig.DatabaseConfig)
	Expect(err).NotTo(HaveOccurred())
	err = database.InitGormInstance(&database.DatabaseConfig{
		URL:      testPostgres.URI,
		Dialect:  database.PostgresDialect,
		PoolSize: 5,
	})
	Expect(err).NotTo(HaveOccurred())

	By("Start cloudevents producer")
	producer, err = genericproducer.NewGenericProducer(managerConfig.TransportConfig)
	Expect(err).NotTo(HaveOccurred())

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		MetricsBindAddress: "0",
		Scheme:             scheme.Scheme,
	})
	Expect(err).NotTo(HaveOccurred())

	By("Add to Scheme")
	managerscheme.AddToScheme(mgr.GetScheme())

	By("Get kubeClient")
	kubeClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(kubeClient).NotTo(BeNil())

	By("Add controllers to manager")
	transportDispatcher, err = statussyncer.AddStatusSyncers(mgr, managerConfig, nil)
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
	if transportPostgreSQL != nil {
		transportPostgreSQL.Stop()
	}
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
