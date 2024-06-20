// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package spec

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/config"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/nonk8sapi"
	specsycner "github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/syncer"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/spec2db"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	commonobjects "github.com/stolostron/multicluster-global-hub/pkg/objects"
	"github.com/stolostron/multicluster-global-hub/pkg/statistics"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	genericconsumer "github.com/stolostron/multicluster-global-hub/pkg/transport/consumer"
	genericproducer "github.com/stolostron/multicluster-global-hub/pkg/transport/producer"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/test/integration/utils/testpostgres"
)

var (
	testenv         *envtest.Environment
	cfg             *rest.Config
	ctx             context.Context
	cancel          context.CancelFunc
	mgr             ctrl.Manager
	runtimeClient   client.Client
	testPostgres    *testpostgres.TestPostgres
	consumer        transport.Consumer
	producer        transport.Producer
	multiclusterhub *mchv1.MultiClusterHub
)

func TestSpecSyncer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Spec Syncer Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	Expect(os.Setenv("POD_NAMESPACE", "default")).To(Succeed())

	ctx, cancel = context.WithCancel(context.Background())

	By("Prepare envtest environment")
	var err error
	testenv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "manifest", "crd"),
		},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err = testenv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	By("Create test postgres")
	testPostgres, err = testpostgres.NewTestPostgres()
	Expect(err).NotTo(HaveOccurred())
	err = database.InitGormInstance(&database.DatabaseConfig{
		URL:        testPostgres.URI,
		Dialect:    database.PostgresDialect,
		CaCertPath: "ca-cert-path",
		PoolSize:   5,
	})
	Expect(err).NotTo(HaveOccurred())

	err = testpostgres.InitDatabase(testPostgres.URI)
	Expect(err).NotTo(HaveOccurred())

	mgr, err = ctrl.NewManager(cfg, ctrl.Options{
		Metrics: metricsserver.Options{
			BindAddress: "0", // disable the metrics serving
		},
		Scheme: config.GetRuntimeScheme(),
	})
	Expect(err).NotTo(HaveOccurred())

	By("Get kubeClient")
	runtimeClient, err = client.New(cfg, client.Options{Scheme: config.GetRuntimeScheme()})
	Expect(err).NotTo(HaveOccurred())
	Expect(runtimeClient).NotTo(BeNil())

	managerConfig := &config.ManagerConfig{
		SyncerConfig: &config.SyncerConfig{
			SpecSyncInterval:              1 * time.Second,
			DeletedLabelsTrimmingInterval: 2 * time.Second,
		},
		TransportConfig: &transport.TransportConfig{
			TransportType:     string(transport.Chan),
			CommitterInterval: 10 * time.Second,
		},
		StatisticsConfig:      &statistics.StatisticsConfig{},
		NonK8sAPIServerConfig: &nonk8sapi.NonK8sAPIServerConfig{},
		ElectionConfig:        &commonobjects.LeaderElectionConfig{},
	}

	By("Create consumer/producer")
	producer, err = genericproducer.NewGenericProducer(managerConfig.TransportConfig, "spec")
	Expect(err).NotTo(HaveOccurred())
	consumer, err = genericconsumer.NewGenericConsumer(managerConfig.TransportConfig, []string{"spec"})
	Expect(err).NotTo(HaveOccurred())

	By("Add db to transport")
	Expect(mgr.Add(consumer)).Should(Succeed())
	Expect(specsycner.AddDB2TransportSyncers(mgr, managerConfig, producer)).Should(Succeed())
	Expect(specsycner.AddManagedClusterLabelSyncer(mgr,
		managerConfig.SyncerConfig.DeletedLabelsTrimmingInterval)).Should(Succeed())

	By("Add spec to database")
	Expect(spec2db.AddSpec2DBControllers(mgr)).Should(Succeed())

	By("Start the manager")
	go func() {
		defer GinkgoRecover()
		Expect(mgr.Start(ctx)).ToNot(HaveOccurred(), "failed to run manager")
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
	Expect(runtimeClient.Create(ctx, multiclusterhub)).Should(Succeed())
	Expect(runtimeClient.Get(ctx, types.NamespacedName{
		Namespace: multiclusterhub.GetNamespace(),
		Name:      multiclusterhub.GetName(),
	}, multiclusterhub)).Should(Succeed())
})

var _ = AfterSuite(func() {
	cancel()
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
