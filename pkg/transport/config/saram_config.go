package config

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/IBM/sarama"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const errClientCertKeyMismatch = "client certificate and key must be provided together"

var errTLSClientCredentialsRequired = errors.New("client certificate and key are required when TLS is enabled")

// GetSaramaConfig builds a Sarama client config, requiring mTLS materials when TLS is enabled.
func GetSaramaConfig(kafkaConfig *transport.KafkaInternalConfig) (*sarama.Config, error) {
	saramaConfig := sarama.NewConfig()
	saramaConfig.Version = sarama.V2_0_0_0

	if kafkaConfig.EnableTLS {
		var err error
		saramaConfig.Net.TLS.Enable = true
		saramaConfig.Net.TLS.Config, err = NewTLSConfig(kafkaConfig.ClientCertPath, kafkaConfig.ClientKeyPath,
			kafkaConfig.CaCertPath)
		if err != nil {
			return nil, err
		}
	}
	return saramaConfig, nil
}

// NewTLSConfig loads client certificate, key, and CA materials for Kafka TLS.
func NewTLSConfig(clientCertFile, clientKeyFile, caCertFile string) (*tls.Config, error) {
	_, validCert := utils.Validate(clientCertFile)
	_, validKey := utils.Validate(clientKeyFile)
	if validCert != validKey {
		return nil, fmt.Errorf("%s", errClientCertKeyMismatch)
	}
	if !validCert || !validKey {
		return nil, errTLSClientCredentialsRequired
	}

	cert, err := tls.LoadX509KeyPair(clientCertFile, clientKeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load client certificate: %w", err)
	}

	caCert, err := os.ReadFile(filepath.Clean(caCertFile))
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate: %w", err)
	}
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, errors.New("failed to parse CA certificate")
	}

	tlsConfig := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
	}

	return tlsConfig, nil
}
