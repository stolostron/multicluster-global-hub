package consumer

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"os/exec"

	"github.com/Shopify/sarama"
	"github.com/cloudevents/sdk-go/protocol/kafka_sarama/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func GetSaramaConfig() (string, *sarama.Config, error) {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		return "", nil, fmt.Errorf("should set the KUBECONFIG environment")
	}
	namespace := os.Getenv("SECRET_NAMESPACE")
	if namespace == "" {
		namespace = "open-cluster-management"
	}
	name := os.Getenv("SECRET_NAME")
	if name == "" {
		name = "transport-secret"
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get kubeconfig %v", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create kubeclient %v", err)
	}
	secret, err := clientset.CoreV1().Secrets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		panic(err.Error())
	}
	bootstrapSever := secret.Data["bootstrap_server"]
	cert := secret.Data["ca.crt"]
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(cert)

	// #nosec
	tlsConfig := &tls.Config{
		RootCAs:            caCertPool,
		InsecureSkipVerify: true,
	}

	saramaConfig := sarama.NewConfig()
	saramaConfig.Version = sarama.V2_0_0_0
	saramaConfig.Net.TLS.Enable = true
	saramaConfig.Net.TLS.Config = tlsConfig
	saramaConfig.Consumer.Offsets.AutoCommit.Enable = false
	saramaConfig.Consumer.Offsets.Initial = sarama.OffsetOldest
	return string(bootstrapSever), saramaConfig, nil
}

func GetConsumerProtocol() (*kafka_sarama.Consumer, error) {
	server, config, err := GetSaramaConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get sarama config")
	}
	return kafka_sarama.NewConsumer([]string{server}, config, "test-group-id", "status")
}

func Kubectl(args ...string) error {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		return fmt.Errorf("should set the KUBECONFIG environment")
	}
	args = append(args, "--kubeconfig", kubeconfig)
	output, err := exec.Command("kubectl", args...).CombinedOutput()
	if err == nil {
		fmt.Println(string(output))
	}
	return err
}

// Consumer represents a Sarama consumer group consumer
type Consumer struct {
	ready chan bool
}

func GetSaramaGroupConsumer() *Consumer {
	return &Consumer{
		ready: make(chan bool),
	}
}

func (consumer *Consumer) ReadyChan() chan bool {
	return consumer.ready
}

// Setup is run at the beginning of a new session, before ConsumeClaim
func (consumer *Consumer) Setup(sarama.ConsumerGroupSession) error {
	// Mark the consumer as ready
	close(consumer.ready)
	return nil
}

// Cleanup is run at the end of a session, once all ConsumeClaim goroutines have exited
func (consumer *Consumer) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

// ConsumeClaim must start a consumer loop of ConsumerGroupClaim's Messages().
func (consumer *Consumer) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	// NOTE:
	// Do not move the code below to a goroutine.
	// The `ConsumeClaim` itself is called within a goroutine, see:
	// https://github.com/Shopify/sarama/blob/main/consumer_group.go#L27-L29
	for {
		select {
		case message := <-claim.Messages():
			fmt.Printf("Message claimed:[%d-%d] value = %s, timestamp = %v, topic = %s", message.Partition,
				message.Offset, string(message.Value), message.Timestamp, message.Topic)
			session.MarkMessage(message, "")

		// Should return when `session.Context()` is done.
		// If not, will raise `ErrRebalanceInProgress` or `read tcp <ip>:<port>: i/o timeout` when kafka rebalance. see:
		// https://github.com/Shopify/sarama/issues/1192
		case <-session.Context().Done():
			return nil
		}
	}
}
