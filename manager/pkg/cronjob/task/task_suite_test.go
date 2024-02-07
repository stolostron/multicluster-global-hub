package task

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/lib/pq"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/test/pkg/testpostgres"
)

var (
	testenv      *envtest.Environment
	cfg          *rest.Config
	ctx          context.Context
	cancel       context.CancelFunc
	testPostgres *testpostgres.TestPostgres
	db           *gorm.DB
)

func TestTasks(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Task Suite")
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

	By("Create test postgres")
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
