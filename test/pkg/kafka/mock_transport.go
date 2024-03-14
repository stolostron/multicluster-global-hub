package kafka

import (
	"context"

	cloudevents "github.com/cloudevents/sdk-go/v2"
)

type MockTransport struct {
	eventChan chan cloudevents.Event
}

func NewMockTransport() *MockTransport {
	return &MockTransport{
		eventChan: make(chan cloudevents.Event),
	}
}

func (p *MockTransport) SendEvent(ctx context.Context, evt cloudevents.Event) error {
	p.eventChan <- evt
	return nil
}

func (p *MockTransport) GetEvent() *cloudevents.Event {
	evt := <-p.eventChan
	return &evt
}
