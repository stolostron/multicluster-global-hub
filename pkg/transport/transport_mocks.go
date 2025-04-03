// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package transport

import (
	"context"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/event"
)

// ProducerMock is a mock for the producer interface, intended for use in unit tests.
type ProducerMock struct {
	SendEventFunc func(ctx context.Context, evt cloudevents.Event) error
	ReconnectFunc func(config *TransportInternalConfig) error
}

// Make sure we implement the producer interface:
var _ Producer = (*ProducerMock)(nil)

func (m *ProducerMock) SendEvent(ctx context.Context, evt event.Event) error {
	if m == nil {
		panic("nil mock")
	}
	if m.ReconnectFunc == nil {
		panic("nil SendEventFunc")
	}
	return m.SendEventFunc(ctx, evt)
}

func (m *ProducerMock) Reconnect(config *TransportInternalConfig, topic string) error {
	if m == nil {
		panic("nil mock")
	}
	if m.ReconnectFunc == nil {
		panic("nil ReconnectFunc")
	}
	return m.ReconnectFunc(config)
}
