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

const (
	kiloBytesToBytes = 1000
)

type KafkaConfig struct {
	BootstrapServer string
	CertPath        string
	EnableTSL       bool
	ProducerConfig  *KafkaProducerConfig
	ConsumerConfig  *KafkaConsumerConfig
}

type KafkaProducerConfig struct {
	ProducerID     string
	ProducerTopic  string
	MsgSizeLimitKB int
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

	saramaConfig.Producer.MaxMessageBytes = config.ProducerConfig.MsgSizeLimitKB * kiloBytesToBytes
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
	// set the consumer groupId = clientId
	receiver, err := kafka_sarama.NewConsumer([]string{config.BootstrapServer}, saramaConfig, config.ConsumerConfig.ConsumerID, config.ConsumerConfig.ConsumerID)
	if err != nil {
		return nil, err
	}
	return receiver, nil
}

func getSaramaConfig(kafkaConfig *KafkaConfig) (*sarama.Config, error) {
	saramaConfig := sarama.NewConfig()
	saramaConfig.Version = sarama.V2_0_0_0

	if kafkaConfig.EnableTSL {
		saramaConfig.Net.TLS.Enable = true

		cert, err := ioutil.ReadFile(kafkaConfig.CertPath)
		if err != nil {
			return nil, err
		}
		certPool := x509.NewCertPool()
		certPool.AppendCertsFromPEM(cert)

		tlsConfig := &tls.Config{
			RootCAs:            certPool,
			InsecureSkipVerify: true,
		}
		saramaConfig.Net.TLS.Config = tlsConfig
	}
	return saramaConfig, nil
}
