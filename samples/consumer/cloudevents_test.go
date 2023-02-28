package consumer

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/Shopify/sarama"
	"github.com/cloudevents/sdk-go/protocol/kafka_sarama/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/protocol"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const INFORM_POLICY_YAML = "./deploy/inform-limitrange-policy.yaml"

func TestCloudeventsWithACK(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	// wait 1 minute to close the connection
	go func() {
		time.Sleep(1 * time.Minute)
		cancel()
	}()

	server, config, err := GetSaramaConfig()
	// if set this to false, it will consume message from beginning when restart the client
	config.Consumer.Offsets.AutoCommit.Enable = true
	if err != nil {
		t.Fatalf("failed to get sarama config")
	}
	receiver, err := kafka_sarama.NewConsumer([]string{server}, config, "test-group-ack", "status")
	if err != nil {
		t.Fatalf("failed to create cloudevents client: %s", err)
	}
	defer receiver.Close(ctx)
	c, err := cloudevents.NewClient(receiver)
	if err != nil {
		t.Fatalf("failed to create client, %v", err)
	}

	// Start the receiver
	go func() {
		err = c.StartReceiver(ctx, func(ctx context.Context, event cloudevents.Event) protocol.Result {
			fmt.Println("=====================")
			fmt.Printf("%s", event)
			return protocol.ResultACK
		})
		if err != nil {
			fmt.Printf("failed to start receiver: %s", err)
		}
	}()

	// consumer.Kubectl("apply", "-f", INFORM_POLICY_YAML)
	<-ctx.Done()
	// consumer.Kubectl("delete", "-f", INFORM_POLICY_YAML)
}

func TestCloudeventsWithNACK(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	// wait 1 minute to close the connection
	go func() {
		time.Sleep(1 * time.Minute)
		cancel()
	}()

	server, config, err := GetSaramaConfig()
	// if set this to false, it will consume message from beginning when restart the client
	config.Consumer.Offsets.AutoCommit.Enable = true
	if err != nil {
		t.Fatalf("failed to get sarama config")
	}
	receiver, err := kafka_sarama.NewConsumer([]string{server}, config, "test-group-nack", "status")
	if err != nil {
		t.Fatalf("failed to create cloudevents client: %s", err)
	}
	defer receiver.Close(ctx)
	c, err := cloudevents.NewClient(receiver)
	if err != nil {
		t.Fatalf("failed to create client, %v", err)
	}

	// Start the receiver
	go func() {
		err = c.StartReceiver(ctx, func(ctx context.Context, event cloudevents.Event) protocol.Result {
			fmt.Println("=====================")
			fmt.Printf("%s", event)
			return protocol.ResultNACK
		})
		if err != nil {
			fmt.Printf("failed to start receiver: %s", err)
		}
	}()

	<-ctx.Done()
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
	saramaConfig.Consumer.Offsets.Initial = sarama.OffsetOldest
	return string(bootstrapSever), saramaConfig, nil
}
