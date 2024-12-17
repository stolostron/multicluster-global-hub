package config

import (
	"crypto/tls"
	"crypto/x509"
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
	// #nosec G402
	tlsConfig := tls.Config{}

	// Load client cert
	_, validCert := utils.Validate(clientCertFile)
	_, validKey := utils.Validate(clientKeyFile)
	if validCert && validKey {
		cert, err := tls.LoadX509KeyPair(clientCertFile, clientKeyFile)
		if err != nil {
			return &tlsConfig, err
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	} else {
		// #nosec
		tlsConfig.InsecureSkipVerify = true
	}

	// Load CA cert
	caCert, err := os.ReadFile(filepath.Clean(caCertFile))
	if err != nil {
		return &tlsConfig, err
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)
	tlsConfig.RootCAs = caCertPool

	tlsConfig.BuildNameToCertificate()
	return &tlsConfig, err
}
