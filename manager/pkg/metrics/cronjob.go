package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
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

func init() {
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(
		GlobalHubCronJobGaugeVec,
	)
}
