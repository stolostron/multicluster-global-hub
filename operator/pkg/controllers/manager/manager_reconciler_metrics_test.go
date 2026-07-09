/*
Copyright Contributors to the Open Cluster Management project.
*/

package manager

import (
	"testing"

	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagerMetricsEndpoint(t *testing.T) {
	ep := managerMetricsEndpoint("30s")
	assert.Equal(t, "metrics", ep.Port)
	assert.Equal(t, "/metrics", ep.Path)
	assert.Equal(t, "https", ep.Scheme)
	assert.Equal(t, promv1.Duration("30s"), ep.Interval)
	require.NotNil(t, ep.TLSConfig)
	require.NotNil(t, ep.TLSConfig.InsecureSkipVerify)
	// Interim: controller-runtime self-signed metrics cert. Remove skip-verify
	// when a serving-cert secret is wired (tracked under ACM-30175 follow-up).
	assert.True(t, *ep.TLSConfig.InsecureSkipVerify)
}

func TestManagerMetricsEndpointCustomInterval(t *testing.T) {
	ep := managerMetricsEndpoint("1m")
	assert.Equal(t, promv1.Duration("1m"), ep.Interval)
	assert.Equal(t, "https", ep.Scheme)
	require.NotNil(t, ep.TLSConfig.InsecureSkipVerify)
	// Same interim skip-verify as TestManagerMetricsEndpoint (ACM-30175).
	assert.True(t, *ep.TLSConfig.InsecureSkipVerify)
}
