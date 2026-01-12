package transport_test

import (
	"context"
	"fmt"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/consumer"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/producer"
)

func TestAssembler(t *testing.T) {
	transportConfig := &transport.TransportInternalConfig{
		TransportType: string(transport.Chan),
		KafkaCredential: &transport.KafkaConfig{
			SpecTopic:   "spec",
			StatusTopic: "status",
		},
	}

	genericProducer, err := producer.NewGenericProducer(transportConfig,
		transportConfig.KafkaCredential.SpecTopic, nil)
	assert.Nil(t, err)
	genericProducer.SetDataLimit(5)

	genericConsumer, err := consumer.NewGenericConsumer(false, false)
	assert.Nil(t, err)

	go func() {
		err = genericConsumer.Start(context.TODO())
		assert.Nil(t, err)
	}()

	// Send config to consumer to trigger initialization
	genericConsumer.ConfigChan() <- transportConfig

	e := cloudevents.NewEvent()
	e.SetID(uuid.New().String())
	e.SetType("com.cloudevents.sample.sent")
	e.SetSource("https://github.com/cloudevents/sdk-go/samples/kafka/sender")
	_ = e.SetData(cloudevents.ApplicationJSON, map[string]interface{}{
		"id":      0,
		"message": "Hello, World!",
	})

	err = genericProducer.SendEvent(context.TODO(), e)
	assert.Nil(t, err)

	evt := <-genericConsumer.EventChan()
	fmt.Println("whole", evt)
}
