package helper

import (
	"flag"
	"fmt"

	"github.com/operator-framework/operator-sdk/pkg/log/zap"
	"github.com/spf13/pflag"

	commonobjects "github.com/stolostron/multicluster-global-hub/pkg/objects"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/consumer"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/producer"
)

const (
	defaultK8sClientsPoolSize = 10
	maxMessageSizeLimit       = 1024 // to make sure that the message size is below 1 MB.
)

type ConfigManager struct {
	LeafHubName                  string
	PodNameSpace                 string
	TransportType                string
	TransportCompressionType     string
	SpecWorkPoolSize             int
	SpecEnforceHohRbac           bool
	StatusDeltaCountSwitchFactor int
	BootstrapServers             string
	SslCA                        string
	ProducerConfig               *producer.KafkaProducerConfig
	ConsumerConfig               *consumer.KafkaConsumerConfig
	ElectionConfig               *commonobjects.LeaderElectionConfig
	Terminating                  bool
}

func NewConfigManager() (*ConfigManager, error) {
	// add flags for logger
	pflag.CommandLine.AddFlagSet(zap.FlagSet())
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	configManager := &ConfigManager{
		ConsumerConfig: &consumer.KafkaConsumerConfig{},
		ProducerConfig: &producer.KafkaProducerConfig{},
		ElectionConfig: &commonobjects.LeaderElectionConfig{},
	}

	pflag.StringVar(&configManager.LeafHubName, "leaf-hub-name", "", "The name of the leaf hub.")
	pflag.StringVar(&configManager.BootstrapServers, "kafka-bootstrap-server", "",
		"The bootstrap server for kafka.")
	pflag.StringVar(&configManager.SslCA, "kafka-ssl-ca", "",
		"The authentication to connect to the kafka.")

	pflag.StringVar(&configManager.ProducerConfig.ProducerID, "kafka-producer-id", "",
		"Producer Id for the kafka, default is the leaf hub name.")

	pflag.StringVar(&configManager.ConsumerConfig.ConsumerTopic, "kafka-consumer-topic",
		"spec", "Topic for the kafka consumer.")
	pflag.StringVar(&configManager.ProducerConfig.ProducerTopic, "kafka-producer-topic",
		"status", "Topic for the kafka producer.")
	pflag.StringVar(&configManager.PodNameSpace, "pod-namespace", "open-cluster-management",
		"The agent running namespace, also used as leader election namespace")
	pflag.StringVar(&configManager.TransportType, "transport-type", "kafka",
		"The transport type, 'kafka'")
	pflag.IntVar(&configManager.SpecWorkPoolSize, "consumer-worker-pool-size", defaultK8sClientsPoolSize,
		"The goroutine number to propagate the bundles on managed cluster.")
	pflag.BoolVar(&configManager.SpecEnforceHohRbac, "enforce-hoh-rbac", false,
		"enable hoh RBAC or not, default false")
	pflag.StringVar(&configManager.TransportCompressionType,
		"transport-message-compression-type", "gzip",
		"The message compression type for transport layer, 'gzip' or 'no-op'.")
	pflag.IntVar(&configManager.ProducerConfig.MsgSizeLimitKB, "kafka-message-size-limit", 100,
		"The limit for kafka message size in KB.")
	pflag.IntVar(&configManager.StatusDeltaCountSwitchFactor,
		"status-delta-count-switch-factor", 100,
		"default with 100.")
	pflag.IntVar(&configManager.ElectionConfig.LeaseDuration, "lease-duration", 137, "leader election lease duration")
	pflag.IntVar(&configManager.ElectionConfig.RenewDeadline, "renew-deadline", 107, "leader election renew deadline")
	pflag.IntVar(&configManager.ElectionConfig.RetryPeriod, "retry-period", 26, "leader election retry period")
	pflag.BoolVar(&configManager.Terminating, "terminating", false,
		"true is to trigger the PreStop hook to do cleanup. For example: removing finalizer")
	pflag.Parse()

	if configManager.LeafHubName == "" {
		return nil, fmt.Errorf("flag leaf-hub-name can't be empty")
	}
	if configManager.ProducerConfig.ProducerID == "" {
		configManager.ProducerConfig.ProducerID = configManager.LeafHubName
	}
	if configManager.SpecWorkPoolSize < 1 || configManager.SpecWorkPoolSize > 100 {
		return nil, fmt.Errorf("flag consumer-worker-pool-size should be in the scope [1, 100]")
	}

	if configManager.ProducerConfig.MsgSizeLimitKB > maxMessageSizeLimit {
		return nil, fmt.Errorf("flag kafka-message-size-limit %d must not exceed %d",
			configManager.ProducerConfig.MsgSizeLimitKB, maxMessageSizeLimit)
	}
	return configManager, nil
}
