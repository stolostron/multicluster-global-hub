// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package protocol

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"

	"github.com/Shopify/sarama"
	"github.com/cloudevents/sdk-go/protocol/kafka_sarama/v2"
)

const MaxMessageKBLimit = 1024

type KafkaConfig struct {
	BootstrapServer string
	CertPath        string
	EnableTSL       bool
	ProducerConfig  *KafkaProducerConfig
	ConsumerConfig  *KafkaConsumerConfig
}

type KafkaProducerConfig struct {
	ProducerID         string
	ProducerTopic      string
	MessageSizeLimitKB int
}

type KafkaConsumerConfig struct {
	ConsumerID    string
	ConsumerTopic string
}

func NewKafkaSender(config *KafkaConfig) (*kafka_sarama.Sender, error) {
	saramaConfig, err := getSaramaConfig(config)
	if err != nil {
		return nil, err
	}

	// set max message bytes to 1 MB: 1000 000 > config.ProducerConfig.MessageSizeLimitKB * 1000
	saramaConfig.Producer.MaxMessageBytes = MaxMessageKBLimit * 1000
	sender, err := kafka_sarama.NewSender([]string{config.BootstrapServer}, saramaConfig, config.ProducerConfig.ProducerTopic)
	if err != nil {
		return nil, err
	}
	return sender, nil
}

func NewKafkaReceiver(config *KafkaConfig) (*kafka_sarama.Consumer, error) {
	saramaConfig, err := getSaramaConfig(config)
	if err != nil {
		return nil, err
	}
	saramaConfig.Consumer.Offsets.AutoCommit.Enable = false
	saramaConfig.Consumer.Offsets.Initial = sarama.OffsetOldest
	// set the consumer groupId = clientId
	receiver, err := kafka_sarama.NewConsumer([]string{config.BootstrapServer}, saramaConfig, config.ConsumerConfig.ConsumerID, config.ConsumerConfig.ConsumerTopic)
	if err != nil {
		return nil, err
	}
	return receiver, nil
}

func getSaramaConfig(kafkaConfig *KafkaConfig) (*sarama.Config, error) {
	saramaConfig := sarama.NewConfig()
	saramaConfig.Version = sarama.V2_0_0_0

	if kafkaConfig.EnableTSL {
		cert, err := ioutil.ReadFile(kafkaConfig.CertPath) // #nosec G304
		if err != nil {
			return nil, err
		}
		certPool := x509.NewCertPool()
		certPool.AppendCertsFromPEM(cert)
		// #nosec
		tlsConfig := &tls.Config{
			RootCAs:            certPool,
			InsecureSkipVerify: true,
		}
		saramaConfig.Net.TLS.Enable = true
		saramaConfig.Net.TLS.Config = tlsConfig
	}
	return saramaConfig, nil
}
