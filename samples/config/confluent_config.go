package config

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	operatorconfig "github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/addon/certificates"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	KAFKA_USER      = "global-hub-kafka-user"
	KAFKA_CLUSTER   = "kafka"
	KAFKA_NAMESPACE = "multicluster-global-hub"
)

func GetConfluentConfigMapBySecret(isProducer bool) (*kafka.ConfigMap, error) {
	secret, err := GetTransportSecret()
	if err != nil {
		log.Fatalf("failed to get transport secret: %v", err)
		return nil, err
	}
	bootStrapServer := string(secret.Data["bootstrap_server"])

	caCrtPath := "/tmp/ca.crt"
	err = os.WriteFile(caCrtPath, secret.Data["ca.crt"], 0o600)
	if err != nil {
		log.Fatalf("failed to write ca.crt: %v", err)
		return nil, err
	}

	clientCrtPath := "/tmp/client.crt"
	err = os.WriteFile(clientCrtPath, secret.Data["client.crt"], 0o600)
	if err != nil {
		log.Fatalf("failed to write client.crt: %v", err)
		return nil, err
	}

	clientKeyPath := "/tmp/client.key"
	err = os.WriteFile(clientKeyPath, secret.Data["client.key"], 0o600)
	if err != nil {
		log.Fatalf("failed to write client.key: %v", err)
		return nil, err
	}

	kafkaConfig := &transport.KafkaConfig{
		BootstrapServer: bootStrapServer,
		EnableTLS:       true,
		CaCertPath:      caCrtPath,
		ClientCertPath:  clientCrtPath,
		ClientKeyPath:   clientKeyPath,
	}
	configMap, err := config.GetConfluentConfigMap(kafkaConfig, isProducer)
	if err != nil {
		log.Fatalf("failed to get confluent config map: %v", err)
		return nil, err
	}
	return configMap, nil
}

// GetConfluentConfigMap creates the configmap for LH or GH depend on the BOOTSTRAP_SEVER set or not
func GetConfluentConfigMap(producer bool) (*kafka.ConfigMap, error) {
	bootstrapSever := os.Getenv("BOOTSTRAP_SEVER")
	if bootstrapSever == "" {
		return GetConfluentConfigMapFromGlobalHub(producer)
	}
	return GetConfluentConfigMapFromManagedHub(producer)
}

func GetConfluentConfigMapFromManagedHub(producer bool) (*kafka.ConfigMap, error) {
	kubeconfig, err := DefaultKubeConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig")
	}
	c, err := client.New(kubeconfig, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to get runtime client")
	}

	bootstrapSever := os.Getenv("BOOTSTRAP_SEVER")
	if bootstrapSever == "" {
		return nil, fmt.Errorf("Must proivde the bootstrap server: %s", "BOOTSTRAP_SEVER")
	}

	clusterName := os.Getenv("HUB")
	if clusterName == "" {
		return nil, fmt.Errorf("Must set the managed hub: %s", "HUB")
	}

	fmt.Println(">> cluster name:", clusterName)
	namespace := "multicluster-global-hub-agent"
	if ns := os.Getenv("NAMESPACE"); ns != "" {
		namespace = ns
	}

	clientCertSecret := &corev1.Secret{}
	err = c.Get(context.TODO(), types.NamespacedName{
		Name:      certificates.AagentCertificateSecretName(clusterName),
		Namespace: namespace,
	}, clientCertSecret)
	if err != nil {
		return nil, err
	}
	fmt.Println(">> client secret:", clientCertSecret.Name)

	clientCrtPath := "/tmp/client.crt"
	err = os.WriteFile(clientCrtPath, clientCertSecret.Data["tls.crt"], 0o600)
	if err != nil {
		log.Fatalf("failed to write client.crt: %v", err)
		return nil, err
	}

	clientKeyPath := "/tmp/client.key"
	err = os.WriteFile(clientKeyPath, clientCertSecret.Data["tls.key"], 0o600)
	if err != nil {
		log.Fatalf("failed to write client.key: %v", err)
		return nil, err
	}

	caCertSecret := &corev1.Secret{}
	err = c.Get(context.TODO(), types.NamespacedName{
		Name:      "kafka-cluster-ca-cert",
		Namespace: namespace,
	}, caCertSecret)
	if err != nil {
		return nil, err
	}

	fmt.Println(">> cluster ca secret:", caCertSecret.Name)

	caCrtPath := "/tmp/ca.crt"
	err = os.WriteFile(caCrtPath, caCertSecret.Data["ca.crt"], 0o600)
	if err != nil {
		log.Fatalf("failed to write ca.crt: %v", err)
		return nil, err
	}

	kafkaConfig := &transport.KafkaConfig{
		BootstrapServer: bootstrapSever,
		EnableTLS:       true,
		CaCertPath:      caCrtPath,
		ClientCertPath:  clientCrtPath,
		ClientKeyPath:   clientKeyPath,
		ConsumerConfig: &transport.KafkaConsumerConfig{
			ConsumerID: clusterName,
		},
	}
	// true will load the producer config
	configMap, err := config.GetConfluentConfigMap(kafkaConfig, producer)
	if err != nil {
		log.Fatalf("failed to get confluent config map: %v", err)
		return nil, err
	}
	// set the consumer config
	return configMap, nil
}

func GetConfluentConfigMapFromGlobalHub(producer bool) (*kafka.ConfigMap, error) {
	kubeconfig, err := DefaultKubeConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig")
	}
	c, err := client.New(kubeconfig, client.Options{Scheme: operatorconfig.GetRuntimeScheme()})
	if err != nil {
		return nil, fmt.Errorf("failed to get runtime client")
	}

	kafkaUserName := KAFKA_USER
	if user := os.Getenv("KAFKA_USER"); user != "" {
		kafkaUserName = user
	}

	kafkaConfigMap, err := config.GetConfluentConfigMapByUser(c, KAFKA_NAMESPACE, KAFKA_CLUSTER, kafkaUserName)
	if err != nil {
		return nil, err
	}

	if producer {
		config.SetProducerConfig(kafkaConfigMap)
	} else {
		config.SetConsumerConfig(kafkaConfigMap, kafkaUserName)
	}

	return kafkaConfigMap, nil
}
