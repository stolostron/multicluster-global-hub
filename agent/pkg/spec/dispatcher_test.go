package spec

import (
	"context"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
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

func TestIsEventExpired_InvalidFormat(t *testing.T) {
	evt := cloudevents.NewEvent()
	evt.SetExtension(constants.CloudEventExtensionKeyExpireTime, "not-a-timestamp")
	expired, expireStr := isEventExpired(&evt)
	assert.False(t, expired)
	assert.Empty(t, expireStr)
}

func TestDispatch_DropsMissingSubject(t *testing.T) {
	recorder := runDispatchScenario(t, dispatchScenario{
		events: []*cloudevents.Event{
			func() *cloudevents.Event {
				evt := utils.ToCloudEvent("Policy", constants.CloudEventGlobalHubClusterName, "", nil)
				return &evt
			}(),
		},
	})
	assert.Equal(t, 0, recorder.called)
}

func TestDispatch_DropsSubjectMismatch(t *testing.T) {
	recorder := runDispatchScenario(t, dispatchScenario{
		events: []*cloudevents.Event{
			func() *cloudevents.Event {
				evt := utils.ToCloudEvent("Policy", constants.CloudEventGlobalHubClusterName, "other-hub", nil)
				return &evt
			}(),
		},
	})
	assert.Equal(t, 0, recorder.called)
}

func TestDispatch_AllowsBroadcastSubject(t *testing.T) {
	recorder := runDispatchScenario(t, dispatchScenario{
		events: []*cloudevents.Event{
			func() *cloudevents.Event {
				evt := utils.ToCloudEvent("Policy", constants.CloudEventGlobalHubClusterName, transport.Broadcast, nil)
				return &evt
			}(),
		},
	})
	assert.Equal(t, 1, recorder.called)
}

func TestDispatch_DropsExpiredEvent(t *testing.T) {
	recorder := runDispatchScenario(t, dispatchScenario{
		events: []*cloudevents.Event{
			func() *cloudevents.Event {
				evt := utils.ToCloudEvent("Policy", constants.CloudEventGlobalHubClusterName, "hub2", nil)
				evt.SetExtension(constants.CloudEventExtensionKeyExpireTime,
					time.Now().Add(-1*time.Minute).Format(time.RFC3339))
				return &evt
			}(),
		},
	})
	assert.Equal(t, 0, recorder.called)
}

func TestDispatch_UsesTypeSpecificSyncer(t *testing.T) {
	eventChan := make(chan *cloudevents.Event, 1)
	genericRecorder := &recordingSyncer{}
	typedRecorder := &recordingSyncer{}
	agentConfig := &configs.AgentConfig{LeafHubName: "hub2"}

	dispatcher := &genericDispatcher{
		log:         logger.DefaultZapLogger(),
		consumer:    &mockConsumer{eventChan: eventChan},
		agentConfig: agentConfig,
		syncers: map[string]Syncer{
			constants.GenericSpecMsgKey: genericRecorder,
			"Policy":                    typedRecorder,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go dispatcher.dispatch(ctx)

	evt := utils.ToCloudEvent("Policy", constants.CloudEventGlobalHubClusterName, "hub2", nil)
	eventChan <- &evt

	assert.Eventually(t, func() bool {
		return typedRecorder.called == 1 && genericRecorder.called == 0
	}, time.Second, 10*time.Millisecond)
}

type dispatchScenario struct {
	events []*cloudevents.Event
}

func runDispatchScenario(t *testing.T, scenario dispatchScenario) *recordingSyncer {
	t.Helper()
	eventChan := make(chan *cloudevents.Event, len(scenario.events))
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

	for _, evt := range scenario.events {
		eventChan <- evt
	}

	assert.Eventually(t, func() bool {
		return recorder.called == countTrustedEvents(scenario.events)
	}, time.Second, 10*time.Millisecond)
	return recorder
}

func countTrustedEvents(events []*cloudevents.Event) int {
	count := 0
	for _, evt := range events {
		if evt == nil {
			continue
		}
		subject := evt.Subject()
		if subject == "" || (subject != transport.Broadcast && subject != "hub2") {
			continue
		}
		if expired, _ := isEventExpired(evt); expired {
			continue
		}
		if evt.Source() != constants.CloudEventGlobalHubClusterName {
			continue
		}
		count++
	}
	return count
}

func TestDispatch_AllowsRegisteredMigrationDeploying(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = migrationv1alpha1.AddToScheme(scheme)
	migrationCR := &migrationv1alpha1.ManagedClusterMigration{
		ObjectMeta: metav1.ObjectMeta{Name: "migration-1"},
		Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
			From: "hub1",
			To:   "hub2",
		},
		Status: migrationv1alpha1.ManagedClusterMigrationStatus{
			Phase: migrationv1alpha1.PhaseDeploying,
		},
	}
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(migrationCR).
		WithStatusSubresource(migrationCR).
		Build()

	eventChan := make(chan *cloudevents.Event, 2)
	recorder := &recordingSyncer{}
	agentConfig := &configs.AgentConfig{LeafHubName: "hub2"}

	dispatcher := &genericDispatcher{
		log:         logger.DefaultZapLogger(),
		client:      fakeClient,
		consumer:    &mockConsumer{eventChan: eventChan},
		agentConfig: agentConfig,
		syncers: map[string]Syncer{
			string(enum.ManagedClusterMigrationType): recorder,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go dispatcher.dispatch(ctx)

	deploying := utils.ToCloudEvent(string(enum.ManagedClusterMigrationType), "hub1", "hub2", nil)
	deploying.SetExtension(constants.CloudEventExtensionKeyMigrationStage, migrationv1alpha1.PhaseDeploying)
	eventChan <- &deploying

	spoofed := utils.ToCloudEvent(string(enum.ManagedClusterMigrationType), "spoofed-hub", "hub2", nil)
	spoofed.SetExtension(constants.CloudEventExtensionKeyMigrationStage, migrationv1alpha1.PhaseDeploying)
	eventChan <- &spoofed

	assert.Eventually(t, func() bool {
		return recorder.called == 1
	}, time.Second, 10*time.Millisecond, "expected only registered migration source to reach syncer")
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
