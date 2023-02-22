// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package transport

import (
	"context"
	"time"

	"github.com/stolostron/multicluster-global-hub/pkg/transport/protocol"
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

type TransportType string

const (
	Kafka TransportType = "kafka"
	Chan  TransportType = "chan"
)

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
	KafkaConfig            *protocol.KafkaConfig
	Extends                map[string]interface{}
}

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
)
