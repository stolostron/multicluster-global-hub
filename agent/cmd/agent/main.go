package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/go-logr/logr"
	"github.com/operator-framework/operator-sdk/pkg/log/zap"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/controllers"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/helper"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/incarnation"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/lease"
	agentscheme "github.com/stolostron/multicluster-global-hub/agent/pkg/scheme"
	specController "github.com/stolostron/multicluster-global-hub/agent/pkg/spec/controller"
	statusController "github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller"
	"github.com/stolostron/multicluster-global-hub/pkg/compressor"
	"github.com/stolostron/multicluster-global-hub/pkg/jobs"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/consumer"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/producer"
)

const (
	metricsHost                = "0.0.0.0"
	metricsPort          int32 = 8384
	kafkaTransportType         = "kafka"
	leaderElectionLockID       = "multicluster-global-hub-agent-lock"
)

func main() {
	os.Exit(doMain(ctrl.SetupSignalHandler(), ctrl.GetConfigOrDie()))
}

// function to handle defers with exit, see https://stackoverflow.com/a/27629493/553720.
func doMain(ctx context.Context, restConfig *rest.Config) int {
	log := initLog()
	printVersion(log)
	configManager, err := helper.NewConfigManager()
	if err != nil {
		log.Error(err, "failed to load environment variable")
		return 1
	}

	if configManager.Terminating {
		if err := agentscheme.AddToScheme(scheme.Scheme); err != nil {
			log.Error(err, "failed to add to scheme")
			return 1
		}
		client, err := client.New(restConfig, client.Options{Scheme: scheme.Scheme})
		if err != nil {
			log.Error(err, "failed to int controller runtime client")
			return 1
		}
		if err := jobs.NewPruneFinalizer(ctx, client).Run(); err != nil {
			log.Error(err, "failed to prune resources finalizer")
			return 1
		}
		return 0
	}

	mgr, err := createManager(restConfig, configManager)
	if err != nil {
		log.Error(err, "failed to create manager")
		return 1
	}

	log.Info("starting the Cmd")
	if err := mgr.Start(ctx); err != nil {
		log.Error(err, "manager exited non-zero")
		return 1
	}

	return 0
}

func initLog() logr.Logger {
	ctrl.SetLogger(zap.Logger())
	log := ctrl.Log.WithName("cmd")
	return log
}

func printVersion(log logr.Logger) {
	log.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
}

// function to choose transport type based on env var.
func getConsumer(environmentManager *helper.ConfigManager) (consumer.Consumer, error) {
	switch environmentManager.TransportType {
	case kafkaTransportType:
		kafkaConsumer, err := consumer.NewKafkaConsumer(
			environmentManager.BootstrapServers, environmentManager.SslCA,
			environmentManager.ConsumerConfig, ctrl.Log.WithName("kafka-consumer"))
		if err != nil {
			return nil, fmt.Errorf("failed to create kafka-consumer: %w", err)
		}
		kafkaConsumer.SetLeafHubName(environmentManager.LeafHubName)
		return kafkaConsumer, nil
	default:
		return nil, fmt.Errorf("environment variable %q - %q is not a valid option",
			"TRANSPORT_TYPE", environmentManager.TransportType)
	}
}

func getProducer(environmentManager *helper.ConfigManager) (producer.Producer, error) {
	messageCompressor, err := compressor.NewCompressor(
		compressor.CompressionType(environmentManager.TransportCompressionType))
	if err != nil {
		return nil, fmt.Errorf("failed to create transport producer message-compressor: %w", err)
	}

	switch environmentManager.TransportType {
	case kafkaTransportType:
		kafkaProducer, err := producer.NewKafkaProducer(messageCompressor,
			environmentManager.BootstrapServers, environmentManager.SslCA,
			environmentManager.ProducerConfig, ctrl.Log.WithName("kafka-producer"))
		if err != nil {
			return nil, fmt.Errorf("failed to create kafka-producer: %w", err)
		}
		return kafkaProducer, nil
	default:
		return nil, fmt.Errorf("environment variable %q - %q is not a valid option",
			"TRANSPORT_TYPE", environmentManager.TransportType)
	}
}

func createManager(restConfig *rest.Config, environmentManager *helper.ConfigManager) (ctrl.Manager, error) {
	leaseDuration := time.Duration(environmentManager.ElectionConfig.LeaseDuration) * time.Second
	renewDeadline := time.Duration(environmentManager.ElectionConfig.RenewDeadline) * time.Second
	retryPeriod := time.Duration(environmentManager.ElectionConfig.RetryPeriod) * time.Second

	var leaderElectionConfig *rest.Config
	if isAgentTesting, ok := os.LookupEnv("AGENT_TESTING"); ok && isAgentTesting == "true" {
		leaderElectionConfig = restConfig
	} else {
		var err error
		leaderElectionConfig, err = rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
	}

	options := ctrl.Options{
		MetricsBindAddress:      fmt.Sprintf("%s:%d", metricsHost, metricsPort),
		LeaderElection:          true,
		LeaderElectionConfig:    leaderElectionConfig,
		LeaderElectionID:        leaderElectionLockID,
		LeaderElectionNamespace: environmentManager.PodNameSpace,
		LeaseDuration:           &leaseDuration,
		RenewDeadline:           &renewDeadline,
		RetryPeriod:             &retryPeriod,
	}

	mgr, err := ctrl.NewManager(restConfig, options)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new manager: %w", err)
	}

	// add scheme
	if err := agentscheme.AddToScheme(mgr.GetScheme()); err != nil {
		return nil, fmt.Errorf("failed to add schemes: %w", err)
	}

	// incarnation version
	incarnation, err := incarnation.GetIncarnation(mgr, environmentManager.PodNameSpace)
	if err != nil {
		return nil, fmt.Errorf("failed to get incarnation version: %w", err)
	}
	fmt.Printf("Starting the Cmd incarnation: %d", incarnation)

	consumer, err := getConsumer(environmentManager)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize transport consumer: %w", err)
	}

	producer, err := getProducer(environmentManager)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize transport producer: %w", err)
	}

	if err := mgr.Add(consumer); err != nil {
		return nil, fmt.Errorf("failed to add transport consumer: %w", err)
	}

	if err := mgr.Add(producer); err != nil {
		return nil, fmt.Errorf("failed to add transport producer: %w", err)
	}

	if err := specController.AddSyncersToManager(mgr, consumer, *environmentManager); err != nil {
		return nil, fmt.Errorf("failed to add spec syncer: %w", err)
	}

	if err := statusController.AddControllers(mgr, producer, *environmentManager, incarnation); err != nil {
		return nil, fmt.Errorf("failed to add status syncer: %w", err)
	}

	if err := controllers.AddToManager(mgr); err != nil {
		return nil, fmt.Errorf("failed to add controllers: %w", err)
	}

	if err := lease.AddHoHLeaseUpdater(mgr, environmentManager.PodNameSpace,
		"multicluster-global-hub-controller"); err != nil {
		return nil, fmt.Errorf("failed to add lease updater: %w", err)
	}

	return mgr, nil
}
