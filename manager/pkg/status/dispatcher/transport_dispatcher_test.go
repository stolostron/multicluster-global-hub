package dispatcher

import (
	"testing"

	"github.com/cloudevents/sdk-go/protocol/kafka_confluent/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
)

func evtWithSourceAndTopic(source, topic string) *cloudevents.Event {
	e := cloudevents.NewEvent()
	e.SetType("io.test")
	e.SetSource(source)
	if topic != "" {
		e.SetExtension(kafka_confluent.KafkaTopicKey, topic)
	}
	return &e
}

// TestSourceMatchesTopic verifies that the dispatcher binds the
// self-asserted CloudEvent Source to the broker-enforced per-hub status
// topic so a managed hub cannot impersonate a peer in status handlers.
func TestSourceMatchesTopic(t *testing.T) {
	statusTopic := "^gh-status.*"

	cases := []struct {
		name   string
		source string
		topic  string
		want   bool
	}{
		{"matching per-hub topic", "hub-a", "gh-status.hub-a", true},
		{"spoofed source on own topic", "hub-b", "gh-status.hub-a", false},
		{"empty source", "", "gh-status.hub-a", false},
		{"non-kafka transport (no topic ext)", "hub-a", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sourceMatchesTopic(evtWithSourceAndTopic(tc.source, tc.topic), statusTopic); got != tc.want {
				t.Fatalf("sourceMatchesTopic(source=%q, topic=%q) = %v, want %v",
					tc.source, tc.topic, got, tc.want)
			}
		})
	}

	// Shared-topic mode (no '*') must not enforce binding.
	if !sourceMatchesTopic(evtWithSourceAndTopic("hub-b", "gh-status"), "gh-status") {
		t.Fatalf("shared-topic mode should not reject events")
	}
}
