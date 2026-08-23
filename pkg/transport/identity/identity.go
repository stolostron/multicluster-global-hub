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

// Package identity provides helpers to derive and validate hub identity from Kafka transport metadata.
package identity

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"

	kafka_confluent "github.com/cloudevents/sdk-go/protocol/kafka_confluent/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	cetypes "github.com/cloudevents/sdk-go/v2/types"
)

const (
	// ExtensionAuthedHub is the CloudEvent extension carrying the transport-derived hub name.
	ExtensionAuthedHub = "authedhub"

	kafkaUserSuffix = "-kafka-user"
)

// HubFromKafkaUser maps a KafkaUser / mTLS CN (e.g. "hub1-kafka-user") to a hub cluster name.
func HubFromKafkaUser(kafkaUser string) string {
	if !strings.HasSuffix(kafkaUser, kafkaUserSuffix) {
		return ""
	}

	hub := strings.TrimSuffix(kafkaUser, kafkaUserSuffix)
	if hub == "" {
		return ""
	}

	return hub
}

// HubFromStatusTopic extracts the managed-hub name from a Kafka status topic.
// statusTopicPattern is the configured template, e.g. "gh-status.*" or "^gh-status.*".
// Returns ("", false) for shared topics without a per-hub suffix (e.g. BYO "gh-status").
func HubFromStatusTopic(topic, statusTopicPattern string) (string, bool) {
	pattern := strings.TrimPrefix(statusTopicPattern, "^")
	if !strings.Contains(pattern, "*") {
		return "", false
	}

	prefix := strings.ReplaceAll(pattern, "*", "")
	if prefix == "" || !strings.HasPrefix(topic, prefix) {
		return "", false
	}

	hub := strings.TrimPrefix(topic, prefix)
	if hub == "" {
		return "", false
	}

	return hub, true
}

// KafkaTopicFromEvent reads the Kafka topic name from CloudEvent Kafka extensions.
func KafkaTopicFromEvent(evt *cloudevents.Event) (string, bool) {
	if evt == nil {
		return "", false
	}

	topic, err := cetypes.ToString(evt.Extensions()[kafka_confluent.KafkaTopicKey])
	if err != nil || topic == "" {
		return "", false
	}

	return topic, true
}

// SetAuthedHub attaches the transport-derived hub identity to the event.
func SetAuthedHub(evt *cloudevents.Event, hub string) {
	if evt == nil || hub == "" {
		return
	}
	evt.SetExtension(ExtensionAuthedHub, hub)
}

// AuthedHub returns the transport-derived hub from the event extension.
func AuthedHub(evt *cloudevents.Event) (string, bool) {
	if evt == nil {
		return "", false
	}

	hub, err := cetypes.ToString(evt.Extensions()[ExtensionAuthedHub])
	if err != nil || hub == "" {
		return "", false
	}

	return hub, true
}

// LeafHubForStatusEvent returns the trusted leaf-hub name for a manager status event.
// When authedhub is present, evt.Source() must match it. When absent (e.g. chan transport
// in tests), source is used as a fallback.
func LeafHubForStatusEvent(evt *cloudevents.Event) (string, bool) {
	if evt == nil {
		return "", false
	}

	authedHub, hasAuthed := AuthedHub(evt)
	if hasAuthed {
		if evt.Source() != authedHub {
			return "", false
		}
		return authedHub, true
	}

	if evt.Source() == "" {
		return "", false
	}

	return evt.Source(), true
}

// HubFromClientCertCN maps an mTLS certificate CommonName to a hub name when
// the CN follows the KafkaUser convention "{hub}-kafka-user".
func HubFromClientCertCN(commonName string) string {
	return HubFromKafkaUser(commonName)
}

// ValidateBYOClientCert rejects a BYO Kafka client certificate whose CN is a
// KafkaUser for a different hub. Custom CNs (for example global-hub-byo-user)
// are allowed so existing shared-secret BYO installs keep working until they
// switch to per-hub secrets.
func ValidateBYOClientCert(leafHubName, clientCertPEM string) error {
	if leafHubName == "" || clientCertPEM == "" {
		return nil
	}

	cn, err := commonNameFromPEMX509(clientCertPEM)
	if err != nil {
		return fmt.Errorf("failed to parse BYO Kafka client certificate: %w", err)
	}
	if cn == "" {
		return nil
	}

	certHub := HubFromClientCertCN(cn)
	if certHub == "" || certHub == leafHubName {
		return nil
	}

	return fmt.Errorf("BYO Kafka client certificate does not belong to this hub")
}

func commonNameFromPEMX509(certPEM string) (string, error) {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		return "", fmt.Errorf("no PEM certificate found")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", err
	}
	return cert.Subject.CommonName, nil
}

// EnrichManagerStatusEvent sets authedhub from the Kafka status topic when the pattern
// contains a per-hub wildcard segment.
func EnrichManagerStatusEvent(evt *cloudevents.Event, statusTopicPattern string) {
	if evt == nil {
		return
	}

	topic, ok := KafkaTopicFromEvent(evt)
	if !ok {
		return
	}

	hub, ok := HubFromStatusTopic(topic, statusTopicPattern)
	if !ok {
		return
	}

	SetAuthedHub(evt, hub)
}
