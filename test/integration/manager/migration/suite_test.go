package migration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/migration"
	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	genericconsumer "github.com/stolostron/multicluster-global-hub/pkg/transport/consumer"
	genericproducer "github.com/stolostron/multicluster-global-hub/pkg/transport/producer"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/test/integration/utils/testpostgres"
)

var (
	testenv         *envtest.Environment
	transportConfig *transport.TransportInternalConfig
	cfg             *rest.Config
	ctx             context.Context
	cancel          context.CancelFunc
	testPostgres    *testpostgres.TestPostgres
	db              *gorm.DB
	mgr             manager.Manager
)

func TestController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Manager Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	Expect(os.Setenv("POD_NAMESPACE", "default")).To(Succeed())

	ctx, cancel = context.WithCancel(context.Background())

	transportConfig = &transport.TransportInternalConfig{
		TransportType: string(transport.Chan),
		KafkaCredential: &transport.KafkaConfig{
			SpecTopic:   "spec",
			StatusTopic: "status",
		},
	}
	managerConfig := &configs.ManagerConfig{TransportConfig: transportConfig}

	By("Prepare envtest environment")
	var err error
	testenv = &envtest.Environment{
		CRDInstallOptions: envtest.CRDInstallOptions{
			Paths: []string{
				filepath.Join("..", "..", "..", "manifest", "crd"),
				filepath.Join("..", "..", "..", "..", "operator", "config", "crd", "bases"),
			},
			MaxTime: 30 * time.Second,
		},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err = testenv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

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

	By("Add the backup controller to the manager")
	mgr, err = ctrl.NewManager(cfg, ctrl.Options{
		Metrics: metricsserver.Options{
			BindAddress: "0", // disable the metrics serving
		},
		Scheme: configs.GetRuntimeScheme(),
	})
	Expect(err).NotTo(HaveOccurred())

	genericProducer, err := genericproducer.NewGenericProducer(
		transportConfig,
		transportConfig.KafkaCredential.SpecTopic,
		nil,
	)
	Expect(err).NotTo(HaveOccurred())

	consumer, err := genericconsumer.NewGenericConsumer(transportConfig,
		[]string{transportConfig.KafkaCredential.SpecTopic})
	Expect(err).NotTo(HaveOccurred())
	go func() {
		if err := consumer.Start(ctx); err != nil {
			logf.Log.Error(err, "error to start the chan consumer")
		}
	}()
	// use the dispatcher to consume events from transport
	go fakeDispatch(ctx, consumer)
	Expect(err).NotTo(HaveOccurred())
	Expect(err).NotTo(HaveOccurred())

	err = migration.AddMigrationToManager(mgr, genericProducer, managerConfig)
	Expect(err).NotTo(HaveOccurred())

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
	// Set 2 with random
	if err != nil {
		time.Sleep(2 * time.Second)
		Expect(testenv.Stop()).NotTo(HaveOccurred())
	}
	database.CloseGorm(database.GetSqlDb())
	Expect(testPostgres.Stop()).NotTo(HaveOccurred())
	cancel()
})

// createHubAndCluster creates the necessary K8s and DB resources for a test.
func createHubAndCluster(ctx context.Context, hubName, clusterName string) error {
	// Create Namespace for the hub
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: hubName}}
	if err := mgr.GetClient().Create(ctx, ns); err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create namespace %s: %w", hubName, err)
	}

	// Create ManagedCluster for the hub
	hubCluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: hubName,
			Annotations: map[string]string{
				"addon.open-cluster-management.io/on-multicluster-hub": "true",
			},
		},
		Spec: clusterv1.ManagedClusterSpec{ManagedClusterClientConfigs: []clusterv1.ClientConfig{{URL: "https://hub.example.com"}}},
		Status: clusterv1.ManagedClusterStatus{
			Conditions: []metav1.Condition{
				{
					Type:               "ManagedClusterConditionAvailable",
					Status:             "True",
					Reason:             "ManagedClusterAvailable",
					Message:            "The managed cluster is available",
					LastTransitionTime: metav1.Now(),
				},
			},
		},
	}
	if err := mgr.GetClient().Create(ctx, hubCluster); err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create hub cluster %s: %w", hubName, err)
	}

	// Update status subresource to persist the available condition
	// Get latest hubCluster before status update to avoid resource version conflict
	Eventually(func() error {
		latest := &clusterv1.ManagedCluster{}
		err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(hubCluster), latest)
		if err != nil {
			return err
		}
		latest.Status.Conditions = []metav1.Condition{
			{
				Type:               "ManagedClusterConditionAvailable",
				Status:             "True",
				Reason:             "ManagedClusterAvailable",
				Message:            "The managed cluster is available",
				LastTransitionTime: metav1.Now(),
			},
		}

		if err := mgr.GetClient().Status().Update(ctx, latest); err != nil {
			return fmt.Errorf("failed to update hub cluster status %s: %w", hubName, err)
		}
		return nil
	}, "10s", "200ms").Should(Succeed())

	Eventually(func() error {
		latest := &clusterv1.ManagedCluster{}
		err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(hubCluster), latest)
		if err != nil {
			return err
		}
		if len(latest.Status.Conditions) == 0 {
			return fmt.Errorf("ManagedCluster status conditions should not be empty")
		}
		if latest.Status.Conditions[0].Type != "ManagedClusterConditionAvailable" {
			return fmt.Errorf("ManagedCluster status condition type should be ManagedClusterConditionAvailable, got %s", latest.Status.Conditions[0].Type)
		}
		if latest.Status.Conditions[0].Status != "True" {
			return fmt.Errorf("ManagedCluster status condition status should be True, got %s", latest.Status.Conditions[0].Status)
		}
		return nil
	}, "10s", "200ms").Should(Succeed())

	// Create DB entry for the hub
	if err := db.Create(&models.LeafHub{LeafHubName: hubName, ClusterID: uuid.New().String(), Payload: []byte("{}")}).Error; err != nil {
		return fmt.Errorf("failed to create leaf hub DB entry %s: %w", hubName, err)
	}

	// Create DB entry for the managed cluster
	if clusterName != "" {
		managedCluster := &clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{Name: clusterName},
			Spec:       clusterv1.ManagedClusterSpec{ManagedClusterClientConfigs: []clusterv1.ClientConfig{{URL: "https://cluster.example.com"}}},
			Status: clusterv1.ManagedClusterStatus{
				Conditions: []metav1.Condition{
					{
						Type:               "ManagedClusterConditionAvailable",
						Status:             "True",
						Reason:             "ManagedClusterAvailable",
						Message:            "The managed cluster is available",
						LastTransitionTime: metav1.Now(),
					},
				},
			},
		}
		payload, err := json.Marshal(managedCluster)
		if err != nil {
			return err
		}
		if err := db.Create(&models.ManagedCluster{LeafHubName: hubName, ClusterID: uuid.New().String(), Payload: payload, Error: "none"}).Error; err != nil {
			return fmt.Errorf("failed to create managed cluster DB entry %s: %w", clusterName, err)
		}

	}

	return nil
}

func fakeDispatch(ctx context.Context, consumer transport.Consumer) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt := <-consumer.EventChan():
			fmt.Printf("evt: %s", evt.ID())
			continue
		}
	}
}

func ensureManagedServiceAccount(migrationName, toHub string) error {
	// Create a mock ManagedClusterAddOn for the hub
	addOn := &addonapiv1alpha1.ManagedClusterAddOn{
		ObjectMeta: metav1.ObjectMeta{Name: "managed-serviceaccount", Namespace: toHub},
	}
	if err := mgr.GetClient().Create(ctx, addOn); err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create managedserviceaccount addon in hub %s: %w", toHub, err)
	}

	// wait until the managedserviceaccount is created
	time.Sleep(100 * time.Millisecond)

	if err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(addOn), addOn); err != nil {
		return err
	}

	// the addOn.Status.Namespace can not be updated in to the addon
	addOn.Spec.InstallNamespace = "open-cluster-management-agent-addon"
	if err := mgr.GetClient().Update(ctx, addOn); err != nil {
		return fmt.Errorf("failed to update addon status with namespace: %w", err)
	}

	// mock the token secret
	tokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      migrationName,
			Namespace: toHub,
		},
		Data: map[string][]byte{"token": []byte("mock-token")},
	}
	if err := mgr.GetClient().Create(ctx, tokenSecret); err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create token secret %s in namespace %s: %w", migrationName, toHub, err)
	}

	return nil
}

// cleanupHubAndClusters removes all resources created for a test.
func cleanupHubAndClusters(ctx context.Context, hubName, clusterName string) {
	// Delete K8s resources
	if err := mgr.GetClient().Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: hubName}}); err != nil {
		log.Printf("failed to delete namespace: %v", err)
	}
	if err := mgr.GetClient().Delete(ctx, &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: hubName}}); err != nil {
		log.Printf("failed to delete managed cluster: %v", err)
	}
	// mgr.GetClient().Delete(ctx, &addonapiv1alpha1.ManagedClusterAddOn{ObjectMeta: metav1.ObjectMeta{Name: "managed-serviceaccount", Namespace: hubName}})

	// Delete DB entries
	db.Exec("DELETE FROM status.leaf_hubs WHERE leaf_hub_name = ?", hubName)
	db.Exec("DELETE FROM status.managed_clusters WHERE payload->'metadata'->>'name' = ?", clusterName)
}

// createMigrationCR creates a ManagedClusterMigration custom resource.
func createMigrationCR(ctx context.Context, name, fromHub, toHub string, clusters []string) (*migrationv1alpha1.ManagedClusterMigration, error) {
	m := &migrationv1alpha1.ManagedClusterMigration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: utils.GetDefaultNamespace(),
		},
		Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
			From:                    fromHub,
			To:                      toHub,
			IncludedManagedClusters: clusters,
		},
	}
	if err := mgr.GetClient().Create(ctx, m); err != nil {
		return nil, fmt.Errorf("failed to create migration CR %s: %w", name, err)
	}

	return m, nil
}

// simulateHubConfirmation simulates a hub sending a confirmation for a specific phase.
func simulateHubConfirmation(uid types.UID, hubName, phase string) {
	migration.SetStarted(string(uid), hubName, phase)
	migration.SetFinished(string(uid), hubName, phase)
}

// cleanupMigrationCR removes a migration CR - deletion waiting should be handled by Eventually in test
func cleanupMigrationCR(ctx context.Context, name, namespace string) error {
	migration := &migrationv1alpha1.ManagedClusterMigration{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
	}

	// Delete the migration
	if err := mgr.GetClient().Delete(ctx, migration); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete migration %s: %w", name, err)
	}

	// Verify migration is actually deleted
	Eventually(func() bool {
		err := mgr.GetClient().Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, migration)
		return err != nil
	}, "20s", "200ms").Should(BeTrue(), "migration should be deleted")

	return nil
}

func setValidatingFinished(m *migrationv1alpha1.ManagedClusterMigration, fromHubName, toHubName string, custerList []string) {
	mig := &migrationv1alpha1.ManagedClusterMigration{}
	Eventually(func() bool {
		err := mgr.GetClient().Get(ctx, client.ObjectKey{Name: m.Name, Namespace: m.Namespace}, mig)
		if err != nil {
			return false
		}
		if meta.IsStatusConditionTrue(mig.Status.Conditions, migrationv1alpha1.ConditionTypeValidated) {
			return true
		}
		if mig.Status.Phase == migrationv1alpha1.PhaseValidating {
			migration.SetStarted(string(m.GetUID()), fromHubName, migrationv1alpha1.PhaseValidating)
			migration.SetFinished(string(m.GetUID()), fromHubName, migrationv1alpha1.PhaseValidating)
			migration.SetClusterList(string(m.GetUID()), custerList)
			migration.SetStarted(string(m.GetUID()), toHubName, migrationv1alpha1.PhaseValidating)
			migration.SetFinished(string(m.GetUID()), toHubName, migrationv1alpha1.PhaseValidating)
			return true
		}
		return false
	}, "10s", "200ms").Should(BeTrue(), "migration should be deleted")
}
