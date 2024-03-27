package controller_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/lib/pq"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	managerconfig "github.com/stolostron/multicluster-global-hub/manager/pkg/config"
	managerscheme "github.com/stolostron/multicluster-global-hub/manager/pkg/scheme"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/db/postgresql"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/spec2db"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/test/pkg/testpostgres"
)

var (
	testenv         *envtest.Environment
	cfg             *rest.Config
	ctx             context.Context
	cancel          context.CancelFunc
	testPostgres    *testpostgres.TestPostgres
	mgr             ctrl.Manager
	postgresSQL     *postgresql.PostgreSQL
	kubeClient      client.Client
	multiclusterhub *mchv1.MultiClusterHub
)

func TestSpec2db(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Spec2db Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	Expect(os.Setenv("POD_NAMESPACE", "default")).To(Succeed())

	ctx, cancel = context.WithCancel(context.Background())

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

	By("Create test postgres")
	testPostgres, err = testpostgres.NewTestPostgres()
	Expect(err).NotTo(HaveOccurred())
	err = testpostgres.InitDatabase(testPostgres.URI)
	Expect(err).NotTo(HaveOccurred())
	err = database.InitGormInstance(&database.DatabaseConfig{
		URL:      testPostgres.URI,
		Dialect:  database.PostgresDialect,
		PoolSize: 2,
	})
	Expect(err).NotTo(HaveOccurred())

	mgr, err = ctrl.NewManager(cfg, ctrl.Options{
		Metrics: metricsserver.Options{
			BindAddress: "0", // disable the metrics serving
		}, Scheme: scheme.Scheme,
	})
	Expect(err).NotTo(HaveOccurred())

	By("Add to Scheme")
	managerscheme.AddToScheme(mgr.GetScheme())
	Expect(mchv1.AddToScheme(mgr.GetScheme())).NotTo(HaveOccurred())

	By("Get kubeClient")
	kubeClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(kubeClient).NotTo(BeNil())

	By("Connect to the database")
	dataConfig := &managerconfig.DatabaseConfig{
		ProcessDatabaseURL: testPostgres.URI,
		CACertPath:         "ca-cert-path",
	}
	postgresSQL, err = postgresql.NewSpecPostgreSQL(ctx, dataConfig)
	Expect(err).NotTo(HaveOccurred())
	Expect(postgresSQL).NotTo(BeNil())

	By("Adding the controllers to the manager")
	Expect(spec2db.AddSpec2DBControllers(mgr)).Should(Succeed())
	go func() {
		defer GinkgoRecover()
		err = mgr.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()

	By("Waiting for the manager to be ready")
	Expect(mgr.GetCache().WaitForCacheSync(ctx)).To(BeTrue())

	By("Create MGH instance")
	multiclusterhub = &mchv1.MultiClusterHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "multiclusterhub",
			Namespace: utils.GetDefaultNamespace(),
		},
		Spec: mchv1.MultiClusterHubSpec{},
	}
	Expect(kubeClient.Create(ctx, multiclusterhub)).Should(Succeed())
	Expect(kubeClient.Get(ctx, types.NamespacedName{
		Namespace: multiclusterhub.GetNamespace(),
		Name:      multiclusterhub.GetName(),
	}, multiclusterhub)).Should(Succeed())
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testenv.Stop()
	// https://github.com/kubernetes-sigs/controller-runtime/issues/1571
	// Set 4 with random
	if err != nil {
		time.Sleep(4 * time.Second)
		Expect(testenv.Stop()).NotTo(HaveOccurred())
	}
	Expect(testPostgres.Stop()).NotTo(HaveOccurred())
	cancel()
})
