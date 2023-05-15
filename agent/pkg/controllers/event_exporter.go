package controllers

import (
	"context"
	"flag"
	"os"

	"github.com/resmoio/kubernetes-event-exporter/pkg/exporter"
	"github.com/resmoio/kubernetes-event-exporter/pkg/kube"
	"github.com/resmoio/kubernetes-event-exporter/pkg/metrics"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/config"
	"gopkg.in/yaml.v2"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
)

type eventExporterController struct {
	kubeConfig      *rest.Config
	eventConfigFile string
}

func (e *eventExporterController) Start(ctx context.Context) error {
	log := ctrl.Log.WithName("event-exporter")
	b, err := os.ReadFile(e.eventConfigFile)
	if err != nil {
		return err
	}

	var cfg exporter.Config
	err = yaml.Unmarshal(b, &cfg)
	if err != nil {
		return err
	}

	kafkaConfig := cfg.Receivers[0].Kafka
	// issue: https://github.com/resmoio/kubernetes-event-exporter/pull/80
	if config.Validate(kafkaConfig.TLS.CertFile) && config.Validate(kafkaConfig.TLS.KeyFile) {
		kafkaConfig.TLS.InsecureSkipVerify = false
	} else {
		kafkaConfig.TLS.InsecureSkipVerify = true
		kafkaConfig.TLS.CertFile = ""
		kafkaConfig.TLS.KeyFile = ""
	}
	log.Info("event exporter config", "config", cfg)

	metrics.Init(*flag.String("metrics-address", ":2112", "The address to listen on for HTTP requests."))
	metricsStore := metrics.NewMetricsStore(cfg.MetricsNamePrefix)

	engine := exporter.NewEngine(&cfg, &exporter.ChannelBasedReceiverRegistry{MetricsStore: metricsStore})
	watcher := kube.NewEventWatcher(e.kubeConfig, cfg.Namespace,
		cfg.MaxEventAgeSeconds, metricsStore, engine.OnEvent)
	watcher.Start()
	return nil
}
