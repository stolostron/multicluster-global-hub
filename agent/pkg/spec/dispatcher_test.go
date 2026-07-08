package spec

import (
	"context"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/stretchr/testify/assert"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func TestIsEventExpired(t *testing.T) {
	tests := []struct {
		name    string
		evt     func() *cloudevents.Event
		expired bool
	}{
		{
			name: "expired event",
			evt: func() *cloudevents.Event {
				e := cloudevents.NewEvent()
				e.SetExtension(constants.CloudEventExtensionKeyExpireTime,
					time.Now().Add(-1*time.Minute).Format(time.RFC3339))
				return &e
			},
			expired: true,
		},
		{
			name: "valid event",
			evt: func() *cloudevents.Event {
				e := cloudevents.NewEvent()
				e.SetExtension(constants.CloudEventExtensionKeyExpireTime,
					time.Now().Add(10*time.Minute).Format(time.RFC3339))
				return &e
			},
			expired: false,
		},
		{
			name: "no expiry extension",
			evt: func() *cloudevents.Event {
				e := cloudevents.NewEvent()
				return &e
			},
			expired: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expired, _ := isEventExpired(tt.evt())
			assert.Equal(t, tt.expired, expired)
		})
	}
}

type mockConsumer struct {
	eventChan chan *cloudevents.Event
}

func (m *mockConsumer) Start(context.Context) error { return nil }

func (m *mockConsumer) EventChan() chan *cloudevents.Event { return m.eventChan }

func (m *mockConsumer) ConfigChan() chan *transport.TransportInternalConfig {
	return make(chan *transport.TransportInternalConfig)
}

type recordingSyncer struct {
	called int
}

func (s *recordingSyncer) Sync(context.Context, *cloudevents.Event) error {
	s.called++
	return nil
}

func TestDispatch_SourceValidation(t *testing.T) {
	eventChan := make(chan *cloudevents.Event, 2)
	recorder := &recordingSyncer{}
	agentConfig := &configs.AgentConfig{LeafHubName: "hub2"}

	dispatcher := &genericDispatcher{
		log:         logger.DefaultZapLogger(),
		consumer:    &mockConsumer{eventChan: eventChan},
		agentConfig: agentConfig,
		syncers: map[string]Syncer{
			constants.GenericSpecMsgKey: recorder,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go dispatcher.dispatch(ctx)

	trusted := utils.ToCloudEvent("Policy", constants.CloudEventGlobalHubClusterName, "hub2", nil)
	eventChan <- &trusted

	spoofed := utils.ToCloudEvent("Policy", "spoofed-hub", "hub2", nil)
	eventChan <- &spoofed

	assert.Eventually(t, func() bool {
		return recorder.called == 1
	}, time.Second, 10*time.Millisecond, "expected only trusted manager event to reach syncer")
}
