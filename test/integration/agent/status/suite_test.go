package status

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/generic"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/syncers/configmap"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/syncers/events"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/syncers/managedcluster"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/syncers/managedhub"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/syncers/policies"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	genericconsumer "github.com/stolostron/multicluster-global-hub/pkg/transport/consumer"
	genericproducer "github.com/stolostron/multicluster-global-hub/pkg/transport/producer"
)

const (
	ManagedClusterTopic = "ManagedCluster"
	HeartBeatTopic      = "HeartBeat"
	HubClusterInfoTopic = "HubCluster"
	EventTopic          = "Event"
)

var (
	ctx     context.Context
	cancel  context.CancelFunc
	testenv *envtest.Environment

	leafHubName    = "hub1"
	runtimeClient  client.Client
	chanTransport  *ChanTransport
	receivedEvents map[string]*cloudevents.Event
	agentConfig    *configs.AgentConfig
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Status Controller Integration Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	ctx, cancel = context.WithCancel(context.Background())

	testenv = &envtest.Environment{
		CRDInstallOptions: envtest.CRDInstallOptions{
			Paths: []string{
				filepath.Join("..", "..", "..", "manifest", "crd"),
			},
			MaxTime: 1 * time.Minute,
		},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := testenv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	agentConfig = &configs.AgentConfig{
		PodNamespace: constants.GHAgentNamespace,
		LeafHubName:  leafHubName,
		TransportConfig: &transport.TransportInternalConfig{
			CommitterInterval: 1 * time.Second,
			TransportType:     string(transport.Chan),
			KafkaCredential: &transport.KafkaConfig{
				SpecTopic:   "spec",
				StatusTopic: "event",
			},
		},
		EnableGlobalResource: true,
	}
	configs.SetAgentConfig(agentConfig)
	configmap.SetInterval(configmap.GetSyncKey(enum.HubClusterHeartbeatType), 2*time.Second)
	configmap.SetInterval(configmap.GetSyncKey(enum.HubClusterInfoType), 2*time.Second)

	By("Create controller-runtime manager")
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Metrics: metricsserver.Options{
			BindAddress: "0", // disable the metrics serving
		}, Scheme: configs.GetRuntimeScheme(),
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(mgr).NotTo(BeNil())

	By("Create the configmap to disable the heartbeat on the suite test")
	runtimeClient, err = client.New(cfg, client.Options{Scheme: configs.GetRuntimeScheme()})
	Expect(err).NotTo(HaveOccurred())
	mghSystemNamespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: constants.GHAgentNamespace}}
	Expect(runtimeClient.Create(ctx, mghSystemNamespace)).Should(Succeed())
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: constants.GHAgentNamespace,
			Name:      constants.GHAgentConfigCMName,
		},
		Data: map[string]string{
			// "hubClusterHeartbeat": "5m",
			"hubClusterHeartbeat": "5s",
			"hubClusterInfo":      "2s",
		},
	}
	Expect(runtimeClient.Create(ctx, configMap)).Should(Succeed())

	By("Create cloudevents transport")
	chanTransport, err = NewChanTransport(mgr, agentConfig.TransportConfig, []string{
		ManagedClusterTopic,
		HeartBeatTopic,
		HubClusterInfoTopic,
		EventTopic,
	})
	Expect(err).To(Succeed())
	By("Start the manager")
	go func() {
		defer GinkgoRecover()
		Expect(mgr.Start(ctx)).ToNot(HaveOccurred(), "failed to run manager")
	}()

	By("Waiting for the manager to be ready")
	Expect(mgr.GetCache().WaitForCacheSync(ctx)).To(BeTrue())
	By("Add syncers")
	// start periodic syncer
	periodicSyncer, err := generic.AddPeriodicSyncer(mgr)
	Expect(err).Should(Succeed())

	// policy
	err = policies.LaunchPolicySyncer(ctx, mgr, agentConfig, chanTransport.Producer(EventTopic))
	Expect(err).To(Succeed())
	err = policies.AddPolicySyncer(ctx, mgr, chanTransport.Producer(EventTopic), periodicSyncer, agentConfig)
	Expect(err).To(Succeed())

	// hubcluster info
	err = configmap.AddConfigMapController(mgr, agentConfig)
	Expect(err).Should(Succeed())
	err = managedhub.LaunchHubClusterHeartbeatSyncer(mgr, chanTransport.Producer(HeartBeatTopic))
	Expect(err).Should(Succeed())
	err = managedhub.LaunchHubClusterInfoSyncer(mgr, chanTransport.Producer(HubClusterInfoTopic))
	Expect(err).Should(Succeed())

	// managed cluster
	err = managedcluster.AddManagedClusterSyncer(ctx, mgr, chanTransport.Producer(ManagedClusterTopic), periodicSyncer)
	Expect(err).To(Succeed())

	// event
	err = events.AddEventSyncer(ctx, mgr, chanTransport.Producer(EventTopic), periodicSyncer)
	Expect(err).To(Succeed())
	receivedEvents = make(map[string]*cloudevents.Event)
	go func() {
		for {
			select {
			case evt, ok := <-chanTransport.Consumer(EventTopic).EventChan():
				if !ok {
					fmt.Println("event channel closed, exiting...")
					return
				}
				fmt.Println("========== received event: ", evt.Type())
				receivedEvents[evt.Type()] = evt
			case <-ctx.Done():
				fmt.Println("context canceled, exiting...")
				return
			}
		}
	}()
})

var _ = AfterSuite(func() {
	cancel()

	By("Tearing down the test environment")
	err := testenv.Stop()
	// https://github.com/kubernetes-sigs/controller-runtime/issues/1571
	// Set 4 with random
	if err != nil {
		time.Sleep(4 * time.Second)
		Expect(testenv.Stop()).NotTo(HaveOccurred())
	}
})

type ChanTransport struct {
	consumers map[string]transport.Consumer
	producers map[string]transport.Producer
}

func (t *ChanTransport) Consumer(topic string) transport.Consumer {
	return t.consumers[topic]
}

func (t *ChanTransport) Producer(topic string) transport.Producer {
	return t.producers[topic]
}

func NewChanTransport(mgr ctrl.Manager, transConfig *transport.TransportInternalConfig, topics []string) (
	*ChanTransport, error,
) {
	trans := &ChanTransport{
		consumers: map[string]transport.Consumer{},
		producers: map[string]transport.Producer{},
	}

	for _, topic := range topics {

		// mock the consumer in manager
		transConfig.EnableDatabaseOffset = false
		transConfig.KafkaCredential.StatusTopic = topic
		consumer, err := genericconsumer.NewGenericConsumer(transConfig, []string{topic})
		if err != nil {
			return trans, err
		}
		go func() {
			if err := consumer.Start(ctx); err != nil {
				logf.Log.Error(err, "error to start the chan consumer")
			}
		}()
		Expect(err).NotTo(HaveOccurred())

		// mock the producer in agent
		producer, err := genericproducer.NewGenericProducer(transConfig, topic, nil)
		if err != nil {
			return trans, err
		}

		trans.consumers[topic] = consumer
		trans.producers[topic] = producer
	}
	return trans, nil
}
