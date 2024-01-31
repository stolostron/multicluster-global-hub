package eventcollector

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Shopify/sarama"
	"github.com/resmoio/kubernetes-event-exporter/pkg/kube"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	eventprocessor "github.com/stolostron/multicluster-global-hub/manager/pkg/eventcollector/processor"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/test/pkg/kafka"
	"github.com/stolostron/multicluster-global-hub/test/pkg/testpostgres"
)

var (
	testenv      *envtest.Environment
	cfg          *rest.Config
	ctx          context.Context
	cancel       context.CancelFunc
	mgr          ctrl.Manager
	testPostgres *testpostgres.TestPostgres
)

func TestAddEventCollector(t *testing.T) {
	ctx, cancel = context.WithCancel(context.Background())

	// By("Prepare envtest environment")
	testenv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "pkg", "testdata", "crds"),
		},
		ErrorIfCRDPathMissing: true,
	}
	var err error
	err = os.Setenv("POD_NAMESPACE", "default")
	assert.Nil(t, err, "Error should be nil")
	cfg, err = testenv.Start()
	assert.Nil(t, err, "Error should be nil")
	assert.NotNil(t, cfg, "Config should not be nil")

	defer func() {
		err = testenv.Stop()
		// https://github.com/kubernetes-sigs/controller-runtime/issues/1571
		// Set 4 with random
		if err != nil {
			time.Sleep(4 * time.Second)
			assert.Nil(t, testenv.Stop(), "Error should be nil")
		}
	}()

	// By("Prepare postgres connection pool")
	testPostgres, err = testpostgres.NewTestPostgres()
	assert.Nil(t, err, "Error should be nil")
	defer func() {
		err := testPostgres.Stop()
		assert.Nil(t, err, "Error should be nil")
	}()
	err = database.InitGormInstance(&database.DatabaseConfig{
		URL:        testPostgres.URI,
		Dialect:    database.PostgresDialect,
		CaCertPath: "test-ca-cert-path",
		PoolSize:   2,
	})
	assert.Nil(t, err, "Error should be nil")
	defer database.CloseGorm(database.GetSqlDb())

	// By("Prepare kafka cluster")
	val, err := json.Marshal(getEvent())
	assert.Nil(t, err, "Error should be nil")
	kafkaCluster := kafka.MockSaramaCluster(t, []string{string(val)})
	assert.NotNil(t, kafkaCluster, "Kafka cluster should not be nil")
	defer kafkaCluster.Close()
	kafkaConfig := &transport.KafkaConfig{
		BootstrapServer: kafkaCluster.Addr(),
		EnableTLS:       false,
		ConsumerConfig: &transport.KafkaConsumerConfig{
			ConsumerID:    "test-consumer",
			ConsumerTopic: "my_topic",
		},
	}

	// By("Create controller runtime manager")
	mgr, err = ctrl.NewManager(cfg, ctrl.Options{
		MetricsBindAddress: "0",
		Scheme:             scheme.Scheme,
	})
	assert.Nil(t, err, "Error should be nil")

	// By("Adding the controllers to the manager")
	err = AddEventCollector(ctx, mgr, kafkaConfig)
	assert.Nil(t, err, "Error should be nil")

	// By("Waiting for the manager to be ready")
	go func() {
		err = mgr.Start(ctx)
		assert.Nil(t, err, "Error should be nil")
	}()
	synced := mgr.GetCache().WaitForCacheSync(ctx)
	assert.True(t, synced, "Cache should be synced")
	time.Sleep(10 * time.Second)
	cancel()
}

func TestEventCollector(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	messageChan := make(chan *sarama.ConsumerMessage)
	eventDispatcher := newEventDispatcher(messageChan)
	eventDispatcher.RegisterProcessor(policyv1.Kind,
		eventprocessor.NewPolicyProcessor(ctx, &offsetManagerMock{}))
	go func() {
		_ = eventDispatcher.Start(ctx)
	}()
	event := getEvent()
	val, err := json.Marshal(event)
	assert.Nil(t, err, "Error should be nil")
	messageChan <- &sarama.ConsumerMessage{
		Value:     val,
		Topic:     "my_topic",
		Partition: 0,
		Offset:    0,
	}
	time.Sleep(1 * time.Second)
	cancel()
}

type offsetManagerMock struct{}

func (o *offsetManagerMock) MarkOffset(topic string, partition int32, offset int64) {
	fmt.Printf("mark offset: topic=%s, partition=%d, offset=%d\n", topic, partition, offset)
}

func getEvent() *kube.EnhancedEvent {
	e := &kube.EnhancedEvent{
		Event: corev1.Event{
			Message: "foovar",
			ObjectMeta: metav1.ObjectMeta{
				Name:      "event1",
				Namespace: "default",
			},
			Reason: "foovar",
			Source: corev1.EventSource{
				Component: "foovar",
			},
			LastTimestamp: metav1.NewTime(time.Now()),
		},
		InvolvedObject: kube.EnhancedObjectReference{
			ObjectReference: corev1.ObjectReference{
				Kind:      string(policyv1.Kind),
				Name:      "managed-cluster-policy",
				Namespace: "cluster1",
			},
			Labels: map[string]string{
				constants.PolicyEventRootPolicyIdLabelKey: "37c9a640-af05-4bea-9dcc-1873e86bebcd",
				constants.PolicyEventClusterIdLabelKey:    "47c9a640-af05-4bea-9dcc-1873e86bebcd",
				constants.PolicyEventComplianceLabelKey:   string(policyv1.Compliant),
			},
		},
	}
	return e
}
