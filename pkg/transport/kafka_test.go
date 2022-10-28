package transport_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ctrl "sigs.k8s.io/controller-runtime"

	statusbundle "github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/registration"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
	"github.com/stolostron/multicluster-global-hub/pkg/compressor"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/statistics"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/consumer"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/producer"
)

var _ = Describe("Transport", Ordered, func() {
	It("Test consumer without conflation to handle the message", func() {
		By("Start kafka producer")
		kafkaProducerConfig := &producer.KafkaProducerConfig{
			ProducerTopic:  "spec",
			ProducerID:     "spec-producer",
			MsgSizeLimitKB: 100,
		}
		kafkaProducer, err := producer.NewKafkaProducer(&compressor.CompressorGZip{},
			mockCluster.BootstrapServers(), "", kafkaProducerConfig,
			ctrl.Log.WithName("kafka-producer"))
		Expect(err).NotTo(HaveOccurred())
		go kafkaProducer.Start()

		By("Start kafka consumer")
		kafkaConsumerConfig := &consumer.KafkaConsumerConfig{
			ConsumerTopic: "spec",
			ConsumerID:    "spec-consumer",
		}
		kafkaConsumer, err := consumer.NewKafkaConsumer(
			mockCluster.BootstrapServers(), "", kafkaConsumerConfig,
			ctrl.Log.WithName("kafka-consumer"))
		Expect(err).NotTo(HaveOccurred())
		kafkaConsumer.SetLeafHubName("hub1")
		go kafkaConsumer.Start()

		By("Send message to create PlacementRule")
		kafkaProducer.SendAsync(&transport.Message{
			ID:      "PlacementRule", // entry.transportBundleKey
			MsgType: constants.SpecBundle,
			Version: "2022-10-26_08-32-00.739891", // entry.bundle.GetBundleVersion().String()
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
			ID:      "PlacementRule", // entry.transportBundleKey
			MsgType: constants.SpecBundle,
			Version: "2022-10-26_08-32-00.739891", // entry.bundle.GetBundleVersion().String()
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

		By("Send message to update managedcluster")
		kafkaProducer.SendAsync(&transport.Message{
			Destination: "hub1",
			ID:          "update", // entry.transportBundleKey
			MsgType:     constants.SpecBundle,
			Version:     "2022-10-26_08-32-00.739891", // entry.bundle.GetBundleVersion().String()
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
					  "uid": "a789c8c1-137b-4b78-9412-9f101b08cc91"
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

	It("Test consumer with conflation to handle the message", func() {
		By("Start kafka producer")
		kafkaProducerConfig := &producer.KafkaProducerConfig{
			ProducerTopic:  "status",
			ProducerID:     "status-producer",
			MsgSizeLimitKB: 1,
		}
		kafkaProducer, err := producer.NewKafkaProducer(&compressor.CompressorGZip{},
			mockCluster.BootstrapServers(), "", kafkaProducerConfig,
			ctrl.Log.WithName("kafka-producer"))
		Expect(err).NotTo(HaveOccurred())
		go kafkaProducer.Start()

		By("Start kafka consumer")
		kafkaConsumerConfig := &consumer.KafkaConsumerConfig{
			ConsumerTopic: "status",
			ConsumerID:    "status-consumer",
		}
		kafkaConsumer, err := consumer.NewKafkaConsumer(
			mockCluster.BootstrapServers(), "", kafkaConsumerConfig,
			ctrl.Log.WithName("kafka-consumer"))
		Expect(err).NotTo(HaveOccurred())

		stats := statistics.NewStatistics(ctrl.Log.WithName("statistics"), &statistics.StatisticsConfig{},
			[]string{"ManagedClustersStatusBundle"})
		conflationReadyQueue := conflator.NewConflationReadyQueue(stats)
		conflationManager := conflator.NewConflationManager(ctrl.Log.WithName("conflation"),
			conflationReadyQueue, false, stats) // manage all Conflation Units
		conflationManager.Register(conflator.NewConflationRegistration(
			conflator.ManagedClustersPriority,
			bundle.CompleteStateMode,
			"ManagedClustersStatusBundle",
			func(ctx context.Context, bundle status.Bundle,
				dbClient database.StatusTransportBridgeDB,
			) error {
				return nil
			},
		))
		kafkaConsumer.SetCommitter(consumer.NewCommitter(
			100*time.Second, kafkaConsumerConfig.ConsumerTopic, kafkaConsumer.Consumer(),
			conflationManager.GetBundlesMetadata, ctrl.Log.WithName("kafka-consumer")),
		)
		kafkaConsumer.SetStatistics(stats)
		kafkaConsumer.SetConflationManager(conflationManager)
		go kafkaConsumer.Start()

		kafkaConsumer.BundleRegister(&registration.BundleRegistration{
			MsgID:            constants.ManagedClustersMsgKey,
			CreateBundleFunc: statusbundle.NewManagedClustersStatusBundle,
			Predicate:        func() bool { return true }, // always get managed clusters bundles
		})

		By("Test the size of message is greater than the message limit")
		kafkaProducer.SendAsync(&transport.Message{
			Key:     "hub1.ManagedClusters",
			ID:      "hub1.ManagedClusters", // entry.transportBundleKey
			MsgType: constants.StatusBundle,
			Version: "8.1", // entry.bundle.GetBundleVersion().String()
			Payload: []byte(`{
				"objects": [
				  {
					"kind": "ManagedCluster",
					"apiVersion": "cluster.open-cluster-management.io/v1",
					"metadata": {
					  "name": "hub1-cluster1"
					},
					"spec": {
					  "managedClusterClientConfigs": [
						{
						  "url": "https://hub1-cluster2-control-plane:6443",
						  "caBundle": ""
						}
					  ],
					  "hubAcceptsClient": true,
					  "leaseDurationSeconds": 60
					},
					"status": {
					}
				  },
				  {
					"kind": "ManagedCluster",
					"apiVersion": "cluster.open-cluster-management.io/v1",
					"metadata": {
					  "name": "hub1-cluster2"
					},
					"spec": {
					  "managedClusterClientConfigs": [
						{
						  "url": "https://hub1-cluster2-control-plane:6443",
						  "caBundle": ""
						}
					  ],
					  "hubAcceptsClient": true,
					  "leaseDurationSeconds": 60
					},
					"status": {
					}
				  },
				  {
					"kind": "ManagedCluster",
					"apiVersion": "cluster.open-cluster-management.io/v1",
					"metadata": {
					  "name": "hub1-cluster3"
					},
					"spec": {
					  "managedClusterClientConfigs": [
						{
						  "url": "https://hub1-cluster2-control-plane:6443",
						  "caBundle": ""
						}
					  ],
					  "hubAcceptsClient": true,
					  "leaseDurationSeconds": 60
					},
					"status": {
					}
				  },
				  {
					"kind": "ManagedCluster",
					"apiVersion": "cluster.open-cluster-management.io/v1",
					"metadata": {
					  "name": "hub1-cluster4"
					},
					"spec": {
					  "managedClusterClientConfigs": [
						{
						  "url": "https://hub1-cluster2-control-plane:6443",
						  "caBundle": ""
						}
					  ],
					  "hubAcceptsClient": true,
					  "leaseDurationSeconds": 60
					},
					"status": {
					}
				  }
				],
				"leafHubName": "hub1",
				"bundleVersion": {
				  "incarnation": 8,
				  "generation": 1
				}
			  }`),
		})

		Eventually(func() bool {
			b, _, _, err := conflationReadyQueue.BlockingDequeue().GetNext()
			return err == nil && b != nil
		}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())
	})
})
