package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/stolostron/multicluster-global-hub/samples/config"
)

const (
	messageCount = 10
	consumerId   = "consumer-group-1"
)


func main() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	if len(os.Args) < 2 {
		fmt.Println("Please provide at least one topic command-line argument.")
		os.Exit(1)
	}
	topic := os.Args[1]

	// kafkaConfigMap, err := config.GetConfluentConfigMap()
	kafkaConfigMap, err := config.GetConfluentConfigMap(false)
	if err != nil {
		log.Fatalf("failed to get kafka config map: %v", err)
	}
	// _ = kafkaConfigMap.SetKey("client.id", consumerId)
	_ = kafkaConfigMap.SetKey("group.id", consumerId)
	_ = kafkaConfigMap.SetKey("auto.offset.reset", "earliest")
	_ = kafkaConfigMap.SetKey("enable.auto.commit", "true")

	consumer, err := kafka.NewConsumer(kafkaConfigMap)
	if err != nil {
		log.Fatalf("failed to create kafka consumer: %v", err)
	}

	log.Printf(">> subscribe topic %s", topic)
	if err := consumer.SubscribeTopics([]string{topic}, rebalanceCallback); err != nil {
		log.Fatalf("failed to subscribe topic: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for {
			select {
			case <-ctx.Done():
				_ = consumer.Unsubscribe()
				log.Printf("unsubscribed topic: %s", topic)
				return
			default:
				ev := consumer.Poll(100)
				if ev == nil {
					continue
				}
				if err := processEvent(consumer, ev); err != nil {
					log.Printf("failed to process event: %s \n", ev)
				}

			}
		}
	}()

	// committer := NewCommitter(5*time.Second, topic, consumer)
	// committer := NewCommitter(5*time.Second, topic, consumer, getKafkaMessages)
	// committer.start(ctx)

	sig := <-signals
	log.Printf("got signal: %s\n", sig.String())
	cancel()
	log.Println("context is done")

	// graceful shutdown
	time.Sleep(1 * time.Second)
	log.Printf("exit main")
	os.Exit(0)
}

// processEvent processes the message/error received from the kafka Consumer's
// Poll() method.
func processEvent(c *kafka.Consumer, ev kafka.Event) error {
	switch e := ev.(type) {

	case *kafka.Message:
		if e.TopicPartition.Error != nil {
			log.Printf("failed message on %s [%d %v]: %v\n", *e.TopicPartition.Topic, e.TopicPartition.Partition, e.TopicPartition.Offset, e.TopicPartition.Error)
		} else {
			log.Printf("received message on %s [%d %v]: %s\n", *e.TopicPartition.Topic, e.TopicPartition.Partition, e.TopicPartition.Offset, e.Value)
		}

		// https://github.com/confluentinc/confluent-kafka-go/blob/master/examples/consumer_rebalance_example/consumer_rebalance_example.go
		// // Handle manual commit since enable.auto.commit is unset.
		// if err := maybeCommit(c, e.TopicPartition); err != nil {
		// 	return err
		// }

	case kafka.Error:
		// Errors should generally be considered informational, the client
		// will try to automatically recover.
		return fmt.Errorf("kafka error with %v", e)
	default:
		// log.Printf("ignored event %v\n", e)
	}

	return nil
}


// rebalanceCallback is called on each group rebalance to assign additional
// partitions, or remove existing partitions, from the consumer's current
// assignment.
//
// A rebalance occurs when a consumer joins or leaves a consumer group, if it
// changes the topic(s) it's subscribed to, or if there's a change in one of
// the topics it's subscribed to, for example, the total number of partitions
// increases.
//
// The application may use this optional callback to inspect the assignment,
// alter the initial start offset (the .Offset field of each assigned partition),
// and read/write offsets to commit to an alternative store outside of Kafka.
func rebalanceCallback(c *kafka.Consumer, event kafka.Event) error {
	switch ev := event.(type) {
	case kafka.AssignedPartitions:
		// log.Printf("%% %s rebalance: %d new partition(s) assigned: %v\n", c.GetRebalanceProtocol(), len(ev.Partitions), ev.Partitions)

		log.Printf("%% %s rebalance: %d new partition(s) assigned\n", c.GetRebalanceProtocol(), len(ev.Partitions))

		// The application may update the start .Offset of each assigned
		// partition and then call Assign(). It is optional to call Assign
		// in case the application is not modifying any start .Offsets. In
		// that case we don't, the library takes care of it.
		// It is called here despite not modifying any .Offsets for illustrative
		// purposes.
		err := c.Assign(ev.Partitions)
		if err != nil {
			return err
		}

	case kafka.RevokedPartitions:
		// log.Printf("%% %s rebalance: %d partition(s) revoked: %v\n", c.GetRebalanceProtocol(), len(ev.Partitions), ev.Partitions)

		log.Printf("%% %s rebalance: %d partition(s) revoked\n", c.GetRebalanceProtocol(), len(ev.Partitions))

		// Usually, the rebalance callback for `RevokedPartitions` is called
		// just before the partitions are revoked. We can be certain that a
		// partition being revoked is not yet owned by any other consumer.
		// This way, logic like storing any pending offsets or committing
		// offsets can be handled.
		// However, there can be cases where the assignment is lost
		// involuntarily. In this case, the partition might already be owned
		// by another consumer, and operations including committing
		// offsets may not work.
		if c.AssignmentLost() {
			// Our consumer has been kicked out of the group and the
			// entire assignment is thus lost.
			fmt.Fprintln(os.Stderr, "Assignment lost involuntarily, commit may fail")
		}

		// Since enable.auto.commit is unset, we need to commit offsets manually
		// before the partition is revoked.
		commitedOffsets, err := c.Commit()

		if err != nil && err.(kafka.Error).Code() != kafka.ErrNoOffset {
			fmt.Fprintf(os.Stderr, "Failed to commit offsets: %s\n", err)
			return err
		}
		fmt.Printf("%% Commited offsets to Kafka: %v\n", commitedOffsets)

		// Similar to Assign, client automatically calls Unassign() unless the
		// callback has already called that method. Here, we don't call it.

	default:
		fmt.Fprintf(os.Stderr, "Unxpected event type: %v\n", event)
	}

	return nil
}
