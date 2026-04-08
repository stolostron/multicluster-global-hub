package config

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"

	"github.com/IBM/sarama"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func GetSaramaConfig(kafkaConfig *transport.KafkaInternalConfig) (*sarama.Config, error) {
	saramaConfig := sarama.NewConfig()
	saramaConfig.Version = sarama.V2_0_0_0

	_, validCa := utils.Validate(kafkaConfig.CaCertPath)
	if kafkaConfig.EnableTLS && validCa {
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

func NewTLSConfig(clientCertFile, clientKeyFile, caCertFile string) (*tls.Config, error) {
	tlsConfig := tls.Config{}

	// Load client cert for mutual TLS (optional)
	_, validCert := utils.Validate(clientCertFile)
	_, validKey := utils.Validate(clientKeyFile)
	if validCert && validKey {
		cert, err := tls.LoadX509KeyPair(clientCertFile, clientKeyFile)
		if err != nil {
			return &tlsConfig, fmt.Errorf("failed to load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	// Load CA cert - this is required to verify the server
	caCert, err := os.ReadFile(filepath.Clean(caCertFile))
	if err != nil {
		return &tlsConfig, fmt.Errorf("failed to read CA certificate: %w", err)
	}
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return &tlsConfig, fmt.Errorf("failed to parse CA certificate")
	}
	tlsConfig.RootCAs = caCertPool

	tlsConfig.BuildNameToCertificate()
	return &tlsConfig, nil
}
