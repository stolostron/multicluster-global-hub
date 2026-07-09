package utils

import (
	"crypto/tls"

	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

// NewSecureMetricsServerOptions builds controller-runtime metrics server options
// with SecureServing enabled so TLSOpts are applied.
//
// Without SecureServing, controller-runtime serves metrics over plain HTTP and
// TLSOpts are ignored (ACM-30175 follow-up to #2487).
func NewSecureMetricsServerOptions(
	bindAddress string,
	tlsOpts ...func(*tls.Config),
) metricsserver.Options {
	return metricsserver.Options{
		BindAddress:   bindAddress,
		SecureServing: true,
		TLSOpts:       tlsOpts,
	}
}
