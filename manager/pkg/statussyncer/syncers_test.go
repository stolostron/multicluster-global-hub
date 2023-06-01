package statussyncer_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/config"
	managerscheme "github.com/stolostron/multicluster-global-hub/manager/pkg/scheme"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer"
	operatorv1alpha2 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha2"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/statistics"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/consumer"
	"github.com/stolostron/multicluster-global-hub/test/pkg/testpostgres"
)

var (
	cfg              *rest.Config
	mockKafkaCluster *kafka.MockCluster
	testPostgres     *testpostgres.TestPostgres
)

func TestMain(m *testing.M) {
	var err error

	// start testenv
	testenv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "..", "..", "pkg", "testdata", "crds"),
		},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err = testenv.Start()
	if err != nil {
		panic(err)
	}

	if cfg == nil {
		panic(fmt.Errorf("empty kubeconfig!"))
	}

	// init mock kafka cluster
	mockKafkaCluster, err = kafka.NewMockCluster(1)
	if err != nil {
		panic(err)
	}

	if mockKafkaCluster == nil {
		panic(fmt.Errorf("empty mock kafka cluster!"))
	}

	// init test postgres
	testPostgres, err = testpostgres.NewTestPostgres()
	if err != nil {
		panic(err)
	}

	// run testings
	code := m.Run()

	// stop mock kafka cluster
	mockKafkaCluster.Close()

	err = testPostgres.Stop()
	if err != nil {
		panic(err)
	}

	// stop testenv
	err = testenv.Stop()
	if err != nil {
		panic(err)
	}

	os.Exit(code)
}

func TestConsumer(t *testing.T) {
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		MetricsBindAddress: "0",
		Scheme:             scheme.Scheme,
	})
	if err != nil {
		t.Errorf("failed to create runtime manager: %v", err)
	}

	if err := managerscheme.AddToScheme(mgr.GetScheme()); err != nil {
		t.Errorf("failed to add scheme: %v", err)
	}

	kubeClient, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		t.Errorf("failed to create kubeclient: %v", err)
	}

	mghSystemNamespace := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: constants.GHSystemNamespace}}
	if err := kubeClient.Create(context.TODO(), mghSystemNamespace); err != nil {
		t.Errorf("failed to create namespace: %v", err)
	}
	mghSystemConfigMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.GHConfigCMName,
			Namespace: constants.GHSystemNamespace,
			Labels:    map[string]string{constants.GlobalHubGlobalResourceLabel: ""},
		},
		Data: map[string]string{"aggregationLevel": "full", "enableLocalPolicies": "true"},
	}
	if err := kubeClient.Create(context.TODO(), mghSystemConfigMap); err != nil {
		t.Errorf("failed to create configmap: %v", err)
	}

	managerConfig := &config.ManagerConfig{
		DatabaseConfig: &config.DatabaseConfig{
			ProcessDatabaseURL:         testPostgres.URI,
			TransportBridgeDatabaseURL: testPostgres.URI,
		},
		TransportConfig: &transport.TransportConfig{
			TransportType:   string(transport.Kafka),
			TransportFormat: string(operatorv1alpha2.KafkaMessage),
			KafkaConfig: &transport.KafkaConfig{
				BootstrapServer: mockKafkaCluster.BootstrapServers(),
				EnableTLS:       false,
				ConsumerConfig: &transport.KafkaConsumerConfig{
					ConsumerID:    "hello",
					ConsumerTopic: "world",
				},
			},
		},
		StatisticsConfig: &statistics.StatisticsConfig{},
	}

	registerable, err := statussyncer.AddStatusSyncers(mgr, managerConfig)
	if err != nil {
		t.Errorf("failed to add transport to db: %v", err)
	}

	_, ok := registerable.(*consumer.KafkaConsumer)
	if !ok {
		t.Errorf("the registerable should be KafkaConsumer: %v", err)
	}
}
