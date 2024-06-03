package transport

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	. "github.com/onsi/gomega"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/hubofhubs/transport/transporter"
	operatorutils "github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var (
	cfg           *rest.Config
	runtimeClient client.Client
	runtimeMgr    ctrl.Manager
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

	cfg, err = testenv.Start()
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
		LeaderElectionID:        "549a8911.open-cluster-management.io",
	})
	if err != nil {
		panic(err)
	}

	go func() {
		if err = runtimeMgr.Start(ctx); err != nil {
			panic(err)
		}
	}()

	// run testings
	code := m.Run()

	cancel()

	// stop testenv
	if err := testenv.Stop(); err != nil {
		panic(err)
	}
	os.Exit(code)
}

func TestBYOTransport(t *testing.T) {
	RegisterTestingT(t)

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
			EnableMetrics: true,
		},
	}
	Expect(runtimeClient.Create(ctx, mgh)).To(Succeed())

	// byo case
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.GHTransportSecretName,
			Namespace: utils.GetDefaultNamespace(),
		},
		Data: map[string][]byte{
			"bootstrap_server": []byte("localhost:test"),
			"ca.crt":           []byte("ca.crt"),
			"client.crt":       []byte("client.crt"),
			"client.key":       []byte("client.key"),
		},
		Type: corev1.SecretTypeOpaque,
	}
	Expect(runtimeClient.Create(ctx, secret)).To(Succeed())

	conn := config.GetTransporterConn()
	Expect(conn).To(BeNil())

	// update the transport protocol configuration
	err := config.SetBYOKafka(ctx, runtimeClient, utils.GetDefaultNamespace())
	Expect(err).To(Succeed())

	reconciler := NewTransportReconciler(runtimeMgr)
	err = reconciler.Reconcile(ctx, mgh)
	Expect(err).To(Succeed())
	conn = config.GetTransporterConn()
	Expect(conn).NotTo(BeNil())

	err = runtimeClient.Delete(ctx, secret)
	Expect(err).To(Succeed())
}

func TestStrimziTransport(t *testing.T) {
	RegisterTestingT(t)

	// update the transport protocol configuration
	err := config.SetBYOKafka(ctx, runtimeClient, utils.GetDefaultNamespace())
	Expect(err).To(Succeed())
	config.SetKafkaResourceReady(true)

	mgh := &v1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-mgh",
			Namespace:   utils.GetDefaultNamespace(),
			Annotations: map[string]string{},
		},
		Spec: v1alpha4.MulticlusterGlobalHubSpec{
			EnableMetrics: true,
		},
	}
	err = runtimeClient.Get(ctx, client.ObjectKeyFromObject(mgh), mgh)
	if err != nil && errors.IsNotFound(err) {
		Expect(runtimeClient.Create(ctx, mgh)).To(Succeed())
	} else {
		Expect(err).To(Succeed())
	}

	Expect(config.TransporterProtocol()).To(Equal(transport.StrimziTransporter))

	// blocking until get the connection
	go func() {
		reconciler := NewTransportReconciler(runtimeMgr)
		_ = reconciler.Reconcile(ctx, mgh)
	}()

	// the subscription
	Eventually(func() error {
		sub, err := operatorutils.GetSubscriptionByName(ctx, runtimeClient, transporter.DefaultKafkaSubName)
		if err != nil {
			return err
		}
		if sub == nil {
			return fmt.Errorf("should get the subscription %s", transporter.DefaultKafkaSubName)
		}

		return nil
	}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

	// the kafka cluster
	Eventually(func() error {
		kafka := &kafkav1beta2.Kafka{}
		err := runtimeClient.Get(ctx, types.NamespacedName{
			Name:      transporter.KafkaClusterName,
			Namespace: utils.GetDefaultNamespace(),
		}, kafka)
		if err != nil {
			return err
		}
		return nil
	}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

	// get metrics resources
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kafka-metrics",
			Namespace: utils.GetDefaultNamespace(),
		},
	}
	err = runtimeClient.Get(ctx, client.ObjectKeyFromObject(cm), cm)
	Expect(err).To(Succeed())

	podMonitor := &promv1.PodMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kafka-resources-metrics",
			Namespace: utils.GetDefaultNamespace(),
		},
	}
	err = runtimeClient.Get(ctx, client.ObjectKeyFromObject(podMonitor), podMonitor)
	Expect(err).To(Succeed())
}
