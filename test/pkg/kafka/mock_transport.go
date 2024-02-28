package kafka

import (
	"context"

	cloudevents "github.com/cloudevents/sdk-go/v2"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

type MockTransport struct {
	eventChan chan cloudevents.Event
}

func NewMockTransport() *MockTransport {
	return &MockTransport{
		eventChan: make(chan cloudevents.Event),
	}
}

func (p *MockTransport) Send(ctx context.Context, msg *transport.Message) error {
	return nil
}

func (p *MockTransport) SendEvent(ctx context.Context, evt cloudevents.Event) error {
	p.eventChan <- evt
	return nil
}

func (p *MockTransport) GetEvent() *cloudevents.Event {
	evt := <-p.eventChan
	return &evt
}
