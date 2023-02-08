package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/go-logr/logr"
	"github.com/operator-framework/operator-sdk/pkg/log/zap"
	"github.com/spf13/pflag"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/controllers"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/incarnation"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/lease"
	agentscheme "github.com/stolostron/multicluster-global-hub/agent/pkg/scheme"
	specController "github.com/stolostron/multicluster-global-hub/agent/pkg/spec/controller"
	statusController "github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller"
	"github.com/stolostron/multicluster-global-hub/pkg/compressor"
	"github.com/stolostron/multicluster-global-hub/pkg/jobs"
	commonobjects "github.com/stolostron/multicluster-global-hub/pkg/objects"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/producer"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/protocol"
)

const (
	metricsHost                = "0.0.0.0"
	metricsPort          int32 = 8384
	kafkaTransportType         = "kafka"
	leaderElectionLockID       = "multicluster-global-hub-agent-lock"
)

func main() {
	// adding and parsing flags should be done before the call of 'ctrl.GetConfigOrDie()',
	// otherwise kubeconfig will not be passed to agent main process
	agentConfig := parseFlags()
	if agentConfig.Terminating {
		os.Exit(doTermination(ctrl.SetupSignalHandler(), ctrl.GetConfigOrDie()))
	}
	os.Exit(doMain(ctrl.SetupSignalHandler(), ctrl.GetConfigOrDie(), agentConfig))
}

func doTermination(ctx context.Context, restConfig *rest.Config) int {
	log := initLog()
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

// function to handle defers with exit, see https://stackoverflow.com/a/27629493/553720.
func doMain(ctx context.Context, restConfig *rest.Config, agentConfig *config.AgentConfig) int {
	log := initLog()

	if err := completeConfig(agentConfig); err != nil {
		log.Error(err, "failed to get regional hub configuration from command line flags")
		return 1
	}

	mgr, err := createManager(ctx, restConfig, agentConfig, log)
	if err != nil {
		log.Error(err, "failed to create manager")
		return 1
	}

	log.Info("starting the agent controller manager")
	if err := mgr.Start(ctx); err != nil {
		log.Error(err, "manager exited non-zero")
		return 1
	}
	return 0
}

func initLog() logr.Logger {
	ctrl.SetLogger(zap.Logger())
	log := ctrl.Log.WithName("cmd")
	log.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
	return log
}

func parseFlags() *config.AgentConfig {
	agentConfig := &config.AgentConfig{
		ElectionConfig: &commonobjects.LeaderElectionConfig{},
		TransportConfig: &transport.TransportConfig{
			KafkaConfig: &protocol.KafkaConfig{
				ProducerConfig: &protocol.KafkaProducerConfig{},
				ConsumerConfig: &protocol.KafkaConsumerConfig{},
			},
		},
	}

	// add flags for logger
	pflag.CommandLine.AddFlagSet(zap.FlagSet())
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	pflag.StringVar(&agentConfig.LeafHubName, "leaf-hub-name", "", "The name of the leaf hub.")
	pflag.StringVar(&agentConfig.TransportConfig.KafkaConfig.BootstrapServer, "kafka-bootstrap-server", "",
		"The bootstrap server for kafka.")
	pflag.StringVar(&agentConfig.TransportConfig.KafkaConfig.CertPath, "kafka-ca-path", "",
		"The certificate path of CA certificate for kafka bootstrap server.")
	pflag.StringVar(&agentConfig.TransportConfig.KafkaConfig.ProducerConfig.ProducerID, "kafka-producer-id", "",
		"Producer Id for the kafka, default is the leaf hub name.")
	pflag.StringVar(&agentConfig.TransportConfig.KafkaConfig.ProducerConfig.ProducerTopic, "kafka-producer-topic",
		"status", "Topic for the kafka producer.")
	pflag.IntVar(&agentConfig.TransportConfig.KafkaConfig.ProducerConfig.MessageSizeLimitKB,
		"kafka-message-size-limit", 940, "The limit for kafka message size in KB.")
	pflag.StringVar(&agentConfig.TransportConfig.KafkaConfig.ConsumerConfig.ConsumerTopic, "kafka-consumer-topic",
		"spec", "Topic for the kafka consumer.")
	pflag.StringVar(&agentConfig.TransportConfig.KafkaConfig.ConsumerConfig.ConsumerID, "kakfa-consumer-id",
		"multicluster-global-hub-agent", "ID for the kafka consumer.")
	pflag.StringVar(&agentConfig.PodNameSpace, "pod-namespace", "open-cluster-management",
		"The agent running namespace, also used as leader election namespace")
	pflag.StringVar(&agentConfig.TransportConfig.TransportType, "transport-type", "kafka",
		"The transport type, 'kafka'")
	pflag.IntVar(&agentConfig.SpecWorkPoolSize, "consumer-worker-pool-size", 10,
		"The goroutine number to propagate the bundles on managed cluster.")
	pflag.BoolVar(&agentConfig.SpecEnforceHohRbac, "enforce-hoh-rbac", false,
		"enable hoh RBAC or not, default false")
	pflag.StringVar(&agentConfig.TransportConfig.MessageCompressionType,
		"transport-message-compression-type", "gzip",
		"The message compression type for transport layer, 'gzip' or 'no-op'.")
	pflag.IntVar(&agentConfig.StatusDeltaCountSwitchFactor,
		"status-delta-count-switch-factor", 100,
		"default with 100.")
	pflag.IntVar(&agentConfig.ElectionConfig.LeaseDuration, "lease-duration", 137,
		"leader election lease duration")
	pflag.IntVar(&agentConfig.ElectionConfig.RenewDeadline, "renew-deadline", 107,
		"leader election renew deadline")
	pflag.IntVar(&agentConfig.ElectionConfig.RetryPeriod, "retry-period", 26,
		"leader election retry period")
	pflag.BoolVar(&agentConfig.Terminating, "terminating", false,
		"true is to trigger the PreStop hook to do cleanup. For example: removing finalizer")

	pflag.Parse()
	return agentConfig
}

func completeConfig(agentConfig *config.AgentConfig) error {
	if agentConfig.LeafHubName == "" {
		return fmt.Errorf("flag regional-hub-name can't be empty")
	}
	if agentConfig.TransportConfig.KafkaConfig.ProducerConfig.ProducerID == "" {
		agentConfig.TransportConfig.KafkaConfig.ProducerConfig.ProducerID = agentConfig.LeafHubName
	}
	if agentConfig.SpecWorkPoolSize < 1 ||
		agentConfig.SpecWorkPoolSize > 100 {
		return fmt.Errorf("flag consumer-worker-pool-size should be in the scope [1, 100]")
	}

	if agentConfig.TransportConfig.KafkaConfig.ProducerConfig.MessageSizeLimitKB > producer.MaxMessageSizeLimit {
		return fmt.Errorf("flag kafka-message-size-limit %d must not exceed %d",
			agentConfig.TransportConfig.KafkaConfig.ProducerConfig.MessageSizeLimitKB, producer.MaxMessageSizeLimit)
	}
	agentConfig.TransportConfig.KafkaConfig.EnableTSL = true
	return nil
}

func getProducer(agentConfig *config.AgentConfig) (producer.Producer, error) {
	messageCompressor, err := compressor.NewCompressor(
		compressor.CompressionType(agentConfig.TransportConfig.MessageCompressionType))
	if err != nil {
		return nil, fmt.Errorf("failed to create transport producer message-compressor: %w", err)
	}

	switch agentConfig.TransportConfig.TransportType {
	case kafkaTransportType:
		kafkaProducer, err := producer.NewKafkaProducer(messageCompressor,
			agentConfig.TransportConfig.KafkaConfig.BootstrapServer,
			agentConfig.TransportConfig.KafkaConfig.CertPath,
			agentConfig.TransportConfig.KafkaConfig.ProducerConfig, ctrl.Log.WithName("kafka-producer"))
		if err != nil {
			return nil, fmt.Errorf("failed to create kafka-producer: %w", err)
		}
		return kafkaProducer, nil
	default:
		return nil, fmt.Errorf("flag transport-type - %q is not a valid option",
			agentConfig.TransportConfig.TransportType)
	}
}

func createManager(ctx context.Context, restConfig *rest.Config, agentConfig *config.AgentConfig,
	log logr.Logger,
) (ctrl.Manager, error) {
	leaseDuration := time.Duration(agentConfig.ElectionConfig.LeaseDuration) * time.Second
	renewDeadline := time.Duration(agentConfig.ElectionConfig.RenewDeadline) * time.Second
	retryPeriod := time.Duration(agentConfig.ElectionConfig.RetryPeriod) * time.Second

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
		LeaderElectionNamespace: agentConfig.PodNameSpace,
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
	incarnation, err := incarnation.GetIncarnation(mgr)
	if err != nil {
		return nil, fmt.Errorf("failed to get incarnation version: %w", err)
	}
	log.Info("start agent with incarnation version", "version", incarnation)

	// add spec controllers
	if err := specController.AddToManager(mgr, agentConfig); err != nil {
		return nil, fmt.Errorf("failed to add spec syncer: %w", err)
	}
	log.Info("add spec controllers to manager")

	// producer, err := getProducer(agentConfig)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to initialize transport producer: %w", err)
	// }

	// if err := mgr.Add(producer); err != nil {
	// 	return nil, fmt.Errorf("failed to add transport producer: %w", err)
	// }

	if err := statusController.AddControllers(mgr, agentConfig, incarnation); err != nil {
		return nil, fmt.Errorf("failed to add status syncer: %w", err)
	}

	if err := controllers.AddToManager(mgr); err != nil {
		return nil, fmt.Errorf("failed to add controllers: %w", err)
	}

	if err := lease.AddHoHLeaseUpdater(mgr, agentConfig.PodNameSpace,
		"multicluster-global-hub-controller"); err != nil {
		return nil, fmt.Errorf("failed to add lease updater: %w", err)
	}

	return mgr, nil
}
