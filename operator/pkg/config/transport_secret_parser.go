package config

import (
	"context"
	"encoding/base64"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/kyaml/yaml"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

// ParseTransportSecret extracts and decodes Kafka credentials from transport-config secret.
// This function avoids importing pkg/transport/config to prevent CGO dependency (librdkafka) in operator build.
func ParseTransportSecret(transportSecret *corev1.Secret, c client.Client) (*transport.KafkaConfig, error) {
	kafkaConfigBytes, ok := transportSecret.Data["kafka.yaml"]
	if !ok {
		return nil, fmt.Errorf("must set the `kafka.yaml` in the transport secret(%s)", transportSecret.Name)
	}

	kafkaConfig := &transport.KafkaConfig{}
	if err := yaml.Unmarshal(kafkaConfigBytes, kafkaConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal kafka config: %w", err)
	}

	// Decode base64 encoded certificates
	if err := decodeCredentials(transportSecret.Namespace, c, kafkaConfig); err != nil {
		return nil, fmt.Errorf("failed to decode credentials: %w", err)
	}

	return kafkaConfig, nil
}

// decodeCredentials decodes base64 encoded certificates and keys
func decodeCredentials(namespace string, c client.Client, kafkaConfig *transport.KafkaConfig) error {
	// Decode CA cert
	if kafkaConfig.CACert != "" {
		decoded, err := base64.StdEncoding.DecodeString(kafkaConfig.CACert)
		if err != nil {
			return fmt.Errorf("failed to decode CA cert: %w", err)
		}
		kafkaConfig.CACert = string(decoded)
	}

	// Decode client cert
	if kafkaConfig.ClientCert != "" {
		decoded, err := base64.StdEncoding.DecodeString(kafkaConfig.ClientCert)
		if err != nil {
			return fmt.Errorf("failed to decode client cert: %w", err)
		}
		kafkaConfig.ClientCert = string(decoded)
	}

	// Decode client key
	if kafkaConfig.ClientKey != "" {
		decoded, err := base64.StdEncoding.DecodeString(kafkaConfig.ClientKey)
		if err != nil {
			return fmt.Errorf("failed to decode client key: %w", err)
		}
		kafkaConfig.ClientKey = string(decoded)
	}

	// Load CA cert from separate secret if specified
	if kafkaConfig.CASecretName != "" {
		caSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      kafkaConfig.CASecretName,
			},
		}
		if err := c.Get(context.Background(), client.ObjectKeyFromObject(caSecret), caSecret); err != nil {
			return fmt.Errorf("failed to get CA secret: %w", err)
		}
		kafkaConfig.CACert = string(caSecret.Data["ca.crt"])
	}

	// Load client cert/key from separate secret if specified
	if kafkaConfig.ClientSecretName != "" {
		clientSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      kafkaConfig.ClientSecretName,
			},
		}
		if err := c.Get(context.Background(), client.ObjectKeyFromObject(clientSecret), clientSecret); err != nil {
			return fmt.Errorf("failed to get client secret: %w", err)
		}
		kafkaConfig.ClientCert = string(clientSecret.Data["tls.crt"])
		kafkaConfig.ClientKey = string(clientSecret.Data["tls.key"])
		if kafkaConfig.ClientCert == "" || kafkaConfig.ClientKey == "" {
			return fmt.Errorf("client cert or key is empty in secret: %s", kafkaConfig.ClientSecretName)
		}
	}

	return nil
}
