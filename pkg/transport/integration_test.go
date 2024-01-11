package transport_test

import (
	"context"
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/cluster"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/compressor"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/statistics"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/consumer"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/producer"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/registration"
)

var _ = Describe("Transport Integration", Ordered, func() {
	ctx := context.Background()
	It("Should get the message without conflation", func() {
		By("Create a kafka producer client")
		kafkaProducer, err := producer.NewKafkaProducer(&compressor.CompressorGZip{},
			&transport.KafkaConfig{
				BootstrapServer: mockCluster.BootstrapServers(),
				EnableTLS:       false,
				ProducerConfig: &transport.KafkaProducerConfig{
					ProducerTopic:      "spec",
					ProducerID:         "spec-producer",
					MessageSizeLimitKB: 100,
				},
			}, ctrl.Log.WithName("kafka-producer"))
		Expect(err).NotTo(HaveOccurred())
		go kafkaProducer.Start(ctx)

		By("Start kafka consumer")
		kafkaConsumer, err := consumer.NewKafkaConsumer(&transport.KafkaConfig{
			BootstrapServer: mockCluster.BootstrapServers(),
			EnableTLS:       false,
			ConsumerConfig: &transport.KafkaConsumerConfig{
				ConsumerTopic: "spec",
				ConsumerID:    "spec-consumer",
			},
		}, ctrl.Log.WithName("kafka-consumer"))
		Expect(err).NotTo(HaveOccurred())
		kafkaConsumer.SetLeafHubName("hub1")
		go kafkaConsumer.Start(ctx)

		By("Send message to create PlacementRule")
		kafkaProducer.SendAsync(&transport.Message{
			Key:     "PlacementRule", // entry.transportBundleKey
			MsgType: constants.SpecBundle,
			Payload: []byte(`{
				"objects": [
				  {
					"kind": "PlacementRule",
					"apiVersion": "apps.open-cluster-management.io/v1",
					"metadata": {
					  "name": "placement-policy-limitrange",
					  "namespace": "default",
					  "creationTimestamp": "2022-10-26T08:32:29Z"
					},
					"spec": {
					  "clusterSelector": {
						"matchExpressions": [
						  {
							"key": "global-policy",
							"operator": "In",
							"values": [
							  "test"
							]
						  }
						]
					  },
					  "clusterConditions": [
						{
						  "type": "ManagedClusterConditionAvailable",
						  "status": "True"
						}
					  ]
					},
					"status": {
			  
					}
				  }
				],
				"deletedObjects": [
				]
			  }`),
		})

		Eventually(func() bool {
			_, ok := <-kafkaConsumer.GetGenericBundleChan()
			return ok
		}, 1*time.Second, 100*time.Millisecond).Should(BeTrue())

		By("Send message to delete PlacementRule")
		kafkaProducer.SendAsync(&transport.Message{
			Key:     "PlacementRule", // entry.transportBundleKey
			MsgType: constants.SpecBundle,
			Payload: []byte(`{
					"objects": [
					],
					"deletedObjects": [
					  {
						"kind": "PlacementRule",
						"apiVersion": "apps.open-cluster-management.io/v1",
						"metadata": {
						  "name": "placement-policy-limitrange",
						  "namespace": "default",
						  "uid": "a789c8c1-137b-4b78-9412-9f101b08cc91",
						  "creationTimestamp": "2022-10-26T08:04:15Z"
						},
						"spec": {
						  "clusterSelector": {
							"matchExpressions": [
							  {
								"key": "global-policy",
								"operator": "In",
								"values": [
								  "test"
								]
							  }
							]
						  },
						  "clusterConditions": [
							{
							  "type": "ManagedClusterConditionAvailable",
							  "status": "True"
							}
						  ]
						},
						"status": {
						}
					  }
					]
				  }`),
		})

		Eventually(func() bool {
			_, ok := <-kafkaConsumer.GetGenericBundleChan()
			return ok
		}, 1*time.Second, 100*time.Millisecond).Should(BeTrue())
	})

	It("Should get the message ith conflation", func() {
		By("Create kafka producer client")
		kafkaProducer, err := producer.NewKafkaProducer(&compressor.CompressorGZip{},
			&transport.KafkaConfig{
				BootstrapServer: mockCluster.BootstrapServers(),
				EnableTLS:       false,
				ProducerConfig: &transport.KafkaProducerConfig{
					ProducerTopic:      "status",
					ProducerID:         "status-producer",
					MessageSizeLimitKB: 1,
				},
			}, ctrl.Log.WithName("kafka-producer"))
		Expect(err).NotTo(HaveOccurred())
		go kafkaProducer.Start(ctx)

		By("Start kafka consumer")
		kafkaConsumer, err := consumer.NewKafkaConsumer(&transport.KafkaConfig{
			BootstrapServer: mockCluster.BootstrapServers(),
			EnableTLS:       false,
			ConsumerConfig: &transport.KafkaConsumerConfig{
				ConsumerTopic: "status",
				ConsumerID:    "status-consumer",
			},
		}, ctrl.Log.WithName("kafka-consumer"))
		Expect(err).NotTo(HaveOccurred())

		stats := statistics.NewStatistics(&statistics.StatisticsConfig{}, []string{"ManagedClustersStatusBundle"})
		conflationReadyQueue := conflator.NewConflationReadyQueue(stats)
		conflationManager := conflator.NewConflationManager(
			conflationReadyQueue, stats) // manage all Conflation Units

		// register the heartbeat
		conflationManager.Register(conflator.NewConflationRegistration(
			conflator.HubClusterHeartbeatPriority,
			metadata.CompleteStateMode,
			bundle.GetBundleType(cluster.NewManagerHubClusterHeartbeatBundle()),
			func(ctx context.Context, bundle bundle.ManagerBundle) error {
				return nil
			},
		))

		conflationManager.Register(conflator.NewConflationRegistration(
			conflator.ManagedClustersPriority,
			metadata.CompleteStateMode,
			"ManagedClustersStatusBundle",
			func(ctx context.Context, bundle bundle.ManagerBundle) error {
				return nil
			},
		))
		kafkaConsumer.SetCommitter(consumer.NewCommitter(100*time.Second, "status", kafkaConsumer.Consumer(),
			conflationManager.GetTransportMetadatas, ctrl.Log.WithName("kafka-consumer")),
		)
		kafkaConsumer.SetStatistics(stats)
		kafkaConsumer.SetConflationManager(conflationManager)
		go kafkaConsumer.Start(ctx)

		kafkaConsumer.BundleRegister(&registration.BundleRegistration{
			MsgID:            constants.ManagedClustersMsgKey,
			CreateBundleFunc: cluster.NewManagerManagedClusterBundle,
			Predicate:        func() bool { return true }, // always get managed clusters bundles
		})

		By("Test the size of message is greater than the message limit")
		// https://github.com/stolostron/multicluster-global-hub/blob/main/agent/pkg/status/controller/
		// managedclusters/clusters_status_sync.go#L46
		statusBundle := &GenericStatusBundle{
			Objects:       make([]Object, 0),
			LeafHubName:   "hub1",
			BundleVersion: metadata.NewBundleVersion(),
		}
		cluster := &clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "hub1-cluster1",
			},
			Spec: clusterv1.ManagedClusterSpec{
				ManagedClusterClientConfigs: []clusterv1.ClientConfig{
					{
						URL: "https://hub1-cluster2-control-plane:6443",
					},
				},
				HubAcceptsClient:     true,
				LeaseDurationSeconds: 60,
			},
		}
		statusBundle.BundleVersion.Incr()
		statusBundle.Objects = append(statusBundle.Objects, cluster)
		payload, err := json.Marshal(statusBundle)
		Expect(err).NotTo(HaveOccurred())

		kafkaProducer.SendAsync(&transport.Message{
			Key:     "hub1.ManagedClusters", // entry.transportBundleKey
			MsgType: constants.StatusBundle,
			Payload: payload,
		})

		Eventually(func() bool {
			b, _, _, err := conflationReadyQueue.BlockingDequeue().GetNext()
			return err == nil && b != nil
		}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())
	})
})

type GenericStatusBundle struct {
	Objects       []Object                `json:"objects"`
	LeafHubName   string                  `json:"leafHubName"`
	BundleVersion *metadata.BundleVersion `json:"bundleVersion"`
}

type Object interface {
	metav1.Object
	runtime.Object
}
