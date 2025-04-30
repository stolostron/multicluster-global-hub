// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package producer

import (
	"testing"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/stretchr/testify/require"

	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

func TestGenericProducer(t *testing.T) {
	p := &GenericProducer{}
	tranConfig := &transport.TransportInternalConfig{
		TransportType: string(transport.Rest),
		KafkaCredential: &transport.KafkaConfig{
			SpecTopic:      "gh-spec",
			StatusTopic:    "gh-status",
			MigrationTopic: "gh-migration",
		},
	}
	err := p.initClient(tranConfig, tranConfig.KafkaCredential.StatusTopic)
	require.Equal(t, "transport-type - rest is not a valid option", err.Error())
}

func Test_handleProducerEvents(t *testing.T) {
	tests := []struct {
		name                      string
		event                     kafka.Event
		transportFailureThreshold int
	}{
		{
			name:                      "kafka error",
			transportFailureThreshold: 10,
			event:                     kafka.NewError(kafka.ErrFail, "errStr", false),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logger.DefaultZapLogger()
			eventChan := make(chan kafka.Event)
			go handleProducerEvents(log, eventChan, tt.transportFailureThreshold, nil)
			eventChan <- tt.event
		})
	}
}
