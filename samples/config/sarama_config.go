package config

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	"github.com/Shopify/sarama"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetSaramaConfig() (string, *sarama.Config, error) {
	kafkaSecret, err := GetTransportSecret()
	if err != nil {
		return "", nil, err
	}
	bootstrapSever := kafkaSecret.Data["bootstrap_server"]
	caCrt := kafkaSecret.Data["ca.crt"]

	// #nosec G402
	tlsConfig := &tls.Config{}

	// Load CA cert
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCrt)
	tlsConfig.RootCAs = caCertPool

	// Load client cert
	if len(kafkaSecret.Data["client.crt"]) > 0 && len(kafkaSecret.Data["client.key"]) > 0 {
		cert, err := tls.X509KeyPair(kafkaSecret.Data["client.crt"], kafkaSecret.Data["client.key"])
		if err != nil {
			return "", nil, err
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
		tlsConfig.InsecureSkipVerify = false
	} else {
		// #nosec
		tlsConfig.InsecureSkipVerify = true
	}

	// or manual generate client cert(the client ca and crt from the kafka operator)
	// oc get secret kafka-clients-ca -n kafka -ojsonpath='{.data.ca\.key}' | base64 -d > client-ca.key
	// oc get secret kafka-clients-ca-cert -n kafka -ojsonpath='{.data.ca\.crt}' | base64 -d >
	// client-ca.crt
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

func GetSaramaConfigFromKafkaUser() (string, *sarama.Config, error) {
	userName := KAFkA_USER
	kubeconfig, err := loadDynamicKubeConfig(EnvKubconfig)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get kubeconfig")
	}

	kafkav1beta2.AddToScheme(scheme.Scheme)
	c, err := client.New(kubeconfig, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return "", nil, fmt.Errorf("failed to get runtime client")
	}

	// #nosec G402
	tlsConfig := &tls.Config{}

	kafkaCluster := &kafkav1beta2.Kafka{}
	err = c.Get(context.TODO(), types.NamespacedName{
		Name:      KAFKA_CLUSTER,
		Namespace: KAFKA_NAMESPACE,
	}, kafkaCluster)
	if err != nil {
		return "", nil, err
	}

	bootstrapServer := *kafkaCluster.Status.Listeners[1].BootstrapServers

	// Load CA cert
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM([]byte(kafkaCluster.Status.Listeners[1].Certificates[0]))
	tlsConfig.RootCAs = caCertPool

	kafkaUserSecret := &corev1.Secret{}
	err = c.Get(context.TODO(), types.NamespacedName{
		Name:      userName,
		Namespace: KAFKA_NAMESPACE,
	}, kafkaUserSecret)
	if err != nil {
		return "", nil, err
	}

	// Load client cert
	if len(kafkaUserSecret.Data["user.crt"]) > 0 && len(kafkaUserSecret.Data["user.key"]) > 0 {
		cert, err := tls.X509KeyPair(kafkaUserSecret.Data["user.crt"], kafkaUserSecret.Data["user.key"])
		if err != nil {
			return "", nil, err
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
		tlsConfig.InsecureSkipVerify = false
	} else {
		// #nosec
		tlsConfig.InsecureSkipVerify = true
	}

	saramaConfig := sarama.NewConfig()
	saramaConfig.Version = sarama.V2_0_0_0
	saramaConfig.Net.TLS.Config = tlsConfig
	saramaConfig.Net.TLS.Enable = true

	return bootstrapServer, saramaConfig, nil
}
