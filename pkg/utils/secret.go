package utils

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/stretchr/testify/assert/yaml"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

// GetKafkaCredentialBySecret extracts and decodes Kafka credentials from transport-config secret.
// This function avoids importing pkg/transport/config to prevent CGO dependency (librdkafka) in operator build.
func GetKafkaCredentialBySecret(transportSecret *corev1.Secret, c client.Client) (*transport.KafkaConfig, error) {
	kafkaConfigBytes, ok := transportSecret.Data["kafka.yaml"]
	if !ok {
		return nil, fmt.Errorf("must set the `kafka.yaml` in the transport secret(%s)", transportSecret.Name)
	}

	kafkaConfig := &transport.KafkaConfig{}
	if err := yaml.Unmarshal(kafkaConfigBytes, kafkaConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal kafka config: %w", err)
	}

	// Decode base64 encoded certificates
	if err := DecodeTransportCertificate(transportSecret.Namespace, c, kafkaConfig); err != nil {
		return nil, fmt.Errorf("failed to decode credentials: %w", err)
	}

	return kafkaConfig, nil
}

// DecodeTransportCertificate decodes base64 encoded certificates and keys
func DecodeTransportCertificate(namespace string, c client.Client, conn transport.TransportCertificate) error {
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
