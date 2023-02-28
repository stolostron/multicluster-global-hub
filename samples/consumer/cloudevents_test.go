package consumer_test

import (
	"context"
	"fmt"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/protocol"

	"github.com/stolostron/multicluster-global-hub/samples/consumer"
)

const INFORM_POLICY_YAML = "./deploy/inform-limitrange-policy.yaml"

func TestCloudeventsConsumer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	receiver, err := consumer.GetConsumerProtocol()
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
			cancel()
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
