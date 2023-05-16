// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controllers_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/resmoio/kubernetes-event-exporter/pkg/exporter"
	"github.com/resmoio/kubernetes-event-exporter/pkg/sinks"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/controllers"
)

var _ = Describe("event exporter", func() {
	It("validate event config", func() {
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
		controllers.ValidateEventConfig(cfg, ctrl.Log.WithName("event-exporter-test"))
		Expect(cfg.Receivers[0].Kafka.TLS.InsecureSkipVerify).Should(Equal(true))
		Expect(cfg.Receivers[0].Kafka.TLS.CertFile).Should(Equal(""))
		Expect(cfg.Receivers[0].Kafka.TLS.KeyFile).Should(Equal(""))
	})
})
