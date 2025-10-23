package consumer

import (
	"context"

	"github.com/IBM/sarama"
	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/config"
)

type SaramaConsumer interface {
	Start(ctx context.Context) error
	MessageChan() chan *sarama.ConsumerMessage
	MarkOffset(topic string, partition int32, offset int64)
}

type saramaConsumer struct {
	ctx           context.Context
	log           logr.Logger
	kafkaConfig   *transport.KafkaInternalConfig
	client        sarama.ConsumerGroup
	messageChan   chan *sarama.ConsumerMessage
	processedChan chan *sarama.ConsumerMessage
	topics        []string
}

func NewSaramaConsumer(ctx context.Context, kafkaConfig *transport.KafkaInternalConfig,
	topics []string,
) (SaramaConsumer, error) {
	log := ctrl.Log.WithName("sarama-consumer")
	saramaConfig, err := config.GetSaramaConfig(kafkaConfig)
	if err != nil {
		return nil, err
	}
	saramaConfig.Consumer.Offsets.AutoCommit.Enable = true
	saramaConfig.Consumer.Offsets.Initial = sarama.OffsetOldest

	client, err := sarama.NewConsumerGroup([]string{kafkaConfig.BootstrapServer}, kafkaConfig.ConsumerConfig.ConsumerID,
		saramaConfig)
	if err != nil {
		return nil, err
	}

	// used to commit offset asynchronously
	processedChan := make(chan *sarama.ConsumerMessage)
	messageChan := make(chan *sarama.ConsumerMessage)

	consumer := &saramaConsumer{
		ctx:           ctx,
		log:           log,
		kafkaConfig:   kafkaConfig,
		client:        client,
		messageChan:   messageChan,
		processedChan: processedChan,
		topics:        topics,
	}
	return consumer, nil
}

func (c *saramaConsumer) MessageChan() chan *sarama.ConsumerMessage {
	return c.messageChan
}

func (c *saramaConsumer) MarkOffset(topic string, partition int32, offset int64) {
	c.processedChan <- &sarama.ConsumerMessage{
		Topic:     topic,
		Partition: partition,
		Offset:    offset,
	}
}

func (c *saramaConsumer) Start(ctx context.Context) error {
	for {
		err := c.client.Consume(ctx, c.topics, &consumeGroupHandler{
			log:           c.log.WithName("handler"),
			messageChan:   c.messageChan,
			processedChan: c.processedChan,
		})
		if err != nil {
			c.log.Error(err, "Error from sarama consumer", "topic", c.topics)
		}
		// check if context was cancelled, signaling that the consumer should stop
		if ctx.Err() != nil {
			c.log.Info("context was cancelled, signaling that the consumer should stop", "ctx err", ctx.Err())
			close(c.messageChan)
			return c.client.Close()
		}
	}
}

// Consumer represents a Sarama consumer group consumer
type consumeGroupHandler struct {
	log           logr.Logger
	messageChan   chan *sarama.ConsumerMessage
	processedChan chan *sarama.ConsumerMessage
}

// Setup is run at the beginning of a new session, before ConsumeClaim
func (cgh *consumeGroupHandler) Setup(session sarama.ConsumerGroupSession) error {
	cgh.log.Info("Sarama consumer handler setup")
	go func() {
		for {
			select {
			case msg := <-cgh.processedChan:
				cgh.log.V(2).Info("mark offset", "topic", msg.Topic, "partition", msg.Partition, "offset", msg.Offset)
				session.MarkMessage(msg, "")
			case <-session.Context().Done():
				return
			}
		}
	}()
	return nil
}

// Cleanup is run at the end of a session, once all ConsumeClaim goroutines have exited
func (cgh *consumeGroupHandler) Cleanup(sarama.ConsumerGroupSession) error {
	cgh.log.Info("Sarama consumer handler cleanup")
	return nil
}

// ConsumeClaim must start a consumer loop of ConsumerGroupClaim's Messages().
func (cgh *consumeGroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession,
	claim sarama.ConsumerGroupClaim,
) error {
	for {
		select {
		case message := <-claim.Messages():
			cgh.messageChan <- message
			// session.MarkMessage(message, "")
		case <-session.Context().Done():
			return nil
		}
	}
}
