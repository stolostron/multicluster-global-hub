package consumer

import (
	"context"

	"github.com/Shopify/sarama"
	"github.com/go-logr/logr"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/config"
	ctrl "sigs.k8s.io/controller-runtime"
)

type SaramaConsumer struct {
	ctx         context.Context
	log         logr.Logger
	kafkaConfig *transport.KafkaConfig
	client      sarama.ConsumerGroup
	handler     sarama.ConsumerGroupHandler
	messageChan chan *sarama.ConsumerMessage
}

func NewSaramaConsumer(ctx context.Context, kafkaConfig *transport.KafkaConfig,
	messageChan chan *sarama.ConsumerMessage,
) (*SaramaConsumer, error) {
	log := ctrl.Log.WithName("sarama-consumer")
	saramaConfig, err := config.GetSaramaConfig(kafkaConfig)
	if err != nil {
		return nil, err
	}
	saramaConfig.Consumer.Offsets.AutoCommit.Enable = true
	saramaConfig.Consumer.Offsets.Initial = sarama.OffsetOldest

	client, err := sarama.NewConsumerGroup([]string{kafkaConfig.BootstrapServer}, kafkaConfig.ConsumerConfig.ConsumerID, saramaConfig)
	if err != nil {
		return nil, err
	}

	handler := &consumeGroupHandler{
		log:         log.WithName("handler"),
		messageChan: messageChan,
	}

	return &SaramaConsumer{
		ctx:         ctx,
		log:         log,
		kafkaConfig: kafkaConfig,
		client:      client,
		handler:     handler,
		messageChan: messageChan,
	}, nil
}

func (c *SaramaConsumer) Start(ctx context.Context) error {
	for {
		// `Consume` should be called inside an infinite loop, when a
		// server-side rebalance happens, the consumer session will need to be
		// recreated to get the new claims
		if err := c.client.Consume(ctx, []string{c.kafkaConfig.ConsumerConfig.ConsumerTopic}, c.handler); err != nil {
			c.log.Error(err, "Error from sarama consumer")
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
	log         logr.Logger
	messageChan chan *sarama.ConsumerMessage
}

// Setup is run at the beginning of a new session, before ConsumeClaim
func (cgh *consumeGroupHandler) Setup(sarama.ConsumerGroupSession) error {
	cgh.log.Info("Sarama consumer handler setup")
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
	// NOTE:
	// Do not move the code below to a goroutine.
	// The `ConsumeClaim` itself is called within a goroutine, see:
	// https://github.com/Shopify/sarama/blob/main/consumer_group.go#L27-L29
	for {
		select {
		case message := <-claim.Messages():
			cgh.messageChan <- message
			session.MarkMessage(message, "")

		// Should return when `session.Context()` is done.
		// If not, will raise `ErrRebalanceInProgress` or `read tcp <ip>:<port>: i/o timeout` when kafka rebalance. see:
		// https://github.com/Shopify/sarama/issues/1192
		case <-session.Context().Done():
			return nil
		}
	}
}
