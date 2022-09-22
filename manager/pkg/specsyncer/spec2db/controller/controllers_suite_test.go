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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	managerscheme "github.com/stolostron/multicluster-global-hub/manager/pkg/scheme"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/db/postgresql"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/spec2db/controller"
	mghv1alpha2 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha2"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/test/pkg/testpostgres"
)

var (
	testenv      *envtest.Environment
	cfg          *rest.Config
	ctx          context.Context
	cancel       context.CancelFunc
	testPostgres *testpostgres.TestPostgres
	mgr          ctrl.Manager
	postgresSQL  *postgresql.PostgreSQL
	kubeClient   client.Client
	mghInstance  *mghv1alpha2.MulticlusterGlobalHub
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
			filepath.Join("..", "..", "..", "test", "crd"),
		},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err = testenv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	By("Create test postgres")
	testPostgres, err = testpostgres.NewTestPostgres()
	Expect(err).NotTo(HaveOccurred())

	By("Create controller manager")
	mgr, err = ctrl.NewManager(cfg, ctrl.Options{
		MetricsBindAddress: ":8083",
		Scheme:             scheme.Scheme,
	})
	Expect(err).NotTo(HaveOccurred())

	By("Add to Scheme")
	Expect(managerscheme.AddToScheme(mgr.GetScheme())).NotTo(HaveOccurred())

	By("Get kubeClient")
	kubeClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(kubeClient).NotTo(BeNil())

	By("Connect to the database")
	postgresSQL, err = postgresql.NewPostgreSQL(testPostgres.URI)
	Expect(err).NotTo(HaveOccurred())
	Expect(postgresSQL).NotTo(BeNil())

	By("Adding the controllers to the manager")
	controller.AddHubOfHubsConfigController(mgr, postgresSQL)
	controller.AddApplicationController(mgr, postgresSQL)
	controller.AddChannelController(mgr, postgresSQL)
	controller.AddSubscriptionController(mgr, postgresSQL)
	go func() {
		defer GinkgoRecover()
		err = mgr.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()

	By("Waiting for the manager to be ready")
	Expect(mgr.GetCache().WaitForCacheSync(ctx)).To(BeTrue())

	By("Create MGH instance")
	mghInstance = &mghv1alpha2.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "multicluster-global-hub",
			Namespace: config.GetDefaultNamespace(),
		},
		Spec: mghv1alpha2.MulticlusterGlobalHubSpec{
			DataLayer: &mghv1alpha2.DataLayerConfig{
				Type:   mghv1alpha2.Native,
				Native: &mghv1alpha2.NativeConfig{},
			},
		},
	}
	Expect(kubeClient.Create(ctx, mghInstance)).Should(Succeed())
	Expect(kubeClient.Get(ctx, types.NamespacedName{
		Namespace: mghInstance.GetNamespace(),
		Name:      mghInstance.GetName(),
	}, mghInstance)).Should(Succeed())
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testenv.Stop()
	// https://github.com/kubernetes-sigs/controller-runtime/issues/1571
	// Set 4 with random
	if err != nil {
		time.Sleep(4 * time.Second)
	}
	cancel()
	postgresSQL.Stop()
	Expect(testPostgres.Stop()).NotTo(HaveOccurred())
	Expect(testenv.Stop()).NotTo(HaveOccurred())
})
