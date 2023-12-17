// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package transport

import (
	"context"
)

// init the transport with different implementation/protocol: secret, strimzi operator or plain deployment
type Transporter interface {
	// define the user for each hub clusters, user lifecycle is managed by the transporter
	GenerateUserName(clusterIdentity string) string
	CreateUser(name string) error
	DeleteUser(name string) error

	// define the topic for each hub clusters(or shared topic), topic lifecycle is managed by the transporter
	GenerateClusterTopic(clusterIdentity string) *ClusterTopic
	CreateTopic(topic *ClusterTopic) error
	DeleteTopic(topic *ClusterTopic) error

	// get the connection credential by user
	GetConnCredential(userName string) (*ConnCredential, error)
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
