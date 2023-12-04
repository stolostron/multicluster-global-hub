package config

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	"github.com/Shopify/sarama"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	EnvUserName  = "KAFKA_USER"
	EnvKubconfig = "KUBECONFIG"
)

func GetSaramaConfigFromKafkaUser() (string, *sarama.Config, error) {
	userName := os.Getenv(EnvUserName)
	if userName == "" {
		return "", nil, fmt.Errorf("not found env %s", EnvUserName)
	}

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
		Name:      "kafka",
		Namespace: "multicluster-global-hub",
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
		Namespace: "multicluster-global-hub",
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
