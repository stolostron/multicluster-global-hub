// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package transport

import (
	"context"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
)

type Transport interface {
	// init the transport with different implementation/protocol: AMQ, strimzi operator or plain deployment
	Initialize(mgh *globalhubv1alpha4.MulticlusterGlobalHub, waitReady bool) error

	// create the transport user(KafkaUser) if not exist for each hub clusters
	CreateUser(name string, waitReady bool) (error, string)

	// create the transport topic(KafkaTopic) if not exist for each hub clusters
	CreateTopic(clusterIdentity string, waitReady bool) (error, string)
}

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
