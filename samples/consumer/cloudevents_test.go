package consumer

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Shopify/sarama"
	"github.com/cloudevents/sdk-go/protocol/kafka_sarama/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/cloudevents/sdk-go/v2/protocol"
	"github.com/cloudevents/sdk-go/v2/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const INFORM_POLICY_YAML = "./deploy/inform-limitrange-policy.yaml"

func TestCloudeventsWithACK(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	// wait 1 minute to close the connection
	go func() {
		time.Sleep(2 * time.Minute)
		cancel()
	}()

	server, config, err := GetSaramaConfig()
	// if set this to false, it will consume message from beginning when restart the client
	if err != nil {
		t.Fatalf("failed to get sarama config")
	}
	config.Consumer.Offsets.AutoCommit.Enable = false

	receiver, err := kafka_sarama.NewConsumer([]string{server}, config, "hub3", "events")
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

func TestCloudeventsProducer(t *testing.T) {
	server, config, err := GetSaramaConfig()
	// if set this to false, it will consume message from beginning when restart the client
	if err != nil {
		t.Fatalf("failed to get sarama config")
	}

	sender, _ := kafka_sarama.NewSender([]string{server}, config, "events")
	s, err := cloudevents.NewClient(sender)
	if err != nil {
		t.Fatalf("failed to create client, %v", err)
	}

	s.Send(kafka_sarama.WithMessageKey(context.TODO(), sarama.StringEncoder("hello")), FullEvent())

	fmt.Println("send successfully")

}

var (
	Source    = types.URIRef{URL: url.URL{Scheme: "http", Host: "example.com", Path: "/source"}}
	Timestamp = types.Timestamp{Time: time.Now()}
	Schema    = types.URI{URL: url.URL{Scheme: "http", Host: "example.com", Path: "/schema"}}
)

func FullEvent() event.Event {
	e := event.Event{
		Context: event.EventContextV1{
			Type:       "com.example.FullEvent",
			Source:     Source,
			ID:         "full-event-1",
			Time:       &Timestamp,
			DataSchema: &Schema,
			Subject:    strptr("topic"),
		}.AsV1(),
	}

	e.SetExtension("exbool", true)
	e.SetExtension("exint", 42)
	e.SetExtension("exstring", "exstring")
	e.SetExtension("exbinary", []byte{0, 1, 2, 3})
	e.SetExtension("exurl", Source)
	e.SetExtension("extime", Timestamp)

	if err := e.SetData("text/json", "hello"); err != nil {
		panic(err)
	}
	return e
}

func strptr(s string) *string { return &s }

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
	saramaConfig.Version = sarama.MaxVersion
	saramaConfig.Net.TLS.Enable = true
	saramaConfig.Net.TLS.Config = tlsConfig
	saramaConfig.Consumer.Offsets.Initial = sarama.OffsetNewest
	saramaConfig.Consumer.Offsets.AutoCommit.Enable = true

	return string(bootstrapSever), saramaConfig, nil
}

const count = 10

func TestCloudeventsAAA(t *testing.T) {
	fmt.Println("here")
	server, config, err := GetSaramaConfig()

	// With NewProtocol you can use the same client both to send and receive.
	protocol, err := kafka_sarama.NewProtocol([]string{server}, config, "events", "events")
	if err != nil {
		t.Fatalf("failed to create protocol: %s", err.Error())
	}

	defer protocol.Close(context.Background())

	c, err := cloudevents.NewClient(protocol, cloudevents.WithTimeNow(), cloudevents.WithUUIDs())
	if err != nil {
		t.Fatalf("failed to create client, %v", err)
	}

	// Create a done channel to block until we've received (count) messages
	done := make(chan struct{})

	// Start the receiver
	go func() {
		fmt.Printf("will listen consuming topic events\n")
		var recvCount int32
		err = c.StartReceiver(context.TODO(), func(ctx context.Context, event cloudevents.Event) {
			receive(ctx, event)
			if atomic.AddInt32(&recvCount, 1) == count*100 {
				done <- struct{}{}
			}
		})
		if err != nil {
			fmt.Printf("failed to start receiver: %s", err)
		} else {
			fmt.Printf("receiver stopped\n")
		}
	}()

	// Start sending the events
	for i := 0; i < count; i++ {
		e := cloudevents.NewEvent()
		e.SetType("com.cloudevents.sample.sent")
		e.SetSource("https://github.com/cloudevents/sdk-go/samples/kafka/sender-receiver")
		_ = e.SetData(cloudevents.ApplicationJSON, map[string]interface{}{
			"id":      i,
			"message": "Hello, World!",
		})

		if result := c.Send(kafka_sarama.WithMessageKey(context.TODO(), sarama.StringEncoder("hello")), e); cloudevents.IsUndelivered(result) {
			log.Printf("failed to send: %v", result)
		} else {
			log.Printf("sent: %d, accepted: %t", i, cloudevents.IsACK(result))
		}
	}

	<-done
}

func receive(ctx context.Context, event cloudevents.Event) {
	log.Printf("receive %s", event)
}
