package migration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ctrl "sigs.k8s.io/controller-runtime"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/consumer"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/controller"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/producer"
)

var (
	testenv         *envtest.Environment
	runtimeClient   runtimeclient.Client
	ctx             context.Context
	cancel          context.CancelFunc
	transportClient *controller.TransportClient
	specConsumer    *consumer.GenericConsumer
	transportConfig *transport.TransportInternalConfig
	receivedEvents  []*cloudevents.Event
)

func TestMigration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Agent Migration Integration Test Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.Background())

	By("bootstrapping test environment")
	testenv = &envtest.Environment{
		CRDInstallOptions: envtest.CRDInstallOptions{
			Paths: []string{
				filepath.Join("..", "..", "..", "manifest", "crd"),
				filepath.Join("..", "..", "..", "..", "operator", "config", "crd", "bases"),
			},
			MaxTime: 1 * time.Minute,
		},
	}

	cfg, err := testenv.Start()
	Expect(err).NotTo(HaveOccurred())

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Metrics: metricsserver.Options{
			BindAddress: "0", // disable the metrics serving
		},
		Scheme: configs.GetRuntimeScheme(),
	})
	Expect(err).NotTo(HaveOccurred())
	runtimeClient = mgr.GetClient()

	transportConfig = &transport.TransportInternalConfig{
		TransportType: string(transport.Chan),
		KafkaCredential: &transport.KafkaConfig{
			SpecTopic:   "spec",
			StatusTopic: "status",
		},
	}

	agentConfig := &configs.AgentConfig{
		TransportConfig:  transportConfig,
		SpecWorkPoolSize: 2,
		LeafHubName:      "test-hub",
	}

	genericProducer, err := producer.NewGenericProducer(
		agentConfig.TransportConfig,
		agentConfig.TransportConfig.KafkaCredential.SpecTopic,
		nil,
	)
	Expect(err).NotTo(HaveOccurred())

	specConsumer, err = consumer.NewGenericConsumer(
		agentConfig.TransportConfig,
		[]string{agentConfig.TransportConfig.KafkaCredential.SpecTopic},
	)
	Expect(mgr.Add(specConsumer)).To(Succeed())
	Expect(err).NotTo(HaveOccurred())

	transportClient = &controller.TransportClient{}
	// transportClient.SetConsumer(genericConsumer)
	transportClient.SetProducer(genericProducer)

	go func() {
		defer GinkgoRecover()
		for event := range specConsumer.EventChan() {
			receivedEvents = append(receivedEvents, event)
		}
	}()

	go func() {
		Expect(mgr.Start(ctx)).NotTo(HaveOccurred())
	}()

	By("Waiting for the manager to be ready")
	Expect(mgr.GetCache().WaitForCacheSync(ctx)).To(BeTrue())
})

var _ = AfterSuite(func() {
	cancel()

	By("Tearing down the test environment")
	err := testenv.Stop()
	// https://github.com/kubernetes-sigs/controller-runtime/issues/1571
	// Set 4 with random
	if err != nil {
		time.Sleep(4 * time.Second)
	}
	Expect(testenv.Stop()).NotTo(HaveOccurred())
})

func verifyMigrationEvent(expectedSource, expectedType, expectedCluster, expectedMigrationID, expectedStage string) error {
	for _, event := range receivedEvents {
		if event.Source() == expectedSource &&
			event.Type() == expectedType &&
			event.Extensions()[constants.CloudEventExtensionKeyClusterName] == expectedCluster {

			var migrationBundle migration.MigrationStatusBundle
			if err := json.Unmarshal(event.Data(), &migrationBundle); err != nil {
				return err
			}

			if migrationBundle.MigrationId == expectedMigrationID &&
				migrationBundle.Stage == expectedStage {
				return nil
			}
		}
	}
	return fmt.Errorf("expected migration event not found: source=%s, type=%s, migration=%s, stage=%s",
		expectedSource, expectedType, expectedMigrationID, expectedStage)
}

func verifyDeployingEvent(expectedSource, expectedMigrationID string) error {
	for _, event := range receivedEvents {
		if event.Source() == expectedSource &&
			event.Type() == constants.MigrationTargetMsgKey {

			var migrationBundle migration.MigrationResourceBundle
			if err := json.Unmarshal(event.Data(), &migrationBundle); err != nil {
				return err
			}

			if migrationBundle.MigrationId == expectedMigrationID {
				return nil
			}
		}
	}
	return fmt.Errorf("expected deploying event not found: source=%s, migration=%s",
		expectedSource, expectedMigrationID)
}
