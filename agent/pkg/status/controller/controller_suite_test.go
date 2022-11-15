// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/incarnation"
	agentscheme "github.com/stolostron/multicluster-global-hub/agent/pkg/scheme"
	statusController "github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller"
	statusbundle "github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
	"github.com/stolostron/multicluster-global-hub/pkg/compressor"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/consumer"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/producer"
)

var (
	testenv       *envtest.Environment
	cfg           *rest.Config
	ctx           context.Context
	cancel        context.CancelFunc
	mgr           ctrl.Manager
	kubeClient    client.Client
	mockCluster   *kafka.MockCluster
	kafkaConsumer *consumer.KafkaConsumer
	kafkaProducer *producer.KafkaProducer
)

var (
	compressorsMap           = map[compressor.CompressionType]compressor.Compressor{}
	msgIDBundleCreateFuncMap = map[string]status.CreateBundleFunction{
		constants.ControlInfoMsgKey:              statusbundle.NewControlInfoBundle,
		constants.ManagedClustersMsgKey:          statusbundle.NewManagedClustersStatusBundle,
		constants.ClustersPerPolicyMsgKey:        statusbundle.NewClustersPerPolicyBundle,
		constants.PolicyCompleteComplianceMsgKey: statusbundle.NewCompleteComplianceStatusBundle,
		constants.PolicyDeltaComplianceMsgKey:    statusbundle.NewDeltaComplianceStatusBundle,
		constants.SubscriptionStatusMsgKey:       statusbundle.NewSubscriptionStatusesBundle,
		constants.SubscriptionReportMsgKey:       statusbundle.NewSubscriptionReportsBundle,
		constants.PlacementRuleMsgKey:            statusbundle.NewPlacementRulesBundle,
		constants.PlacementMsgKey:                statusbundle.NewPlacementsBundle,
		constants.PlacementDecisionMsgKey:        statusbundle.NewPlacementDecisionsBundle,
	}
)

var (
	leafHubName = "hub1"
	statusTopic = "status"
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Integration Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	ctx, cancel = context.WithCancel(context.TODO())

	By("Prepare envtest environment")
	var err error
	testenv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "..", "pkg", "testdata", "crds"),
		},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err = testenv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	By("Create mock kafka cluster")
	mockCluster, err = kafka.NewMockCluster(1)
	Expect(err).NotTo(HaveOccurred())
	fmt.Fprintf(GinkgoWriter, "mock kafka bootstrap server address: %s\n", mockCluster.BootstrapServers())

	By("Start kafka producer")
	kafkaProducerConfig := &producer.KafkaProducerConfig{
		ProducerTopic:  statusTopic,
		ProducerID:     "status-producer",
		MsgSizeLimitKB: 100,
	}
	kafkaProducer, err = producer.NewKafkaProducer(&compressor.CompressorGZip{},
		mockCluster.BootstrapServers(), "", kafkaProducerConfig,
		ctrl.Log.WithName("kafka-producer"))
	Expect(err).NotTo(HaveOccurred())

	By("Start kafka consumer")
	kafkaConsumerConfig := &consumer.KafkaConsumerConfig{
		ConsumerTopic: statusTopic,
		ConsumerID:    "status-consumer",
	}
	kafkaConsumer, err = consumer.NewKafkaConsumer(
		mockCluster.BootstrapServers(), "", kafkaConsumerConfig,
		ctrl.Log.WithName("kafka-consumer"))
	Expect(err).NotTo(HaveOccurred())
	// go kafkaConsumer.Start()

	By("Create controller-runtime manager")
	mgr, err = ctrl.NewManager(cfg, ctrl.Options{
		MetricsBindAddress: "0",
		Scheme:             scheme.Scheme,
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(mgr).NotTo(BeNil())

	By("Add to Scheme")
	Expect(agentscheme.AddToScheme(mgr.GetScheme())).NotTo(HaveOccurred())

	By("Get kubeClient")
	kubeClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(kubeClient).NotTo(BeNil())

	By("Create global hub system namespace")
	mghSystemNamespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: constants.GHSystemNamespace}}
	Expect(kubeClient.Create(ctx, mghSystemNamespace)).Should(Succeed())

	By("Create configmap that contains the agent sync-intervals configurations")
	syncerIntervalsConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sync-intervals",
			Namespace: constants.GHSystemNamespace,
		},
		Data: map[string]string{"control_info": "5s", "managed_clusters": "5s", "policies": "5s"},
	}
	Expect(kubeClient.Create(ctx, syncerIntervalsConfigMap)).Should(Succeed())

	By("Get agent incarnation from manager")
	incarnation, err := incarnation.GetIncarnation(mgr)
	Expect(err).NotTo(HaveOccurred())

	By("Add controllers to manager")
	err = statusController.AddControllers(mgr, kafkaProducer, leafHubName, 100, incarnation)
	Expect(err).NotTo(HaveOccurred())

	By("Add kafka producer to manager")
	Expect(mgr.Add(kafkaProducer)).Should(Succeed())

	By("Start the manager")
	go func() {
		defer GinkgoRecover()
		Expect(mgr.Start(ctx)).ToNot(HaveOccurred(), "failed to run manager")
	}()

	By("Waiting for the manager to be ready")
	Expect(mgr.GetCache().WaitForCacheSync(ctx)).To(BeTrue())
})

var _ = AfterSuite(func() {
	cancel()
	mockCluster.Close()

	By("Tearing down the test environment")
	err := testenv.Stop()
	// https://github.com/kubernetes-sigs/controller-runtime/issues/1571
	// Set 4 with random
	if err != nil {
		time.Sleep(4 * time.Second)
	}
	Expect(testenv.Stop()).NotTo(HaveOccurred())
})

func processKafkaMessage(message *kafka.Message) (int32, kafka.Offset, string, status.Bundle, error) {
	compressionType := compressor.NoOp
	for _, header := range message.Headers {
		if header.Key == transport.CompressionType {
			compressionType = compressor.CompressionType(header.Value)
		}
	}

	msgCompressor, found := compressorsMap[compressionType]
	if !found {
		newCompressor, err := compressor.NewCompressor(compressionType)
		if err != nil {
			return message.TopicPartition.Partition, message.TopicPartition.Offset, "",
				nil, fmt.Errorf("failed to create compressor: %w", err)
		}

		msgCompressor = newCompressor
		compressorsMap[compressionType] = msgCompressor
	}

	decompressedBytes, err := msgCompressor.Decompress(message.Value)
	if err != nil {
		return message.TopicPartition.Partition, message.TopicPartition.Offset, "", nil,
			fmt.Errorf("failed to decompress message: %w", err)
	}

	transportMessage := &transport.Message{}
	if err := json.Unmarshal(decompressedBytes, transportMessage); err != nil {
		return message.TopicPartition.Partition, message.TopicPartition.Offset, "", nil,
			fmt.Errorf("failed to unmarshal transport message: %w", err)
	}

	// get msgID
	msgIDTokens := strings.Split(transportMessage.ID, ".") // object id is LH_ID.MSG_ID
	if len(msgIDTokens) != 2 {
		return message.TopicPartition.Partition, message.TopicPartition.Offset, "", nil,
			fmt.Errorf("bad message ID format: %s", transportMessage.ID)
	}

	msgID := msgIDTokens[1]
	if _, found := msgIDBundleCreateFuncMap[msgID]; !found {
		return message.TopicPartition.Partition, message.TopicPartition.Offset, msgID, nil,
			fmt.Errorf("no bundle-registration available for massage ID: %s", transportMessage.ID)
	}

	receivedBundle := msgIDBundleCreateFuncMap[msgID]()
	if err := json.Unmarshal(transportMessage.Payload, receivedBundle); err != nil {
		return message.TopicPartition.Partition, message.TopicPartition.Offset, msgID, nil,
			fmt.Errorf("failed to unmarshal bundle: %w", err)
	}

	return message.TopicPartition.Partition, message.TopicPartition.Offset, msgID, receivedBundle, nil
}
