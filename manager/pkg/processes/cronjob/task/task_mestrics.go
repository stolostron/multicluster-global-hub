package task

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

// RegisterMetrics will register metrics with the global prometheus registry
func RegisterMetrics() {
	metrics.Registry.MustRegister(GlobalHubCronJobGaugeVec)
}
