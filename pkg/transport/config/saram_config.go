package config

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"path/filepath"

	"github.com/Shopify/sarama"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

func GetSaramaConfig(kafkaConfig *transport.KafkaConfig) (*sarama.Config, error) {
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

func NewTLSConfig(clientCertFile, clientKeyFile, caCertFile string) (*tls.Config, error) {
	// #nosec G402
	tlsConfig := tls.Config{}

	// Load CA cert
	caCert, err := ioutil.ReadFile(filepath.Clean(caCertFile))
	if err != nil {
		return &tlsConfig, err
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)
	tlsConfig.RootCAs = caCertPool

	// Load client cert
	cert, err := tls.LoadX509KeyPair(clientCertFile, clientKeyFile)
	if err != nil {
		return &tlsConfig, err
	}
	tlsConfig.Certificates = []tls.Certificate{cert}

	tlsConfig.BuildNameToCertificate()
	return &tlsConfig, err
}
