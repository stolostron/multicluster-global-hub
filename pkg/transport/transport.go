// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package transport

import (
	"context"

	cloudevents "github.com/cloudevents/sdk-go/v2"
)

type Producer interface {
	SendEvent(ctx context.Context, evt cloudevents.Event) error
}

type Consumer interface {
	// start the transport to consume message
	Start(ctx context.Context) error
	EventChan() chan *cloudevents.Event
}

// init the transport with different implementation/protocol: secret, strimzi operator or plain deployment
type Transporter interface {
	// CreateUser creates/updates a kafka user for the cluster, the kafka user name is also the CN of cert
	EnsureUser(clusterName string) (string, error)
	// CreateTopic creates/updates a kafka topic
	EnsureTopic(clusterName string) (*ClusterTopic, error)
	// EnsureKafka creates/updates a kafka
	EnsureKafka() error
	// Cleanup will delete the user or topic for the cluster
	Prune(clusterName string) error

	// get the connection credential by clusterName
	GetConnCredential(clusterName string) (*KafkaConnCredential, error)
}
