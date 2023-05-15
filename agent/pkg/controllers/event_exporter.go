package controllers

import (
	"context"
	"flag"
	"os"

	"github.com/resmoio/kubernetes-event-exporter/pkg/exporter"
	"github.com/resmoio/kubernetes-event-exporter/pkg/kube"
	"github.com/resmoio/kubernetes-event-exporter/pkg/metrics"
	"gopkg.in/yaml.v2"
	"k8s.io/client-go/rest"
)

type eventExporterController struct {
	kubeConfig      *rest.Config
	eventConfigFile string
}

func (e *eventExporterController) Start(ctx context.Context) error {
	b, err := os.ReadFile(e.eventConfigFile)
	if err != nil {
		return err
	}

	var cfg exporter.Config
	err = yaml.Unmarshal(b, &cfg)
	if err != nil {
		return err
	}

	metrics.Init(*flag.String("metrics-address", ":2112", "The address to listen on for HTTP requests."))
	metricsStore := metrics.NewMetricsStore(cfg.MetricsNamePrefix)

	engine := exporter.NewEngine(&cfg, &exporter.ChannelBasedReceiverRegistry{MetricsStore: metricsStore})
	watcher := kube.NewEventWatcher(e.kubeConfig, cfg.Namespace,
		cfg.MaxEventAgeSeconds, metricsStore, engine.OnEvent)
	watcher.Start()
	return nil
}
