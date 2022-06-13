package helper

import (
	"flag"
	"fmt"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/operator-framework/operator-sdk/pkg/log/zap"
	"github.com/spf13/pflag"
	kafkaclient "github.com/stolostron/hub-of-hubs-kafka-transport/kafka-client"
)

const (
	defaultK8sClientsPoolSize = 10
	maxMessageSizeLimit       = 1024 // to make sure that the message size is below 1 MB.
)

type KafkaConfig struct {
	BootstrapServers     string
	SslCa                string
	ComsumerTopic        string
	ProducerId           string
	ProducerTopic        string
	ProducerMessageLimit int
}

type SyncServiceConfig struct {
	Protocol                string
	ConsumerHost            string
	ConsumerPort            int
	ConsumerPollingInterval int
	ProducerHost            string
	ProducerPort            int
}

type ConfigManager struct {
	LeafHubName                  string
	PodNameSpace                 string
	TransportType                string
	TransportCompressionType     string
	SpecWorkPoolSize             int
	SpecEnforceHohRbac           bool
	StatusDeltaCountSwitchFactor int
	Kafka                        *KafkaConfig
	SyncService                  *SyncServiceConfig
}

func NewConfigManager() (*ConfigManager, error) {
	// add flags for logger
	pflag.CommandLine.AddFlagSet(zap.FlagSet())
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	configManager := &ConfigManager{
		Kafka:       &KafkaConfig{},
		SyncService: &SyncServiceConfig{},
	}

	pflag.StringVar(&configManager.LeafHubName, "leaf-hub-name", "", "The name of the leaf hub.")
	pflag.StringVar(&configManager.Kafka.BootstrapServers, "kafka-bootstrap-server", "", "The bootstrap server for kafka.")
	pflag.StringVar(&configManager.Kafka.SslCa, "kafka-ssl-ca", "", "The authentication to connect to the kafka.")

	pflag.StringVar(&configManager.Kafka.ProducerId, "kafka-producer-id", "", "Producer Id for the kafka, default is the leaf hub name.")

	pflag.StringVar(&configManager.Kafka.ComsumerTopic, "kafka-consumer-topic", "spec", "Topic for the kafka consumer.")
	pflag.StringVar(&configManager.Kafka.ProducerTopic, "kafka-producer-topic", "status", "Topic for the kafka producer.")
	pflag.StringVar(&configManager.PodNameSpace, "pod-namespace", "open-cluster-management", "The agent running namespace, also used as leader election namespace")
	pflag.StringVar(&configManager.TransportType, "transport-type", "kafka", "The transport type, 'kafka' or 'sync-service'.")
	pflag.StringVar(&configManager.SyncService.Protocol, "sync-service-protocol", "http", "The protocol for sync-service communication.")
	pflag.StringVar(&configManager.SyncService.ConsumerHost, "cloud-sync-service-consumer-host", "sync-service-ess.sync-service.svc.cluster.local", "The host for Cloud Sync Service.")
	pflag.StringVar(&configManager.SyncService.ProducerHost, "cloud-sync-service-producer-host", "sync-service-ess.sync-service.svc.cluster.local", "The host for Cloud Sync Service.")
	pflag.IntVar(&configManager.SyncService.ConsumerPort, "cloud-sync-service-consumer-port", 8090, "The port for Cloud Sync Service.")
	pflag.IntVar(&configManager.SyncService.ProducerPort, "cloud-sync-service-producer-port", 8090, "The port for Cloud Sync Service.")
	pflag.IntVar(&configManager.SyncService.ConsumerPollingInterval, "cloud-sync-service-polling-interval", 5, "The polling interval in second for Cloud Sync Service.")
	pflag.IntVar(&configManager.SpecWorkPoolSize, "consumer-worker-pool-size", defaultK8sClientsPoolSize, "The goroutine number to propagate the bundles on managed cluster.")
	pflag.BoolVar(&configManager.SpecEnforceHohRbac, "enforce-hoh-rbac", false, "enable hoh RBAC or not, default false")
	pflag.StringVar(&configManager.TransportCompressionType, "transport-message-compression-type", "gzip", "The message compression type for transport layer, 'gzip' or 'no-op'.")
	pflag.IntVar(&configManager.Kafka.ProducerMessageLimit, "kafka-message-size-limit", 100, "The limit for kafka message size in KB.")
	pflag.IntVar(&configManager.StatusDeltaCountSwitchFactor, "status-delta-count-switch-factor", 100, "default with 100.")
	pflag.Parse()

	if configManager.LeafHubName == "" {
		return nil, fmt.Errorf("flag leaf-hub-name can't be empty")
	}
	if configManager.Kafka.ProducerId == "" {
		configManager.Kafka.ProducerId = configManager.LeafHubName
	}
	if configManager.SpecWorkPoolSize < 1 || configManager.SpecWorkPoolSize > 100 {
		return nil, fmt.Errorf("flag consumer-worker-pool-size should be in the scope [1, 100]")
	}

	if configManager.Kafka.ProducerMessageLimit > maxMessageSizeLimit {
		return nil, fmt.Errorf("flag kafka-message-size-limit %d must not exceed %d", configManager.Kafka.ProducerMessageLimit, maxMessageSizeLimit)
	}
	return configManager, nil
}

func (configManager *ConfigManager) GetKafkaConfigMap() (*kafka.ConfigMap, error) {
	kafkaConfigMap := &kafka.ConfigMap{
		"bootstrap.servers":       configManager.Kafka.BootstrapServers,
		"client.id":               configManager.LeafHubName,
		"group.id":                configManager.LeafHubName,
		"auto.offset.reset":       "earliest",
		"enable.auto.commit":      "false",
		"socket.keepalive.enable": "true",
		"log.connection.close":    "false", // silence spontaneous disconnection logs, kafka recovers by itself.
	}
	err := configManager.loadSslToConfigMap(kafkaConfigMap)
	if err != nil {
		return kafkaConfigMap, fmt.Errorf("failed to configure kafka-consumer - %w", err)
	}
	return kafkaConfigMap, nil
}

func (configManager *ConfigManager) GetProducerKafkaConfigMap() (*kafka.ConfigMap, error) {
	kafkaConfigMap := &kafka.ConfigMap{
		"bootstrap.servers":       configManager.Kafka.BootstrapServers,
		"client.id":               configManager.Kafka.ProducerId,
		"acks":                    "1",
		"retries":                 "0",
		"socket.keepalive.enable": "true",
		"log.connection.close":    "false", // silence spontaneous disconnection logs, kafka recovers by itself.
	}

	err := configManager.loadSslToConfigMap(kafkaConfigMap)
	if err != nil {
		return kafkaConfigMap, fmt.Errorf("failed to configure kafka-producer - %w", err)
	}
	return kafkaConfigMap, nil
}

func (configManger *ConfigManager) loadSslToConfigMap(kafkaConfigMap *kafka.ConfigMap) error {
	// sslBase64EncodedCertificate
	if configManger.Kafka.SslCa != "" {
		certFileLocation, err := kafkaclient.SetCertificate(&configManger.Kafka.SslCa)
		if err != nil {
			return fmt.Errorf("failed to SetCertificate - %w", err)
		}

		if err = kafkaConfigMap.SetKey("security.protocol", "ssl"); err != nil {
			return fmt.Errorf("failed to SetKey security.protocol - %w", err)
		}

		if err = kafkaConfigMap.SetKey("ssl.ca.location", certFileLocation); err != nil {
			return fmt.Errorf("failed to SetKey ssl.ca.location - %w", err)
		}
	}
	return nil
}
