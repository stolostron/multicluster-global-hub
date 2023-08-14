package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var GlobalHubJobGauge = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "multicluster_global_hub_jobs_status",
		Help: "The status of the job. 0 == success, 1 == failure.",
	},
	[]string{
		"name", // The name of the job.
	},
)

func init() {
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(
		GlobalHubJobGauge,
	)
}
