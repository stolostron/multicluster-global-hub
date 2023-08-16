package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var GlobalHubPartitionJobGauge = prometheus.NewGauge(
	prometheus.GaugeOpts{
		Name: "multicluster_global_hub_partition_job_status",
		Help: "The status of the partition/retention job. 0 == success, 1 == failure.",
	},
)

var GlobalHubComplianceJobGauge = prometheus.NewGauge(
	prometheus.GaugeOpts{
		Name: "multicluster_global_hub_compliance_job_status",
		Help: "The status of syncing local compliance history job. 0 == success, 1 == failure.",
	},
)

func init() {
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(
		GlobalHubPartitionJobGauge,
		GlobalHubComplianceJobGauge,
	)
}
