package config

import (
	"crypto/tls"
	"crypto/x509"
	"os"

	"github.com/Shopify/sarama"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func GetSaramaConfig(kafkaConfig *transport.KafkaConfig) (*sarama.Config, error) {
	saramaConfig := sarama.NewConfig()
	saramaConfig.Version = sarama.V2_0_0_0

	if kafkaConfig.EnableTLS {
		var err error
		saramaConfig.Net.TLS.Enable = true
		saramaConfig.Net.TLS.Config, err = NewTLSConfig(
			kafkaConfig.ConnCredential.CACert,
			kafkaConfig.ConnCredential.ClientCert,
			kafkaConfig.ConnCredential.ClientKey)
		if err != nil {
			return nil, err
		}
	}
	return saramaConfig, nil
}

func NewTLSConfig(caCert, clientCert, clientKey string) (*tls.Config, error) {
	// #nosec G402
	tlsConfig := tls.Config{}

	// Load client cert
	clientCertFile := "/tmp/kafka_client.crt"
	clientKeyFile := "/tmp/kafka_client.key"
	if err := os.WriteFile(clientCertFile, []byte(clientCert), 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(clientKeyFile, []byte(clientKey), 0o644); err != nil {
		return nil, err
	}

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
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM([]byte(caCert))
	tlsConfig.RootCAs = caCertPool

	tlsConfig.BuildNameToCertificate()
	return &tlsConfig, nil
}
