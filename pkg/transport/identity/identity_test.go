/*
Copyright Contributors to the Open Cluster Management project.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package identity

import (
	"testing"

	kafka_confluent "github.com/cloudevents/sdk-go/protocol/kafka_confluent/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
)

func TestHubFromKafkaUser(t *testing.T) {
	if got := HubFromKafkaUser("hub1-kafka-user"); got != "hub1" {
		t.Fatalf("HubFromKafkaUser() = %q, want hub1", got)
	}
	if got := HubFromKafkaUser("global-hub-kafka-user"); got != "global-hub" {
		t.Fatalf("HubFromKafkaUser() = %q, want global-hub", got)
	}
	if got := HubFromKafkaUser("other-user"); got != "" {
		t.Fatalf("HubFromKafkaUser() = %q, want empty for malformed principal", got)
	}
}

func TestHubFromStatusTopic(t *testing.T) {
	tests := []struct {
		name    string
		topic   string
		pattern string
		want    string
		ok      bool
	}{
		{
			name:    "per-hub topic",
			topic:   "gh-status.hub-east",
			pattern: "gh-status.*",
			want:    "hub-east",
			ok:      true,
		},
		{
			name:    "regex subscription pattern",
			topic:   "gh-status.hub-west",
			pattern: "^gh-status.*",
			want:    "hub-west",
			ok:      true,
		},
		{
			name:    "shared BYO topic",
			topic:   "gh-status",
			pattern: "gh-status",
			want:    "",
			ok:      false,
		},
		{
			name:    "spoof attempt wrong topic prefix",
			topic:   "gh-spec.hub-east",
			pattern: "gh-status.*",
			want:    "",
			ok:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := HubFromStatusTopic(tt.topic, tt.pattern)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("HubFromStatusTopic(%q, %q) = (%q, %v), want (%q, %v)",
					tt.topic, tt.pattern, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestLeafHubForStatusEvent(t *testing.T) {
	t.Run("matching authed hub", func(t *testing.T) {
		evt := cloudevents.NewEvent()
		evt.SetSource("hub-a")
		SetAuthedHub(&evt, "hub-a")

		hub, ok := LeafHubForStatusEvent(&evt)
		if !ok || hub != "hub-a" {
			t.Fatalf("LeafHubForStatusEvent() = (%q, %v), want (hub-a, true)", hub, ok)
		}
	})

	t.Run("spoofed source rejected", func(t *testing.T) {
		evt := cloudevents.NewEvent()
		evt.SetSource("hub-b")
		SetAuthedHub(&evt, "hub-a")

		if hub, ok := LeafHubForStatusEvent(&evt); ok || hub != "" {
			t.Fatalf("LeafHubForStatusEvent() = (%q, %v), want rejection", hub, ok)
		}
	})

	t.Run("nil event rejected", func(t *testing.T) {
		if hub, ok := LeafHubForStatusEvent(nil); ok || hub != "" {
			t.Fatalf("LeafHubForStatusEvent(nil) = (%q, %v), want rejection", hub, ok)
		}
	})

	t.Run("fallback without authed hub", func(t *testing.T) {
		evt := cloudevents.NewEvent()
		evt.SetSource("hub-a")

		hub, ok := LeafHubForStatusEvent(&evt)
		if !ok || hub != "hub-a" {
			t.Fatalf("LeafHubForStatusEvent() = (%q, %v), want (hub-a, true)", hub, ok)
		}
	})
}

func TestEnrichManagerStatusEvent(t *testing.T) {
	t.Run("enriches from kafka topic", func(t *testing.T) {
		evt := cloudevents.NewEvent()
		evt.SetSource("hub-a")
		evt.SetExtension(kafka_confluent.KafkaTopicKey, "gh-status.hub-a")

		EnrichManagerStatusEvent(&evt, "^gh-status.*")

		hub, ok := AuthedHub(&evt)
		if !ok || hub != "hub-a" {
			t.Fatalf("AuthedHub() = (%q, %v), want (hub-a, true)", hub, ok)
		}
	})

	t.Run("nil event is no-op", func(t *testing.T) {
		EnrichManagerStatusEvent(nil, "^gh-status.*")
	})
}
