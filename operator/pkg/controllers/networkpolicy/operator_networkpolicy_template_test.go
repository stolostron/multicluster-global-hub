package networkpolicy

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	nputils "github.com/stolostron/multicluster-global-hub/operator/pkg/networkpolicy"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
)

func TestRenderOperatorNetworkPolicy_BYOWithCIDRs(t *testing.T) {
	npRenderer := renderer.NewHoHRenderer(fs)
	values := nputils.BaselineManifestValues{
		Namespace:             "multicluster-global-hub",
		PostgresName:          "multicluster-global-hub-postgresql",
		BYOPostgres:           true,
		ExternalPostgresCIDRs: []string{"172.30.5.56/32", "10.131.1.85/32"},
		BYOKafka:              true,
		ExternalKafkaCIDRs:    []string{"172.30.85.220/32"},
		APIServerCIDRs:        []string{"10.96.0.1/32"},
	}
	objects, err := npRenderer.Render("manifests", "", func(profile string) (interface{}, error) {
		return values, nil
	})
	require.NoError(t, err, "render operator networkpolicy manifests")

	yamlText := renderedOperatorNetworkPolicyYAML(t, objects)
	assert.Contains(t, yamlText, "cidr: 172.30.5.56/32", "expected BYO postgres ipBlock")
	assert.Contains(t, yamlText, "cidr: 172.30.85.220/32", "expected BYO kafka ipBlock")
	assert.NotContains(t, yamlText, "cnpg.io/cluster", "CNPG fallback should not render when postgres CIDRs resolve")
	assert.NotContains(t, yamlText, "namespaceSelector: {}", "Strimzi fallback should not render when kafka CIDRs resolve")
}

func TestRenderOperatorNetworkPolicy_BYOFallbackSelectors(t *testing.T) {
	npRenderer := renderer.NewHoHRenderer(fs)
	values := nputils.BaselineManifestValues{
		Namespace:    "multicluster-global-hub",
		PostgresName: "multicluster-global-hub-postgresql",
		BYOPostgres:  true,
		BYOKafka:     true,
	}
	objects, err := npRenderer.Render("manifests", "", func(profile string) (interface{}, error) {
		return values, nil
	})
	require.NoError(t, err, "render operator networkpolicy manifests")

	yamlText := renderedOperatorNetworkPolicyYAML(t, objects)
	assert.Contains(t, yamlText, "cnpg.io/cluster", "expected CNPG pod selector fallback")
	assert.Contains(t, yamlText, "postgres-operator.crunchydata.com/cluster", "expected Crunchy pod selector fallback")
	assert.Contains(t, yamlText, "strimzi.io/cluster: kafka", "expected Strimzi pod selector fallback")
	assert.NotContains(t, yamlText, "ipBlock:", "ipBlock rules should not render without resolved CIDRs")
}

func TestRenderOperatorNetworkPolicy_BuiltInOnly(t *testing.T) {
	npRenderer := renderer.NewHoHRenderer(fs)
	values := nputils.BaselineManifestValues{
		Namespace:    "multicluster-global-hub",
		PostgresName: "multicluster-global-hub-postgresql",
	}
	objects, err := npRenderer.Render("manifests", "", func(profile string) (interface{}, error) {
		return values, nil
	})
	require.NoError(t, err, "render operator networkpolicy manifests")

	yamlText := renderedOperatorNetworkPolicyYAML(t, objects)
	assert.Contains(t, yamlText, "name: multicluster-global-hub-postgresql", "expected in-hub postgres selector")
	assert.NotContains(t, yamlText, "cnpg.io/cluster", "BYO postgres rules should not render")
	assert.NotContains(t, yamlText, "namespaceSelector: {}", "BYO kafka rules should not render")
}

func renderedOperatorNetworkPolicyYAML(t *testing.T, objects []*unstructured.Unstructured) string {
	t.Helper()
	var b strings.Builder
	for _, obj := range objects {
		if obj.GetKind() != "NetworkPolicy" || obj.GetName() != NetworkPolicyOperator {
			continue
		}
		data, err := yaml.Marshal(obj.Object)
		require.NoError(t, err, "marshal rendered networkpolicy")
		b.Write(data)
	}
	return b.String()
}
