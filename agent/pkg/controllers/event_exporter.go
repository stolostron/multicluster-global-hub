package controllers

import (
	"context"
	"errors"
	"flag"
	"os"
	"strings"

	"github.com/go-logr/logr"
	"github.com/resmoio/kubernetes-event-exporter/pkg/exporter"
	"github.com/resmoio/kubernetes-event-exporter/pkg/kube"
	"github.com/resmoio/kubernetes-event-exporter/pkg/metrics"
	"github.com/rs/zerolog"
	"gopkg.in/yaml.v2"
	"k8s.io/client-go/rest"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/event"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/config"
)

type eventExporterController struct {
	kubeConfig      *rest.Config
	eventConfigFile string
	runtimeClient   client.Client
	leafHubName     string
	log             logr.Logger
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

	// issue: https://github.com/resmoio/kubernetes-event-exporter/pull/80
	ValidateEventConfig(&cfg, e.log)
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
		e.addPolicyInfo(ctx, event)
		event.ClusterName = e.leafHubName
		engine.OnEvent(event)
	}
	watcher := event.NewEventWatcher(e.kubeConfig, cfg.Namespace,
		cfg.MaxEventAgeSeconds, metricsStore, onEvent)
	watcher.Start()
	return nil
}

func (e *eventExporterController) addPolicyInfo(ctx context.Context, event *kube.EnhancedEvent) {
	if event.InvolvedObject.Kind != policyv1.Kind {
		return
	}

	rootPolicyNamespacedName, ok := event.InvolvedObject.Labels[constants.PolicyEventRootPolicyNameLabelKey]
	if !ok {
		return
	}
	policyNameSlice := strings.Split(rootPolicyNamespacedName, ".")
	if len(policyNameSlice) != 2 {
		e.log.Error(errors.New("invalid root policy name"),
			"failed to get root policy name by label",
			"name", rootPolicyNamespacedName, "label",
			constants.PolicyEventRootPolicyNameLabelKey)
		return
	}
	policy := policyv1.Policy{}
	if err := e.runtimeClient.Get(ctx, client.ObjectKey{Name: policyNameSlice[1], Namespace: policyNameSlice[0]},
		&policy); err != nil {
		e.log.Error(err, "failed to get rootPolicy", "name", rootPolicyNamespacedName,
			"label", constants.PolicyEventRootPolicyNameLabelKey)
		return
	}
	event.InvolvedObject.Labels[constants.PolicyEventRootPolicyIdLabelKey] = string(policy.GetUID())

	clusterName, ok := event.InvolvedObject.Labels[constants.PolicyEventClusterNameLabelKey]
	if !ok {
		return
	}
	cluster := clusterv1.ManagedCluster{}
	if err := e.runtimeClient.Get(ctx, client.ObjectKey{Name: clusterName}, &cluster); err != nil {
		e.log.Error(err, "failed to get cluster", "cluster", clusterName)
	}
	clusterId := string(cluster.GetUID())
	for _, claim := range cluster.Status.ClusterClaims {
		if claim.Name == "id.k8s.io" {
			clusterId = claim.Value
			break
		}
	}
	event.InvolvedObject.Labels[constants.PolicyEventClusterIdLabelKey] = clusterId
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
