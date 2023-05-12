// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package transport

import (
	"context"
	"time"
)

const (
	// DestinationHub is the key used for destination-hub name header.
	DestinationHub = "destination-hub"
	// CompressionType is the key used for compression type header.
	CompressionType = "content-encoding"
	// Size is the key used for total bundle size header.
	Size = "size"
	// Offset is the key used for message fragment offset header.
	Offset = "offset"
	// FragmentationTimestamp is the key used for bundle fragmentation time header.
	FragmentationTimestamp = "fragmentation-timestamp"
	// Broadcast can be used as destination when a bundle should be broadcasted.
	Broadcast = ""

	// Kafka transportType and transportFormat values
	Kafka              TransportType   = "kafka"
	Chan               TransportType   = "chan"
	KafkaMessageFormat TransportFormat = "message"
	CloudEventsFormat  TransportFormat = "cloudEvents"
)

type (
	TransportType   string
	TransportFormat string
)

type Producer interface {
	Send(ctx context.Context, msg *Message) error
}

type Consumer interface {
	// start the transport to consume message
	Start(ctx context.Context) error
	// provide a blocking message to get the message
	MessageChan() chan *Message
}

// Message abstracts a message object to be used by different transport components.
type Message struct {
	Destination string `json:"destination"`
	Key         string `json:"key"`
	ID          string `json:"id"`
	MsgType     string `json:"msgType"`
	Version     string `json:"version"`
	Payload     []byte `json:"payload"`
}

type TransportConfig struct {
	TransportType          string
	TransportFormat        string
	MessageCompressionType string
	CommitterInterval      time.Duration
	KafkaConfig            *KafkaConfig
	Extends                map[string]interface{}
}

// Kafka Config
type KafkaConfig struct {
	BootstrapServer string
	CaCertPath      string
	ClientCertPath  string
	ClientKeyPath   string
	EnableTLS       bool
	ProducerConfig  *KafkaProducerConfig
	ConsumerConfig  *KafkaConsumerConfig
}

type KafkaProducerConfig struct {
	ProducerID         string
	ProducerTopic      string
	MessageSizeLimitKB int
}

type KafkaConsumerConfig struct {
	ConsumerID    string
	ConsumerTopic string
}
