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

	"github.com/stolostron/multicluster-global-hub/agent/pkg/controllers"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/incarnation"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/lease"
	agentscheme "github.com/stolostron/multicluster-global-hub/agent/pkg/scheme"
	specController "github.com/stolostron/multicluster-global-hub/agent/pkg/spec/controller"
	statusController "github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller"
	"github.com/stolostron/multicluster-global-hub/pkg/compressor"
	"github.com/stolostron/multicluster-global-hub/pkg/jobs"
	commonobjects "github.com/stolostron/multicluster-global-hub/pkg/objects"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/consumer"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/producer"
)

const (
	metricsHost                = "0.0.0.0"
	metricsPort          int32 = 8384
	kafkaTransportType         = "kafka"
	leaderElectionLockID       = "multicluster-global-hub-agent-lock"
)

type regionalHubConfig struct {
	LeafHubName                  string
	PodNameSpace                 string
	TransportType                string
	TransportCompressionType     string
	SpecWorkPoolSize             int
	SpecEnforceHohRbac           bool
	StatusDeltaCountSwitchFactor int
	BootstrapServers             string
	KafkaCAPath                  string
	ProducerConfig               *producer.KafkaProducerConfig
	ConsumerConfig               *consumer.KafkaConsumerConfig
	ElectionConfig               *commonobjects.LeaderElectionConfig
	Terminating                  bool
}

var defaultRegionalHubConfig = &regionalHubConfig{
	ConsumerConfig: &consumer.KafkaConsumerConfig{},
	ProducerConfig: &producer.KafkaProducerConfig{},
	ElectionConfig: &commonobjects.LeaderElectionConfig{},
}

func main() {
	// adding and parsing flags should be done before the call of 'ctrl.GetConfigOrDie()',
	// otherwise kubeconfig will not be passed to agent main process
	addAndParseFlags()
	os.Exit(doMain(ctrl.SetupSignalHandler(), ctrl.GetConfigOrDie()))
}

// function to handle defers with exit, see https://stackoverflow.com/a/27629493/553720.
func doMain(ctx context.Context, restConfig *rest.Config) int {
	log := initLog()
	printVersion(log)

	// create regional hub configuration from command flags
	parsedRegionalHubConfig, err := getRegionalHubConfigFromFlags()
	if err != nil {
		log.Error(err, "failed to get regional hub configuration from command line flags")
		return 1
	}

	if parsedRegionalHubConfig.Terminating {
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

	mgr, err := createManager(restConfig, parsedRegionalHubConfig)
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

func addAndParseFlags() {
	// add flags for logger
	pflag.CommandLine.AddFlagSet(zap.FlagSet())
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	pflag.StringVar(&defaultRegionalHubConfig.LeafHubName, "leaf-hub-name", "", "The name of the leaf hub.")
	pflag.StringVar(&defaultRegionalHubConfig.BootstrapServers, "kafka-bootstrap-server", "",
		"The bootstrap server for kafka.")
	pflag.StringVar(&defaultRegionalHubConfig.KafkaCAPath, "kafka-ca-path", "",
		"The certificate path of CA certificate for kafka bootstrap server.")
	pflag.StringVar(&defaultRegionalHubConfig.ProducerConfig.ProducerID, "kafka-producer-id", "",
		"Producer Id for the kafka, default is the leaf hub name.")
	pflag.StringVar(&defaultRegionalHubConfig.ProducerConfig.ProducerTopic, "kafka-producer-topic",
		"status", "Topic for the kafka producer.")
	pflag.IntVar(&defaultRegionalHubConfig.ProducerConfig.MsgSizeLimitKB, "kafka-message-size-limit", 100,
		"The limit for kafka message size in KB.")
	pflag.StringVar(&defaultRegionalHubConfig.ConsumerConfig.ConsumerTopic, "kafka-consumer-topic",
		"spec", "Topic for the kafka consumer.")
	pflag.StringVar(&defaultRegionalHubConfig.ConsumerConfig.ConsumerID, "kakfa-consumer-id",
		"multicluster-global-hub-agent", "ID for the kafka consumer.")
	pflag.StringVar(&defaultRegionalHubConfig.PodNameSpace, "pod-namespace", "open-cluster-management",
		"The agent running namespace, also used as leader election namespace")
	pflag.StringVar(&defaultRegionalHubConfig.TransportType, "transport-type", "kafka",
		"The transport type, 'kafka'")
	pflag.IntVar(&defaultRegionalHubConfig.SpecWorkPoolSize, "consumer-worker-pool-size", 10,
		"The goroutine number to propagate the bundles on managed cluster.")
	pflag.BoolVar(&defaultRegionalHubConfig.SpecEnforceHohRbac, "enforce-hoh-rbac", false,
		"enable hoh RBAC or not, default false")
	pflag.StringVar(&defaultRegionalHubConfig.TransportCompressionType,
		"transport-message-compression-type", "gzip",
		"The message compression type for transport layer, 'gzip' or 'no-op'.")
	pflag.IntVar(&defaultRegionalHubConfig.StatusDeltaCountSwitchFactor,
		"status-delta-count-switch-factor", 100,
		"default with 100.")
	pflag.IntVar(&defaultRegionalHubConfig.ElectionConfig.LeaseDuration, "lease-duration", 137,
		"leader election lease duration")
	pflag.IntVar(&defaultRegionalHubConfig.ElectionConfig.RenewDeadline, "renew-deadline", 107,
		"leader election renew deadline")
	pflag.IntVar(&defaultRegionalHubConfig.ElectionConfig.RetryPeriod, "retry-period", 26,
		"leader election retry period")
	pflag.BoolVar(&defaultRegionalHubConfig.Terminating, "terminating", false,
		"true is to trigger the PreStop hook to do cleanup. For example: removing finalizer")

	pflag.Parse()
}

func getRegionalHubConfigFromFlags() (*regionalHubConfig, error) {
	if defaultRegionalHubConfig.LeafHubName == "" {
		return nil, fmt.Errorf("flag regional-hub-name can't be empty")
	}
	if defaultRegionalHubConfig.ProducerConfig.ProducerID == "" {
		defaultRegionalHubConfig.ProducerConfig.ProducerID = defaultRegionalHubConfig.LeafHubName
	}
	if defaultRegionalHubConfig.SpecWorkPoolSize < 1 ||
		defaultRegionalHubConfig.SpecWorkPoolSize > 100 {
		return nil, fmt.Errorf("flag consumer-worker-pool-size should be in the scope [1, 100]")
	}

	if defaultRegionalHubConfig.ProducerConfig.MsgSizeLimitKB > producer.MaxMessageSizeLimit {
		return nil, fmt.Errorf("flag kafka-message-size-limit %d must not exceed %d",
			defaultRegionalHubConfig.ProducerConfig.MsgSizeLimitKB, producer.MaxMessageSizeLimit)
	}

	return defaultRegionalHubConfig, nil
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
func getConsumer(parsedRegionalHubConfig *regionalHubConfig) (consumer.Consumer, error) {
	switch parsedRegionalHubConfig.TransportType {
	case kafkaTransportType:
		kafkaConsumer, err := consumer.NewKafkaConsumer(
			parsedRegionalHubConfig.BootstrapServers, parsedRegionalHubConfig.KafkaCAPath,
			parsedRegionalHubConfig.ConsumerConfig, ctrl.Log.WithName("kafka-consumer"))
		if err != nil {
			return nil, fmt.Errorf("failed to create kafka-consumer: %w", err)
		}
		kafkaConsumer.SetLeafHubName(parsedRegionalHubConfig.LeafHubName)
		return kafkaConsumer, nil
	default:
		return nil, fmt.Errorf("command flag transport-type - %q is not a valid option",
			parsedRegionalHubConfig.TransportType)
	}
}

func getProducer(parsedRegionalHubConfig *regionalHubConfig) (producer.Producer, error) {
	messageCompressor, err := compressor.NewCompressor(
		compressor.CompressionType(parsedRegionalHubConfig.TransportCompressionType))
	if err != nil {
		return nil, fmt.Errorf("failed to create transport producer message-compressor: %w", err)
	}

	switch parsedRegionalHubConfig.TransportType {
	case kafkaTransportType:
		kafkaProducer, err := producer.NewKafkaProducer(messageCompressor,
			parsedRegionalHubConfig.BootstrapServers, parsedRegionalHubConfig.KafkaCAPath,
			parsedRegionalHubConfig.ProducerConfig, ctrl.Log.WithName("kafka-producer"))
		if err != nil {
			return nil, fmt.Errorf("failed to create kafka-producer: %w", err)
		}
		return kafkaProducer, nil
	default:
		return nil, fmt.Errorf("flag transport-type - %q is not a valid option",
			parsedRegionalHubConfig.TransportType)
	}
}

func createManager(restConfig *rest.Config, parsedRegionalHubConfig *regionalHubConfig) (ctrl.Manager, error) {
	leaseDuration := time.Duration(parsedRegionalHubConfig.ElectionConfig.LeaseDuration) * time.Second
	renewDeadline := time.Duration(parsedRegionalHubConfig.ElectionConfig.RenewDeadline) * time.Second
	retryPeriod := time.Duration(parsedRegionalHubConfig.ElectionConfig.RetryPeriod) * time.Second

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
		LeaderElectionNamespace: parsedRegionalHubConfig.PodNameSpace,
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
	fmt.Printf("Starting the Cmd incarnation: %d", incarnation)

	consumer, err := getConsumer(parsedRegionalHubConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize transport consumer: %w", err)
	}

	producer, err := getProducer(parsedRegionalHubConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize transport producer: %w", err)
	}

	if err := mgr.Add(consumer); err != nil {
		return nil, fmt.Errorf("failed to add transport consumer: %w", err)
	}

	if err := mgr.Add(producer); err != nil {
		return nil, fmt.Errorf("failed to add transport producer: %w", err)
	}

	if err := specController.AddSyncersToManager(mgr, consumer,
		parsedRegionalHubConfig.SpecWorkPoolSize,
		parsedRegionalHubConfig.SpecEnforceHohRbac); err != nil {
		return nil, fmt.Errorf("failed to add spec syncer: %w", err)
	}

	if err := statusController.AddControllers(mgr, producer,
		parsedRegionalHubConfig.LeafHubName,
		parsedRegionalHubConfig.StatusDeltaCountSwitchFactor, incarnation); err != nil {
		return nil, fmt.Errorf("failed to add status syncer: %w", err)
	}

	if err := controllers.AddToManager(mgr); err != nil {
		return nil, fmt.Errorf("failed to add controllers: %w", err)
	}

	if err := lease.AddHoHLeaseUpdater(mgr, parsedRegionalHubConfig.PodNameSpace,
		"multicluster-global-hub-controller"); err != nil {
		return nil, fmt.Errorf("failed to add lease updater: %w", err)
	}

	return mgr, nil
}
