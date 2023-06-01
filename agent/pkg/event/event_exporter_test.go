package event

import (
	"testing"

	"github.com/resmoio/kubernetes-event-exporter/pkg/exporter"
	"github.com/resmoio/kubernetes-event-exporter/pkg/sinks"
	"github.com/stretchr/testify/assert"
	ctrl "sigs.k8s.io/controller-runtime"
)

func TestEventExporter_ValidateEventConfig(t *testing.T) {
	cfg := &exporter.Config{
		Route:     exporter.Route{},
		Receivers: make([]sinks.ReceiverConfig, 1),
	}
	cfg.Receivers[0].Kafka = &sinks.KafkaConfig{
		Topic: "test-topic",
		Brokers: []string{
			"localhost:9092",
		},
		TLS: struct {
			Enable             bool   `yaml:"enable"`
			CaFile             string `yaml:"caFile"`
			CertFile           string `yaml:"certFile"`
			KeyFile            string `yaml:"keyFile"`
			InsecureSkipVerify bool   `yaml:"insecureSkipVerify"`
		}{
			Enable:             true,
			CaFile:             "caFile",
			CertFile:           "certFile",
			KeyFile:            "keyFile",
			InsecureSkipVerify: false,
		},
	}
	validateEventConfig(cfg, ctrl.Log.WithName("event-exporter-test"))
	assert.Equal(t, true, cfg.Receivers[0].Kafka.TLS.InsecureSkipVerify)
	assert.Equal(t, "", cfg.Receivers[0].Kafka.TLS.CertFile)
	assert.Equal(t, "", cfg.Receivers[0].Kafka.TLS.KeyFile)
}
