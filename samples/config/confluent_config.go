package config

import (
	"context"
	"fmt"
	"log"
	"os"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	operatorconfig "github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	kafkaConfig := &transport.KafkaInternalConfig{
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
		return GetConfluentConfigMapFromGlobalHub(KAFKA_USER, producer)
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
		return nil, fmt.Errorf("must proivde the bootstrap server: %s", "BOOTSTRAP_SEVER")
	}

	namespace := "multicluster-global-hub-agent"
	if ns := os.Getenv("NAMESPACE"); ns != "" {
		namespace = ns
	}

	clientCertSecret := &corev1.Secret{}
	err = c.Get(context.TODO(), types.NamespacedName{
		Name:      operatorconfig.AgentCertificateSecretName(),
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

	consumerGroupId := "test-group-id-managed-hub"
	kafkaConfig := &transport.KafkaInternalConfig{
		BootstrapServer: bootstrapSever,
		EnableTLS:       true,
		CaCertPath:      caCrtPath,
		ClientCertPath:  clientCrtPath,
		ClientKeyPath:   clientKeyPath,
		ConsumerConfig: &transport.KafkaConsumerConfig{
			ConsumerID: consumerGroupId,
		},
	}

	fmt.Println(">> consumer group id:", consumerGroupId)
	// true will load the producer config
	configMap, err := config.GetConfluentConfigMap(kafkaConfig, producer)
	if err != nil {
		log.Fatalf("failed to get confluent config map: %v", err)
		return nil, err
	}
	// set the consumer config
	return configMap, nil
}

func GetConfluentConfigMapFromGlobalHub(kafkaUser string, producer bool) (*kafka.ConfigMap, error) {
	kafkaConfigMap, err := GetConfluentConfigMapByCurrentUser(KAFKA_NAMESPACE, KAFKA_CLUSTER, kafkaUser)
	if err != nil {
		return nil, err
	}

	consumerGroupId := "test-group-id" + kafkaUser
	fmt.Println(">> consumer group id:", consumerGroupId)

	if producer {
		config.SetProducerConfig(kafkaConfigMap, constants.KafkaBrokerMessageMaxBytes)
	} else {
		config.SetConsumerConfig(kafkaConfigMap, consumerGroupId, constants.KafkaBrokerMessageMaxBytes)
	}

	return kafkaConfigMap, nil
}

// GetConfluentConfigMapByUser create a kafka.configmap by the kafkauser
func GetConfluentConfigMapByCurrentUser(namespace, clusterName, userName string) (*kafka.ConfigMap, error) {
	kubeconfig, err := DefaultKubeConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig")
	}
	c, err := client.New(kubeconfig, client.Options{Scheme: operatorconfig.GetRuntimeScheme()})
	if err != nil {
		return nil, fmt.Errorf("failed to get runtime client")
	}
	return GetConfluentConfigMapByUser(c, namespace, clusterName, userName)
}

func GetConfluentConfigMapByUser(c client.Client, namespace, clusterName, userName string) (*kafka.ConfigMap, error) {
	kafkaCluster := &kafkav1beta2.Kafka{}
	err := c.Get(context.TODO(), types.NamespacedName{
		Name:      clusterName,
		Namespace: namespace,
	}, kafkaCluster)
	if err != nil {
		return nil, err
	}

	if kafkaCluster.Status == nil || kafkaCluster.Status.Conditions == nil {
		return nil, fmt.Errorf("kafka cluster %s has no status conditions", kafkaCluster.Name)
	}

	kafkaClientCertSecret := &corev1.Secret{}
	err = c.Get(context.TODO(), types.NamespacedName{
		Name:      userName,
		Namespace: namespace,
	}, kafkaClientCertSecret)
	if err != nil {
		return nil, err
	}
	clientCert := string(kafkaClientCertSecret.Data["user.crt"])
	clientKey := string(kafkaClientCertSecret.Data["user.key"])

	cm := config.GetBasicConfigMap()
	for _, condition := range kafkaCluster.Status.Conditions {
		if *condition.Type == "Ready" && *condition.Status == "True" {
			clusterCaCert := kafkaCluster.Status.Listeners[0].Certificates[0]
			_ = cm.SetKey("bootstrap.servers", *kafkaCluster.Status.Listeners[0].BootstrapServers)
			_ = cm.SetKey("security.protocol", "ssl")
			_ = cm.SetKey("ssl.ca.pem", clusterCaCert)
			_ = cm.SetKey("ssl.certificate.pem", clientCert)
			_ = cm.SetKey("ssl.key.pem", clientKey)
			return cm, nil
		}
	}
	return nil, fmt.Errorf("kafka cluster %s/%s is not ready", namespace, clusterName)
}

func GetConfluentConfigMapByTransportConfig(namespace, consumerGroupID string) (*kafka.ConfigMap, error) {
	kubeconfig, err := DefaultKubeConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig")
	}
	c, err := client.New(kubeconfig, client.Options{Scheme: operatorconfig.GetRuntimeScheme()})
	if err != nil {
		return nil, fmt.Errorf("failed to get runtime client")
	}

	if namespace == "" {
		namespace = KAFKA_NAMESPACE
	}

	transportConfig := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      constants.GHTransportConfigSecret,
		},
	}
	err = c.Get(context.Background(), client.ObjectKeyFromObject(transportConfig), transportConfig)
	if err != nil {
		return nil, err
	}
	configMap, err := config.GetConfluentConfigMapByConfig(transportConfig, c, consumerGroupID)
	if err != nil {
		return nil, err
	}
	return configMap, nil
}
