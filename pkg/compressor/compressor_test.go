package compressor_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	"github.com/stolostron/multicluster-global-hub/pkg/compressor"
)

func TestTransportCompressor(t *testing.T) {
	compressor, err := compressor.NewCompressor(compressor.GZip)
	if err != nil {
		t.Fatal(err)
	}

	in := event.BaseEvent{
		EventName: "kube-system.provision.17ad7b80d4e6f6a4",
		Message:   "The cluster (cluster1) is being provisioned now",
		Reason:    "Provisioning",
	}
	t.Log(prettyMessage(in))

	transportBytes, err := json.Marshal(in)
	assert.Nil(t, err)
	kafkaValue, err := compressor.Compress(transportBytes)
	assert.Nil(t, err)

	// decompress the kafka message
	outPayload, err := compressor.Decompress(kafkaValue)
	if err != nil {
		t.Fatal(err)
	}

	out := event.BaseEvent{}
	if err := json.Unmarshal(outPayload, &out); err != nil {
		t.Fatalf("Failed to unmarshal output payload: %v", err)
	}

	t.Log(prettyMessage(out))
}

func prettyMessage(i interface{}) string {
	s, _ := json.MarshalIndent(i, "", "\t")
	return string(s)
}
