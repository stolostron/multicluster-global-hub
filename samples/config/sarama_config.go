package config

import (
	"crypto/tls"
	"crypto/x509"

	"github.com/Shopify/sarama"
)

func GetSaramaConfig() (string, *sarama.Config, error) {
	kafkaSecret, err := GetTransportSecret()
	if err != nil {
		return "", nil, err
	}
	bootstrapSever := kafkaSecret.Data["bootstrap_server"]
	caCrt := kafkaSecret.Data["ca.crt"]
	cert, err := tls.X509KeyPair(kafkaSecret.Data["client.crt"], kafkaSecret.Data["client.key"])
	if err != nil {
		return "", nil, err
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCrt)
	tlsConfig := &tls.Config{
		RootCAs:            caCertPool,
		InsecureSkipVerify: false,
		Certificates:       []tls.Certificate{cert},
	}

	// or manual generate client cert(the client ca and crt from the kafka operator)
	// oc get secret kafka-brokers-cluster-clients-ca -n kafka -ojsonpath='{.data.ca\.key}' | base64 -d > client-ca.key
	// oc get secret kafka-brokers-cluster-clients-ca-cert -n kafka -ojsonpath='{.data.ca\.crt}' | base64 -d > client-ca.crt
	// openssl genrsa -out client.key 2048
	// openssl req -new -key client.key -out client.csr -subj "/CN=global-hub"
	// openssl x509 -req -in client.csr -CA client-ca.crt -CAkey client-ca.key -CAcreateserial -out client.crt -days 365
	// tlsConfig, err = config.NewTLSConfig(<path-client.crt>, <path-client.key>, <path-ca.crt>)

	saramaConfig := sarama.NewConfig()
	saramaConfig.Version = sarama.V2_0_0_0
	saramaConfig.Net.TLS.Config = tlsConfig
	saramaConfig.Net.TLS.Enable = true

	return string(bootstrapSever), saramaConfig, nil
}
