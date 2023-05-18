package controllers

import (
	"context"
	"flag"
	"os"

	"github.com/go-logr/logr"
	"github.com/resmoio/kubernetes-event-exporter/pkg/exporter"
	"github.com/resmoio/kubernetes-event-exporter/pkg/kube"
	"github.com/resmoio/kubernetes-event-exporter/pkg/metrics"
	"github.com/rs/zerolog"
	"gopkg.in/yaml.v2"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/pkg/transport/config"
)

type eventExporterController struct {
	kubeConfig      *rest.Config
	leafHubName     string
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

	// issue: https://github.com/resmoio/kubernetes-event-exporter/pull/80
	ValidateEventConfig(&cfg, log)
	log.Info("starting event exporter", "config", cfg)

	metrics.Init(*flag.String("metrics-address", ":2112",
		"The address to listen on for HTTP requests."))
	metricsStore := metrics.NewMetricsStore(cfg.MetricsNamePrefix)

	if cfg.LogLevel != "" {
		level, err := zerolog.ParseLevel(cfg.LogLevel)
		if err != nil {
			log.Error(err, "invalid log level")
		}
		zerolog.SetGlobalLevel(level)
	}

	engine := exporter.NewEngine(&cfg, &exporter.ChannelBasedReceiverRegistry{MetricsStore: metricsStore})
	onEvent := func(event *kube.EnhancedEvent) {
		// note that per code this value is not set anywhere on the kubernetes side
		event.ClusterName = e.leafHubName
		engine.OnEvent(event)
	}
	watcher := kube.NewEventWatcher(e.kubeConfig, cfg.Namespace,
		cfg.MaxEventAgeSeconds, metricsStore, onEvent)
	watcher.Start()
	return nil
}

func ValidateEventConfig(eventConfig *exporter.Config, log logr.Logger) {
	if len(eventConfig.Receivers) == 0 || eventConfig.Receivers[0].Kafka == nil {
		log.Info("No kafka config found, skipping validate kafka sinker for event exporter")
		return
	}

	kafkaConfig := eventConfig.Receivers[0].Kafka
	if config.Validate(kafkaConfig.TLS.CertFile) && config.Validate(kafkaConfig.TLS.KeyFile) {
		kafkaConfig.TLS.InsecureSkipVerify = false
	} else {
		kafkaConfig.TLS.InsecureSkipVerify = true
		kafkaConfig.TLS.CertFile = ""
		kafkaConfig.TLS.KeyFile = ""
	}
}
