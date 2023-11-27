// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package transport

import "context"

// init the transport with different implementation/protocol: AMQ, strimzi operator or plain deployment
type Transporter interface {
	// create the transport user(KafkaUser) if not exist for each hub clusters
	CreateUser(name string) error
	DeleteUser(name string) error

	// create the transport topic(KafkaTopic) if not exist for each hub clusters
	CreateTopic(names []string) error
	DeleteTopic(name []string) error

	// get the connection credential by user
	GetConnCredential(username string) (*ConnCredential, error)
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
