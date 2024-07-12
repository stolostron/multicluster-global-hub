package config

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	kafkav2 "github.com/confluentinc/confluent-kafka-go/v2/kafka"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func GetBasicConfigMap() *kafkav2.ConfigMap {
	return &kafkav2.ConfigMap{
		"socket.keepalive.enable": "true",
		// silence spontaneous disconnection logs, kafka recovers by itself.
		"log.connection.close": "false",
		// https://github.com/confluentinc/librdkafka/issues/4349
		"ssl.endpoint.identification.algorithm": "none",
	}
}

func SetProducerConfig(kafkaConfigMap *kafkav2.ConfigMap) {
	_ = kafkaConfigMap.SetKey("go.produce.channel.size", 1000)
	_ = kafkaConfigMap.SetKey("acks", "1")
	_ = kafkaConfigMap.SetKey("retries", "0")
	_ = kafkaConfigMap.SetKey("go.events.channel.size", 1000)
}

func SetConsumerConfig(kafkaConfigMap *kafkav2.ConfigMap, groupId string) {
	_ = kafkaConfigMap.SetKey("enable.auto.commit", "true")
	_ = kafkaConfigMap.SetKey("auto.offset.reset", "earliest")
	_ = kafkaConfigMap.SetKey("group.id", groupId)
	_ = kafkaConfigMap.SetKey("go.events.channel.size", 1000)
}

func SetTLSByLocation(kafkaConfigMap *kafkav2.ConfigMap, caCertPath, certPath, keyPath string) error {
	_, validCA := utils.Validate(caCertPath)
	if !validCA {
		return errors.New("invalid ca certificate")
	}
	_, validCert := utils.Validate(certPath)
	_, validKey := utils.Validate(keyPath)
	if !validCert || !validKey {
		return errors.New("invalid client key or cert")
	}

	// set ca
	certBytes, err := os.ReadFile(filepath.Clean(caCertPath))
	if err != nil {
		return err
	}
	// Get the SystemCertPool, continue with an empty pool on error
	rootCertAuth, _ := x509.SystemCertPool()
	if rootCertAuth == nil {
		rootCertAuth = x509.NewCertPool()
	}

	// Append our cert to the system pool
	if ok := rootCertAuth.AppendCertsFromPEM(certBytes); !ok {
		return errors.New("kafka-certificate-manager: failed to append certificate")
	}

	if err := kafkaConfigMap.SetKey("security.protocol", "ssl"); err != nil {
		return err
	}
	if err := kafkaConfigMap.SetKey("ssl.ca.location", caCertPath); err != nil {
		return err
	}

	// set client certificate
	_ = kafkaConfigMap.SetKey("ssl.certificate.location", certPath)
	_ = kafkaConfigMap.SetKey("ssl.key.location", keyPath)
	return nil
}

// https://github.com/confluentinc/librdkafka/blob/master/CONFIGURATION.md
func GetConfluentConfigMap(kafkaConfig *transport.KafkaConfig, producer bool) (*kafkav2.ConfigMap, error) {
	kafkaConfigMap := GetBasicConfigMap()
	kafkaConfigMap.SetKey("bootstrap.servers", kafkaConfig.BootstrapServer)
	if producer {
		SetProducerConfig(kafkaConfigMap)
	} else {
		SetConsumerConfig(kafkaConfigMap, kafkaConfig.ConsumerConfig.ConsumerID)
	}
	if !kafkaConfig.EnableTLS {
		return kafkaConfigMap, nil
	}
	err := SetTLSByLocation(kafkaConfigMap, kafkaConfig.CaCertPath, kafkaConfig.ClientCertPath, kafkaConfig.ClientKeyPath)
	if err != nil {
		return nil, err
	}
	return kafkaConfigMap, nil
}

// GetConfluentConfigMapByUser create a kafka.configmap by the kafkauser
func GetConfluentConfigMapByUser(c client.Client, namespace, clusterName, userName string) (*kafkav2.ConfigMap, error) {
	kafkaCluster := &kafkav1beta2.Kafka{}
	err := c.Get(context.TODO(), types.NamespacedName{
		Name:      clusterName,
		Namespace: namespace,
	}, kafkaCluster)
	if err != nil {
		return nil, err
	}

	if kafkaCluster.Status == nil || kafkaCluster.Status.Conditions == nil {
		return nil, fmt.Errorf("kafka cluster %s has no status conditions", kafkaCluster.Name)
	}

	kafkaClientCertSecret := &corev1.Secret{}
	err = c.Get(context.TODO(), types.NamespacedName{
		Name:      userName,
		Namespace: namespace,
	}, kafkaClientCertSecret)
	if err != nil {
		return nil, err
	}
	clientCert := string(kafkaClientCertSecret.Data["user.crt"])
	clientKey := string(kafkaClientCertSecret.Data["user.key"])

	cm := GetBasicConfigMap()
	for _, condition := range kafkaCluster.Status.Conditions {
		if *condition.Type == "Ready" && *condition.Status == "True" {
			clusterCaCert := kafkaCluster.Status.Listeners[1].Certificates[0]
			_ = cm.SetKey("bootstrap.servers", *kafkaCluster.Status.Listeners[1].BootstrapServers)
			_ = cm.SetKey("security.protocol", "ssl")
			_ = cm.SetKey("ssl.ca.pem", clusterCaCert)
			_ = cm.SetKey("ssl.certificate.pem", clientCert)
			_ = cm.SetKey("ssl.key.pem", clientKey)
			return cm, nil
		}
	}
	return nil, fmt.Errorf("kafka cluster %s/%s is not ready", namespace, clusterName)
}
