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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/kyaml/yaml"

	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var log = logger.DefaultZapLogger()

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
func GetConfluentConfigMap(kafkaConfig *transport.KafkaInternalConfig, producer bool) (*kafkav2.ConfigMap, error) {
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
	conn, err := GetKafkaCredentailBySecret(transportConfig, c)
	if err != nil {
		return nil, err
	}
	return GetConfluentConfigMapByKafkaCredential(conn, consumerGroupID)
}

func GetConfluentConfigMapByKafkaCredential(conn *transport.KafkaConfig, consumerGroupID string) (
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
		log.Warn("Connect to Kafka without SSL")
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

func GetKafkaCredentailBySecret(transportSecret *corev1.Secret, c client.Client) (
	*transport.KafkaConfig, error,
) {
	kafkaConfig, ok := transportSecret.Data["kafka.yaml"]
	if !ok {
		return nil, fmt.Errorf("must set the `kafka.yaml` in the transport secret(%s)", transportSecret.Name)
	}

	conn := &transport.KafkaConfig{}
	if err := yaml.Unmarshal(kafkaConfig, conn); err != nil {
		return nil, fmt.Errorf("failed to unmarshal kafka config to transport credentail: %w", err)
	}

	err := ParseCredentailConn(transportSecret.Namespace, c, conn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse the cert credentail: %w", err)
	}
	return conn, nil
}

func ParseCredentailConn(namespace string, c client.Client, conn transport.TransportCerticiate) error {
	// decode the ca cert, client key and cert
	if conn.GetCACert() != "" {
		bytes, err := base64.StdEncoding.DecodeString(conn.GetCACert())
		if err != nil {
			return err
		}
		conn.SetCACert(string(bytes))
	}
	if conn.GetClientCert() != "" {
		bytes, err := base64.StdEncoding.DecodeString(conn.GetClientCert())
		if err != nil {
			return err
		}
		conn.SetClientCert(string(bytes))
	}
	if conn.GetClientKey() != "" {
		bytes, err := base64.StdEncoding.DecodeString(conn.GetClientKey())
		if err != nil {
			return err
		}
		conn.SetClientKey(string(bytes))
	}

	// load the ca cert from secret
	if conn.GetCASecretName() != "" {
		caSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      conn.GetCASecretName(),
			},
		}
		if err := c.Get(context.Background(), client.ObjectKeyFromObject(caSecret), caSecret); err != nil {
			return err
		}
		conn.SetCACert(string(caSecret.Data["ca.crt"]))
	}
	// load the client key and cert from secret
	if conn.GetClientSecretName() != "" {
		clientSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      conn.GetClientSecretName(),
			},
		}
		if err := c.Get(context.Background(), client.ObjectKeyFromObject(clientSecret), clientSecret); err != nil {
			return fmt.Errorf("failed to get the client cert: %w", err)
		}
		conn.SetClientCert(string(clientSecret.Data["tls.crt"]))
		conn.SetClientKey(string(clientSecret.Data["tls.key"]))
		if conn.GetClientCert() == "" || conn.GetClientKey() == "" {
			return fmt.Errorf("the client cert or key must not be empty: %s", conn.GetClientSecretName())
		}
	}
	return nil
}

// GetKafkaUserName gives a kafkaUser name based on the cluster name, it's also the CN of the certificate
func GetKafkaUserName(clusterName string) string {
	return fmt.Sprintf("%s-kafka-user", clusterName)
}
