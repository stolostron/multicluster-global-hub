package compressor_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/confluentinc/confluent-kafka-go/kafka"

	statusbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
	"github.com/stolostron/multicluster-global-hub/pkg/compressor"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

func TestTransportCompressor(t *testing.T) {
	compressor, err := compressor.NewCompressor(compressor.GZip)
	if err != nil {
		t.Fatal(err)
	}

	// compress the kafka message with bundle
	clusterPerPolicyBundle := &statusbundle.BaseClustersPerPolicyBundle{
		Objects: []*statusbundle.PolicyGenericComplianceStatus{
			{
				PolicyID:                  "d9347b09-bb46-4e2b-91ea-513e83ab9ea7",
				CompliantClusters:         []string{"cluster1"},
				NonCompliantClusters:      make([]string, 0),
				UnknownComplianceClusters: make([]string, 0),
			},
		},
		LeafHubName: "hub1",
		BundleVersion: &statusbundle.BundleVersion{
			Incarnation: 0,
			Generation:  2,
		},
	}
	transportPayload, err := json.Marshal(clusterPerPolicyBundle)
	if err != nil {
		t.Fatal(err)
	}
	transportMessage := &transport.Message{
		ID:      "hub1.ClustersPerPolicy",
		Key:     "hub1.ClustersPerPolicy",
		MsgType: "StatusBundle",
		Version: "0.2",
		Payload: transportPayload,
	}
	transportBytes, err := json.Marshal(transportMessage)
	if err != nil {
		t.Fatal(err)
	}
	kafkaValue, err := compressor.Compress(transportBytes)
	if err != nil {
		t.Fatal(err)
	}
	topic := "status"
	kafkaMessage := &kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &topic,
			Partition: 0,
			Offset:    6,
			Metadata:  nil,
			Error:     nil,
		},
		Key:           []byte("hub1.ClustersPerPolicy"),
		Value:         kafkaValue,
		Timestamp:     time.Now(),
		TimestampType: 1,
		Opaque:        nil,
		Headers: []kafka.Header{
			{
				Key:   "content-encoding",
				Value: []byte("gzip"),
			},
		},
	}

	t.Log(prettyMessage(kafkaMessage))

	// decompress the kafka message
	decompressMessageValueBytes, err := compressor.Decompress(kafkaMessage.Value)
	if err != nil {
		t.Fatal(err)
	}

	decompressTransportMsg := &transport.Message{}
	json.Unmarshal(decompressMessageValueBytes, decompressTransportMsg)

	decompressBundle := &statusbundle.BaseClustersPerPolicyBundle{}
	json.Unmarshal(decompressTransportMsg.Payload, decompressBundle)

	if decompressBundle.LeafHubName != clusterPerPolicyBundle.LeafHubName {
		t.Fatalf("Expect Bundle leafHubName %s = %s", decompressBundle.LeafHubName, clusterPerPolicyBundle.LeafHubName)
	}

	t.Log(prettyMessage(decompressBundle))
}

func prettyMessage(i interface{}) string {
	s, _ := json.MarshalIndent(i, "", "\t")
	return string(s)
}
