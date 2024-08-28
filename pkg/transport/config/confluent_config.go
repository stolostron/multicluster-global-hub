package config

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	kafkav2 "github.com/confluentinc/confluent-kafka-go/v2/kafka"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/kyaml/yaml"

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
	_ = kafkaConfigMap.SetKey("retries", "1")
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
		return errors.New("invalid ca certificate for tls")
	}
	_, validCert := utils.Validate(certPath)
	_, validKey := utils.Validate(keyPath)
	if !validCert || !validKey {
		return errors.New("invalid client key or cert")
	}

	if err := kafkaConfigMap.SetKey("security.protocol", "ssl"); err != nil {
		return err
	}
	if err := kafkaConfigMap.SetKey("ssl.ca.location", caCertPath); err != nil {
		return err
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
		return errors.New("failed to append ca certificate")
	}

	// set client certificate
	_ = kafkaConfigMap.SetKey("ssl.certificate.location", certPath)
	_ = kafkaConfigMap.SetKey("ssl.key.location", keyPath)
	return nil
}

// https://github.com/confluentinc/librdkafka/blob/master/CONFIGURATION.md
func GetConfluentConfigMap(kafkaConfig *transport.KafkaConfig, producer bool) (*kafkav2.ConfigMap, error) {
	kafkaConfigMap := GetBasicConfigMap()
	_ = kafkaConfigMap.SetKey("bootstrap.servers", kafkaConfig.BootstrapServer)
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

// GetConfluentConfigMapByConfig tries to connect the kafka with transport secret(ca.key, client.crt, client.key)
func GetConfluentConfigMapByConfig(transportConfig *corev1.Secret, c client.Client, consumerGroupID string) (
	*kafkav2.ConfigMap, error,
) {
	conn, err := GetTransportCredentailBySecret(transportConfig, c)
	if err != nil {
		return nil, err
	}
	return GetConfluentConfigMapByKafkaCredential(conn, consumerGroupID)
}

func GetConfluentConfigMapByKafkaCredential(conn *transport.KafkaConnCredential, consumerGroupID string) (
	*kafkav2.ConfigMap, error,
) {
	kafkaConfigMap := GetBasicConfigMap()
	if consumerGroupID != "" {
		SetConsumerConfig(kafkaConfigMap, consumerGroupID)
	} else {
		SetProducerConfig(kafkaConfigMap)
	}
	_ = kafkaConfigMap.SetKey("bootstrap.servers", conn.BootstrapServer)
	// if the certs is invalid
	if conn.CACert == "" || conn.ClientCert == "" || conn.ClientKey == "" {
		klog.Warning("Connect to Kafka without SSL")
		return kafkaConfigMap, nil
	}

	_ = kafkaConfigMap.SetKey("security.protocol", "ssl")
	if err := kafkaConfigMap.SetKey("ssl.ca.pem", conn.CACert); err != nil {
		return nil, err
	}

	if err := kafkaConfigMap.SetKey("ssl.certificate.pem", conn.ClientCert); err != nil {
		return nil, err
	}

	if err := kafkaConfigMap.SetKey("ssl.key.pem", conn.ClientKey); err != nil {
		return nil, err
	}
	return kafkaConfigMap, nil
}

func GetTransportCredentailBySecret(transportConfig *corev1.Secret, c client.Client) (
	*transport.KafkaConnCredential, error,
) {
	kafkaConfig, ok := transportConfig.Data["kafka.yaml"]
	if !ok {
		return nil, fmt.Errorf("must set the `kafka.yaml` in the transport secret(%s)", transportConfig.Name)
	}
	conn := &transport.KafkaConnCredential{}
	if err := yaml.Unmarshal(kafkaConfig, conn); err != nil {
		return nil, fmt.Errorf("failed to unmarshal kafka config to transport credentail: %w", err)
	}

	// decode the ca and client cert
	if conn.CACert != "" {
		bytes, err := base64.StdEncoding.DecodeString(conn.CACert)
		if err != nil {
			return nil, err
		}
		conn.CACert = string(bytes)
	}
	if conn.ClientCert != "" {
		bytes, err := base64.StdEncoding.DecodeString(conn.ClientCert)
		if err != nil {
			return nil, err
		}
		conn.ClientCert = string(bytes)
	}
	if conn.ClientKey != "" {
		bytes, err := base64.StdEncoding.DecodeString(conn.ClientKey)
		if err != nil {
			return nil, err
		}
		conn.ClientKey = string(bytes)
	}

	if conn.CASecretName != "" {
		caSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: transportConfig.Namespace,
				Name:      conn.CASecretName,
			},
		}
		if err := c.Get(context.Background(), client.ObjectKeyFromObject(caSecret), caSecret); err != nil {
			return nil, err
		}
		conn.CACert = string(caSecret.Data["ca.crt"])
	}
	if conn.ClientSecretName != "" {
		clientSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: transportConfig.Namespace,
				Name:      conn.ClientSecretName,
			},
		}
		if err := c.Get(context.Background(), client.ObjectKeyFromObject(clientSecret), clientSecret); err != nil {
			return nil, fmt.Errorf("failed to get the client cert: %w", err)
		}
		conn.ClientCert = string(clientSecret.Data["tls.crt"])
		conn.ClientKey = string(clientSecret.Data["tls.key"])
	}
	return conn, nil
}
