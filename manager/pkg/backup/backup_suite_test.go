package backup_test

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
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/backup"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/test/pkg/testpostgres"
)

var (
	testenv          *envtest.Environment
	cfg              *rest.Config
	ctx              context.Context
	cancel           context.CancelFunc
	testPostgres     *testpostgres.TestPostgres
	db               *gorm.DB
	mgr              manager.Manager
	backupReconciler *backup.BackupPVCReconciler
	timeout          = time.Second * 30
	interval         = time.Millisecond * 250
)

func TestTasks(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Backup Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	Expect(os.Setenv("POD_NAMESPACE", "default")).To(Succeed())

	ctx, cancel = context.WithCancel(context.Background())

	By("Prepare envtest environment")
	var err error
	testenv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "pkg", "testdata", "crds"),
		},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err = testenv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	kubeClient, err := kubernetes.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())
	By("Creating the namespace")
	mghSystemNamespace := &corev1.Namespace{ObjectMeta: v1.ObjectMeta{Name: constants.GHDefaultNamespace}}
	_, err = kubeClient.CoreV1().Namespaces().Create(ctx, mghSystemNamespace, v1.CreateOptions{})
	Expect(err).Should(Succeed())

	mchNamespace := &corev1.Namespace{ObjectMeta: v1.ObjectMeta{Name: "open-cluster-management"}}
	_, err = kubeClient.CoreV1().Namespaces().Create(ctx, mchNamespace, v1.CreateOptions{})
	Expect(err).Should(Succeed())

	By("Create test postgres")
	database.IsBackupEnabled = true
	testPostgres, err = testpostgres.NewTestPostgres()
	Expect(err).NotTo(HaveOccurred())
	err = testpostgres.InitDatabase(testPostgres.URI)
	Expect(err).NotTo(HaveOccurred())

	By("Connect to the database")
	err = database.InitGormInstance(&database.DatabaseConfig{
		URL:      testPostgres.URI,
		Dialect:  database.PostgresDialect,
		PoolSize: 1,
	})
	Expect(err).NotTo(HaveOccurred())
	db = database.GetGorm()
	// add scheme
	Expect(mchv1.AddToScheme(scheme.Scheme)).NotTo(HaveOccurred())

	By("Creating the Manager")

	mgr, err = ctrl.NewManager(cfg, ctrl.Options{
		MetricsBindAddress: "0", // disable the metrics serving
		Scheme:             scheme.Scheme,
	})
	Expect(err).NotTo(HaveOccurred())
	backupReconciler = backup.NewBackupPVCReconciler(mgr, database.GetConn())
	backupReconciler.SetupWithManager(mgr)

	go func() {
		Expect(mgr.Start(ctx)).NotTo(HaveOccurred())
	}()

	By("Waiting for the manager to be ready")
	Expect(mgr.GetCache().WaitForCacheSync(ctx)).To(BeTrue())
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
	database.CloseGorm(database.GetSqlDb())
	Expect(testPostgres.Stop()).NotTo(HaveOccurred())
	cancel()
})
