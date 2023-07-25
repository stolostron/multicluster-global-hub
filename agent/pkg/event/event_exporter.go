package event

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
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/event/enhancers"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

type EventEnhancer interface {
	// Enhance is used to modify the event before it is omitted by the exporter
	// return true if the event should be exported
	Enhance(ctx context.Context, event *kube.EnhancedEvent) bool
}

type EventExporter interface {
	// Start starts the event exporter and make sure it can be added to the manager
	Start(ctx context.Context) error
	// RegisterEnhancer registers an event enhancer
	RegisterEnhancer(enhancer EventEnhancer)
}

type eventExporter struct {
	kubeConfig      *rest.Config
	eventConfigFile string
	runtimeClient   client.Client
	leafHubName     string
	log             logr.Logger
	enhancers       map[string]EventEnhancer
}

func AddEventExporter(mgr ctrl.Manager, eventConfigFile string, leafHubName string) error {
	eventExporter := &eventExporter{
		kubeConfig:      mgr.GetConfig(),
		runtimeClient:   mgr.GetClient(),
		leafHubName:     leafHubName,
		eventConfigFile: eventConfigFile,
		log:             ctrl.Log.WithName("event-exporter"),
		enhancers:       make(map[string]EventEnhancer),
	}

	// add policy event enhancer
	eventExporter.RegisterEnhancer(policyv1.Kind,
		enhancers.NewPolicyEventEnhancer(eventExporter.runtimeClient))

	return mgr.Add(eventExporter)
}

func (e *eventExporter) RegisterEnhancer(eventKey string, enhancer EventEnhancer) {
	e.enhancers[eventKey] = enhancer
}

func (e *eventExporter) Start(ctx context.Context) error {
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
	validateEventConfig(&cfg, e.log)
	e.log.Info("starting event exporter", "config", cfg)

	metrics.Init(*flag.String("metrics-address", ":2112",
		"The address to listen on for HTTP requests."))
	metricsStore := metrics.NewMetricsStore(cfg.MetricsNamePrefix)

	if cfg.LogLevel != "" {
		level, err := zerolog.ParseLevel(cfg.LogLevel)
		if err != nil {
			e.log.Error(err, "invalid log level")
		}
		zerolog.SetGlobalLevel(level)
	}

	engine := exporter.NewEngine(&cfg, &exporter.ChannelBasedReceiverRegistry{MetricsStore: metricsStore})
	onEvent := func(event *kube.EnhancedEvent) {
		// note that per code this value is not set anywhere on the kubernetes side
		enhancer := e.enhancers[event.InvolvedObject.Kind]
		exported := false
		if enhancer != nil {
			exported = enhancer.Enhance(ctx, event)
		}
		event.ClusterName = e.leafHubName
		if exported {
			engine.OnEvent(event)
		}
	}
	watcher := NewEventWatcher(e.kubeConfig, cfg.Namespace,
		cfg.MaxEventAgeSeconds, metricsStore, onEvent)
	watcher.Start()
	return nil
}

func validateEventConfig(eventConfig *exporter.Config, log logr.Logger) {
	if len(eventConfig.Receivers) == 0 || eventConfig.Receivers[0].Kafka == nil {
		log.Info("No kafka config found, skipping validate kafka sinker for event exporter")
		return
	}

	kafkaConfig := eventConfig.Receivers[0].Kafka
	if utils.Validate(kafkaConfig.TLS.CertFile) && utils.Validate(kafkaConfig.TLS.KeyFile) {
		kafkaConfig.TLS.InsecureSkipVerify = false
	} else {
		kafkaConfig.TLS.InsecureSkipVerify = true
		kafkaConfig.TLS.CertFile = ""
		kafkaConfig.TLS.KeyFile = ""
	}
}
