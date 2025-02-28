package kessel

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"testing"
	"time"

	kafka_confluent "github.com/cloudevents/sdk-go/protocol/kafka_confluent/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	ceclient "github.com/cloudevents/sdk-go/v2/client"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	operatorconfig "github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	transportconfig "github.com/stolostron/multicluster-global-hub/pkg/transport/config"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/requester"
	sampleconfig "github.com/stolostron/multicluster-global-hub/samples/config"
)

const (
	TIMEOUT  = 30 * time.Second
	INTERVAL = 1 * time.Second
)

var (
	ctx             context.Context
	cancel          context.CancelFunc
	optionsFile     string
	options         *Options
	inventoryClient *requester.InventoryClient
	runtimeClient   client.Client
	log             = logger.DefaultZapLogger()
	receivedEvents  = make(map[string]*cloudevents.Event)
)

func TestClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Client Suite")
}

func init() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	flag.StringVar(&optionsFile, "options", "", "\"options-kessel.yaml\" file to provide input for the tests")
}

var _ = BeforeSuite(func() {
	ctx, cancel = context.WithCancel(context.Background())

	// Get the options
	data, err := os.ReadFile(optionsFile)
	Expect(err).NotTo(HaveOccurred())

	testOptionsContainer := &OptionsContainer{}
	err = yaml.UnmarshalStrict([]byte(data), testOptionsContainer)
	Expect(err).NotTo(HaveOccurred())

	options = &testOptionsContainer.Options

	// Create runtime client
	config, err := clientcmd.BuildConfigFromFlags("", options.Kubeconfig)
	Expect(err).To(Succeed())

	runtimeClient, err = client.New(config, client.Options{Scheme: operatorconfig.GetRuntimeScheme()})
	Expect(err).To(Succeed())

	// Create invenotry client
	transportConfigSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      options.TransportConfigName,
			Namespace: options.Namespace,
		},
	}
	err = runtimeClient.Get(ctx, client.ObjectKeyFromObject(transportConfigSecret), transportConfigSecret)
	Expect(err).To(Succeed())

	restConfig, err := transportconfig.GetRestfulConnBySecret(transportConfigSecret, runtimeClient)
	Expect(err).To(Succeed())

	inventoryClient, err = requester.NewInventoryClient(ctx, restConfig)
	Expect(err).To(Succeed())

	// Start the event consumer
	kafkaConfigMap, err := sampleconfig.GetConfluentConfigMapByUser(runtimeClient,
		options.Namespace, options.KafkaCluster, options.KafkaUser)
	Expect(err).To(Succeed())

	transportconfig.SetConsumerConfig(kafkaConfigMap, fmt.Sprintf("%s-%d", options.KafkaUser, rand.Intn(1000000)))

	receiver, err := kafka_confluent.New(kafka_confluent.WithConfigMap(kafkaConfigMap),
		kafka_confluent.WithReceiverTopics([]string{options.KafkaTopic}))
	Expect(err).To(Succeed())
	defer receiver.Close(ctx)

	c, err := cloudevents.NewClient(receiver, cloudevents.WithTimeNow(), cloudevents.WithUUIDs(),
		ceclient.WithPollGoroutines(1), ceclient.WithBlockingCallback())
	Expect(err).To(Succeed())
	defer receiver.Close(ctx)

	var mutex sync.Mutex
	go func() {
		err = c.StartReceiver(ctx, func(ctx context.Context, event cloudevents.Event) {
			mutex.Lock()
			defer mutex.Unlock()
			receivedEvents[event.Type()] = &event
			fmt.Println(event)
		})
		if err != nil {
			log.Fatalf("failed to start receiver: %v", err)
		}
	}()
})

var _ = AfterSuite(func() {
	cancel()
})

type OptionsContainer struct {
	Options Options `yaml:"options"`
}

// Define options available for Tests to consume
type Options struct {
	Kubeconfig          string `yaml:"kubeconfig"`
	Namespace           string `yaml:"namespace"`
	TransportConfigName string `yaml:"transportconfig"`
	KafkaUser           string `yaml:"kafkauser"`
	KafkaTopic          string `yaml:"kafkatopic"`
	KafkaCluster        string `yaml:"kafkacluster"`
	RelationsHTTPURL    string `yaml:"relationshttpurl"`
}
