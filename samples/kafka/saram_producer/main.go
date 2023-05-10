package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Shopify/sarama"
	"github.com/stolostron/multicluster-global-hub/samples/config"
)

const (
	TopicDefault        = "status"
	DelayDefault        = 1000
	MessageDefault      = "Hello from Go Kafka Sarama"
	MessageCountDefault = 10
	ProducerAcksDefault = int16(1)
)

func main() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGKILL)

	bootstrapSever, saramaConfig, err := config.GetSaramaConfig()
	if err != nil {
		fmt.Printf("Error getting producer config: %v\n", err)
		os.Exit(1)
	}
	saramaConfig.Producer.RequiredAcks = sarama.RequiredAcks(ProducerAcksDefault)
	saramaConfig.Producer.Return.Successes = true
	saramaConfig.Producer.MaxMessageBytes = 1024 * 1000 // 1024KB

	producer, err := sarama.NewSyncProducer([]string{string(bootstrapSever)}, saramaConfig)
	if err != nil {
		log.Printf("Error creating the Sarama sync producer: %v", err)
		os.Exit(1)
	}

	end := make(chan int, 1)
	go func() {
		for i := 0; i < MessageCountDefault; i++ {
			value := fmt.Sprintf("%s-%d", "hello", int64(i))
			msg := &sarama.ProducerMessage{
				Topic: TopicDefault,
				Value: sarama.StringEncoder(value),
				Key:   sarama.StringEncoder("key"),
			}
			partition, offset, err := producer.SendMessage(msg)
			if err != nil {
				log.Printf("Erros sending message: %v\n", err)
			} else {
				log.Printf("Message sent: partition=%d, offset=%d, msg=%s\n", partition, offset, msg.Value)
			}

			// sleep before next message or avoid sleeping
			// and signal the end on the last message
			if i == MessageCountDefault-1 {
				end <- 1
			} else {
				time.Sleep(time.Duration(DelayDefault) * time.Millisecond)
			}
		}
	}()

	// waiting for the end of all messages sent or an OS signal
	select {
	case <-end:
		log.Printf("Finished to send %d messages\n", MessageCountDefault)
	case sig := <-signals:
		log.Printf("Got signal: %v\n", sig)
	}

	err = producer.Close()
	if err != nil {
		log.Printf("Error closing the Sarama sync producer: %v", err)
		os.Exit(1)
	}
	log.Printf("Producer closed")
}
