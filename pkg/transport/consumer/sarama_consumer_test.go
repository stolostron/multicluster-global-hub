package consumer

import (
	"context"
	"testing"

	"github.com/IBM/sarama"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/config"
)

func TestConsumerGroup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Given
	responses := []string{"Foo", "Bar"}
	kafkaCluster := MockSaramaCluster(t, responses)
	defer kafkaCluster.Close()

	kafkaConfig := &transport.KafkaInternalConfig{
		BootstrapServer: kafkaCluster.Addr(),
		EnableTLS:       false,
		ConsumerConfig: &transport.KafkaConsumerConfig{
			ConsumerID: "test-consumer",
		},
	}

	// consumer, err := NewSaramaConsumer(ctx, kafkaConfig)
	// if err != nil {
	// 	t.Errorf("failed to init sarama consumer - %v", err)
	// }
	// go func() {
	// 	_ = consumer.Start(ctx)
	// }()

	// msg := <-consumer.MessageChan()
	// fmt.Printf("msg: %v\n", msg)
	// time.Sleep(5 * time.Second)
	// cancel()

	saramaConfig, err := config.GetSaramaConfig(kafkaConfig)
	if err != nil {
		t.Fatal(err)
	}
	saramaConfig.Consumer.Offsets.AutoCommit.Enable = true
	saramaConfig.Consumer.Offsets.Initial = sarama.OffsetOldest

	group, err := sarama.NewConsumerGroup([]string{kafkaConfig.BootstrapServer}, "my-group", saramaConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = group.Close() }()

	// used to commit offset asynchronously
	processedChan := make(chan *sarama.ConsumerMessage)
	messageChan := make(chan *sarama.ConsumerMessage)

	// test handler
	handler := &consumeGroupHandler{
		log:           ctrl.Log.WithName("sarama-consumer").WithName("handler"),
		messageChan:   messageChan,
		processedChan: processedChan,
	}

	go func() {
		topics := []string{"my-topic"}
		for {
			if err := group.Consume(ctx, topics, handler); err != nil {
				t.Error(err)
			}
			// check if context was cancelled, signaling that the consumer should stop
			if ctx.Err() != nil {
				return
			}
		}
	}()

	msg := <-messageChan
	got1 := string(msg.Value)
	if got1 != responses[0] {
		t.Errorf("got %s, want %s", got1, responses[0])
	}

	msg = <-messageChan
	got2 := string(msg.Value)
	if got2 != responses[1] {
		t.Errorf("got %s, want %s", got2, responses[1])
	}

	cancel()
}

// the default offset range is 0 ~ 100, topic is "my-topic", partition is 0
func MockSaramaCluster(t *testing.T, messages []string) *sarama.MockBroker {
	// mockFetchResponse := sarama.NewMockFetchResponse(t, 1).
	// 	SetMessage("my-topic", 0, 0, sarama.StringEncoder("foo")).
	// 	SetMessage("my-topic", 0, 1, sarama.StringEncoder("bar")).
	// 	SetMessage("my-topic", 0, 2, sarama.StringEncoder("baz")).
	// 	SetMessage("my-topic", 0, 3, sarama.StringEncoder("qux"))
	oldestOffset := int64(0)
	newestOffset := int64(100)
	mockFetchResponse := sarama.NewMockFetchResponse(t, 1)
	for i, msg := range messages {
		mockFetchResponse = mockFetchResponse.SetMessage("my-topic", 0, oldestOffset+int64(i), sarama.StringEncoder(msg))
	}

	broker0 := sarama.NewMockBroker(t, 0)
	broker0.SetHandlerByMap(map[string]sarama.MockResponse{
		"MetadataRequest": sarama.NewMockMetadataResponse(t).
			SetBroker(broker0.Addr(), broker0.BrokerID()).
			SetLeader("my-topic", 0, broker0.BrokerID()),
		"OffsetRequest": sarama.NewMockOffsetResponse(t).
			SetOffset("my-topic", 0, sarama.OffsetOldest, oldestOffset).
			SetOffset("my-topic", 0, sarama.OffsetNewest, newestOffset),
		"FindCoordinatorRequest": sarama.NewMockFindCoordinatorResponse(t).
			SetCoordinator(sarama.CoordinatorGroup, "my-group", broker0),
		"HeartbeatRequest": sarama.NewMockHeartbeatResponse(t),
		"JoinGroupRequest": sarama.NewMockSequence(
			sarama.NewMockJoinGroupResponse(t).SetError(sarama.ErrOffsetsLoadInProgress),
			sarama.NewMockJoinGroupResponse(t).SetGroupProtocol(sarama.RangeBalanceStrategyName),
		),
		"SyncGroupRequest": sarama.NewMockSequence(
			sarama.NewMockSyncGroupResponse(t).SetError(sarama.ErrOffsetsLoadInProgress),
			sarama.NewMockSyncGroupResponse(t).SetMemberAssignment(
				&sarama.ConsumerGroupMemberAssignment{
					Version: 0,
					Topics: map[string][]int32{
						"my-topic": {0},
					},
				}),
		),
		"OffsetFetchRequest": sarama.NewMockOffsetFetchResponse(t).SetOffset(
			"my-group", "my-topic", 0, oldestOffset, "", sarama.ErrNoError,
		).SetError(sarama.ErrNoError),
		"FetchRequest": sarama.NewMockSequence(
			mockFetchResponse,
		),
	})
	return broker0
}

func MockTransportSecret(c client.Client, namespace string) error {
	// Check if the namespace already exists
	err := c.Create(context.TODO(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	})
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	// Replace the following placeholders with your actual data
	data := map[string][]byte{
		"bootstrap_server": []byte("your_bootstrap_server_data"),
		"ca.crt":           []byte("your_ca_crt_data"),
		"client.crt":       []byte("your_client_crt_data"),
		"client.key":       []byte("your_client_key_data"),
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.GHTransportSecretName,
			Namespace: namespace,
		},
		Data: data,
		Type: corev1.SecretTypeOpaque,
	}

	err = c.Create(context.TODO(), secret)
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}
	return nil
}
