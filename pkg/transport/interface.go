// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package transport

import (
	"context"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/project-kessel/inventory-client-go/v1beta1"
)

// TransportClient provides the consumer, producer and inventory client for the global hub manager, agent, ...
type TransportClient interface {
	GetProducer() Producer
	GetConsumer() Consumer
	GetRequester() Requester
}

type Requester interface {
	RefreshClient(ctx context.Context, restConfig *RestfulConfig) error
	// current we only support the inventory client, we can generilize return client instance if it required
	GetHttpClient() *v1beta1.InventoryHttpClient
}

type Producer interface {
	SendEvent(ctx context.Context, evt cloudevents.Event) error
	Reconnect(config *TransportInternalConfig, topic string) error
}

type Consumer interface {
	// start the transport to consume message
	Start(ctx context.Context) error
	EventChan() chan *cloudevents.Event
	Reconnect(ctx context.Context, config *TransportInternalConfig, topics []string) error
}

type TransportCertificate interface {
	GetCACert() string
	SetCACert(string)
	GetClientCert() string
	SetClientCert(string)
	GetClientKey() string
	SetClientKey(string)
	GetCASecretName() string
	GetClientSecretName() string
}
