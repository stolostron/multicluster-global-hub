package metrics

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var GlobalHubCronJobGaugeVec = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "multicluster_global_hub_jobs_status",
		Help: "The status of the job. 0 == success, 1 == failure.",
	},
	[]string{
		"type", // The name of the cronjob.
	},
)

type globalhubMetrics struct {
	log logr.Logger
}

func NewGlobalHubMetrics() *globalhubMetrics {
	return &globalhubMetrics{
		log: ctrl.Log.WithName("global-hub-metrics"),
	}
}

func (m *globalhubMetrics) Start(ctx context.Context) error {
	m.log.V(2).Info("starting global hub metrics")
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(
		GlobalHubCronJobGaugeVec,
	)
	return nil
}
