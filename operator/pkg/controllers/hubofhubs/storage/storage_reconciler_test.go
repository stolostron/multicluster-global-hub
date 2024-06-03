package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	postgresv1beta1 "github.com/crunchydata/postgres-operator/pkg/apis/postgres-operator.crunchydata.com/v1beta1"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/condition"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	operatorutils "github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/test/pkg/testpostgres"
)

var (
	cfg           *rest.Config
	kubeClient    kubernetes.Interface
	runtimeClient client.Client
	runtimeMgr    ctrl.Manager
	testPostgres  *testpostgres.TestPostgres
	namespace     = "default"
	ctx           context.Context
	cancel        context.CancelFunc
)

func TestMain(m *testing.M) {
	ctx, cancel = context.WithCancel(context.Background())
	err := os.Setenv("POD_NAMESPACE", namespace)
	if err != nil {
		panic(err)
	}

	testenv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "..", "config", "crd", "bases"),
			filepath.Join("..", "..", "..", "..", "..", "pkg", "testdata", "crds"),
		},
		ErrorIfCRDPathMissing: true,
	}
	config.SetKafkaResourceReady(true)

	testPostgres, err = testpostgres.NewTestPostgres()
	if err != nil {
		panic(err)
	}

	cfg, err = testenv.Start()
	if err != nil {
		panic(err)
	}

	kubeClient, err = kubernetes.NewForConfig(cfg)
	if err != nil {
		panic(err)
	}

	runtimeClient, err = client.New(cfg, client.Options{Scheme: config.GetRuntimeScheme()})
	if err != nil {
		panic(err)
	}

	runtimeMgr, err = ctrl.NewManager(cfg, ctrl.Options{
		Metrics: metricsserver.Options{
			BindAddress: "0", // disable the metrics serving
		}, Scheme: config.GetRuntimeScheme(),
		LeaderElection:          true,
		LeaderElectionNamespace: namespace,
		LeaderElectionID:        "549a8919.open-cluster-management.io",
	})
	if err != nil {
		panic(err)
	}

	go func() {
		err = runtimeMgr.Start(ctx)
		if err != nil {
			panic(err)
		}
	}()

	// run testings
	code := m.Run()

	cancel()

	if err = testPostgres.Stop(); err != nil {
		panic(err)
	}

	// stop testenv
	if err := testenv.Stop(); err != nil {
		panic(err)
	}
	os.Exit(code)
}

func TestBYOStorage(t *testing.T) {
	RegisterTestingT(t)
	storageReconciler := NewStorageReconciler(runtimeMgr, true)

	mgh := &v1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-mgh",
			Namespace: utils.GetDefaultNamespace(),
			Annotations: map[string]string{
				operatorconstants.AnnotationImageOverridesCM: "noexisting-cm",
			},
		},
		Spec: v1alpha4.MulticlusterGlobalHubSpec{
			DataLayer: v1alpha4.DataLayerConfig{
				Postgres: v1alpha4.PostgresConfig{
					Retention: "2y",
				},
			},
		},
	}
	Expect(runtimeClient.Create(ctx, mgh)).To(Succeed())

	// byo case
	storageSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.GHStorageSecretName,
			Namespace: utils.GetDefaultNamespace(),
		},
		Data: map[string][]byte{
			"database_uri":                   []byte(testPostgres.URI),
			"database_uri_with_readonlyuser": []byte(testPostgres.URI),
			"ca.crt":                         []byte(""),
		},
		Type: corev1.SecretTypeOpaque,
	}
	Expect(runtimeClient.Create(ctx, storageSecret)).To(Succeed())

	err := storageReconciler.Reconcile(ctx, mgh)
	Expect(err).To(Succeed())

	err = runtimeClient.Get(ctx, client.ObjectKeyFromObject(mgh), mgh)
	Expect(err).To(Succeed())

	count := 0
	for _, cond := range mgh.Status.Conditions {
		if cond.Type == condition.CONDITION_TYPE_DATABASE_INIT {
			count++
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Message).To(ContainSubstring("Database has been initialized"))
		}
	}
	Expect(count).To(Equal(1))

	err = runtimeClient.Delete(ctx, storageSecret)
	Expect(err).To(Succeed())
}

// go test -run ^TestCrunchyOperatorStorage$ github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/hubofhubs/storage -v
func TestCrunchyOperatorStorage(t *testing.T) {
	RegisterTestingT(t)
	storageReconciler := NewStorageReconciler(runtimeMgr, true)

	mgh := &v1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-mgh",
			Namespace: utils.GetDefaultNamespace(),
			Annotations: map[string]string{
				operatorconstants.AnnotationMGHInstallCrunchyOperator: "true",
			},
		},
	}

	sub, err := operatorutils.GetSubscriptionByName(ctx, runtimeClient, SubscriptionName)
	Expect(err).To(Succeed())
	Expect(sub).To(BeNil())

	// blocking until get the connection
	go func() {
		_, _ = storageReconciler.reconcileStorage(ctx, mgh)
	}()

	// the subscription
	Eventually(func() error {
		sub, err := operatorutils.GetSubscriptionByName(ctx, runtimeClient, SubscriptionName)
		if err != nil {
			return err
		}
		if sub == nil {
			return fmt.Errorf("should get the subscription %s", SubscriptionName)
		}
		return nil
	}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

	// the postgres cluster
	Eventually(func() error {
		postgresCluster := &postgresv1beta1.PostgresCluster{}
		err := runtimeClient.Get(ctx, types.NamespacedName{
			Name:      config.PostgresName,
			Namespace: utils.GetDefaultNamespace(),
		}, postgresCluster)
		if err != nil {
			return err
		}
		return nil
	}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
}

// go test -run ^TestStatefulSetStorage$ github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/hubofhubs/storage -v
func TestStatefulSetStorage(t *testing.T) {
	RegisterTestingT(t)
	storageReconciler := NewStorageReconciler(runtimeMgr, true)

	mgh := &v1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-mgh",
			Namespace:   utils.GetDefaultNamespace(),
			Annotations: map[string]string{},
		},
	}

	err := runtimeClient.Get(ctx, client.ObjectKeyFromObject(mgh), mgh)
	if err != nil && errors.IsNotFound(err) {
		Expect(runtimeClient.Create(ctx, mgh)).To(Succeed())
	} else {
		Expect(err).To(Succeed())
	}

	// blocking until get the connection
	go func() {
		_, _ = storageReconciler.reconcileStorage(ctx, mgh)
	}()

	// the subscription
	Eventually(func() error {
		statefulSet := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "multicluster-global-hub-postgres",
				Namespace: utils.GetDefaultNamespace(),
			},
		}
		err := runtimeClient.Get(ctx, client.ObjectKeyFromObject(statefulSet), statefulSet)
		if err != nil {
			return err
		}
		return nil
	}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
}
