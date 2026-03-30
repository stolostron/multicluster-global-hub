package utils

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func TestHubHAResourceFilter_ShouldSyncResource(t *testing.T) {
	filter := NewHubHAResourceFilter()

	tests := []struct {
		name           string
		gvk            schema.GroupVersionKind
		labels         map[string]string
		expectedResult bool
	}{
		{
			name: "Policy should be synced",
			gvk: schema.GroupVersionKind{
				Group:   "policy.open-cluster-management.io",
				Version: "v1",
				Kind:    "Policy",
			},
			labels:         nil,
			expectedResult: true,
		},
		{
			name: "Placement should be synced",
			gvk: schema.GroupVersionKind{
				Group:   "cluster.open-cluster-management.io",
				Version: "v1beta1",
				Kind:    "Placement",
			},
			labels:         nil,
			expectedResult: true,
		},
		{
			name: "Resource with velero exclude label should not be synced",
			gvk: schema.GroupVersionKind{
				Group:   "policy.open-cluster-management.io",
				Version: "v1",
				Kind:    "Policy",
			},
			labels: map[string]string{
				"velero.io/exclude-from-backup": "true",
			},
			expectedResult: false,
		},
		{
			name: "ArgoCDApplication should be synced",
			gvk: schema.GroupVersionKind{
				Group:   "argoproj.io",
				Version: "v1alpha1",
				Kind:    "Application",
			},
			labels:         nil,
			expectedResult: true,
		},
		{
			name: "ClusterDeployment should be synced",
			gvk: schema.GroupVersionKind{
				Group:   "hive.openshift.io",
				Version: "v1",
				Kind:    "ClusterDeployment",
			},
			labels:         nil,
			expectedResult: true,
		},
		{
			name: "Secret without required label should not be synced",
			gvk: schema.GroupVersionKind{
				Group:   "",
				Version: "v1",
				Kind:    "Secret",
			},
			labels:         nil,
			expectedResult: false,
		},
		{
			name: "Secret with cluster type label should be synced",
			gvk: schema.GroupVersionKind{
				Group:   "",
				Version: "v1",
				Kind:    "Secret",
			},
			labels: map[string]string{
				"cluster.open-cluster-management.io/type": "managed",
			},
			expectedResult: true,
		},
		{
			name: "ConfigMap with hive secret type should be synced",
			gvk: schema.GroupVersionKind{
				Group:   "",
				Version: "v1",
				Kind:    "ConfigMap",
			},
			labels: map[string]string{
				"hive.openshift.io/secret-type": "kubeconfig",
			},
			expectedResult: true,
		},
		{
			name: "InfraEnv (ZTP) should be synced",
			gvk: schema.GroupVersionKind{
				Group:   "agent-install.openshift.io",
				Version: "v1beta1",
				Kind:    "InfraEnv",
			},
			labels:         nil,
			expectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &unstructured.Unstructured{}
			obj.SetGroupVersionKind(tt.gvk)
			if tt.labels != nil {
				obj.SetLabels(tt.labels)
			}

			result := filter.ShouldSyncResource(obj, tt.gvk)
			if result != tt.expectedResult {
				t.Errorf("ShouldSyncResource() = %v, want %v", result, tt.expectedResult)
			}
		})
	}
}

func TestIsActiveHub(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		expected bool
	}{
		{
			name: "Managed cluster with active hub label",
			labels: map[string]string{
				constants.GHHubRoleLabelKey: constants.GHHubRoleActive,
			},
			expected: true,
		},
		{
			name: "Managed cluster with standby hub label",
			labels: map[string]string{
				constants.GHHubRoleLabelKey: constants.GHHubRoleStandby,
			},
			expected: false,
		},
		{
			name:     "Managed cluster without hub role label",
			labels:   map[string]string{},
			expected: false,
		},
		{
			name:     "Managed cluster with nil labels",
			labels:   nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &unstructured.Unstructured{}
			obj.SetLabels(tt.labels)

			result := IsActiveHub(obj)
			if result != tt.expected {
				t.Errorf("IsActiveHub() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestIsStandbyHub(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		expected bool
	}{
		{
			name: "Managed cluster with standby hub label",
			labels: map[string]string{
				constants.GHHubRoleLabelKey: constants.GHHubRoleStandby,
			},
			expected: true,
		},
		{
			name: "Managed cluster with active hub label",
			labels: map[string]string{
				constants.GHHubRoleLabelKey: constants.GHHubRoleActive,
			},
			expected: false,
		},
		{
			name:     "Managed cluster without hub role label",
			labels:   map[string]string{},
			expected: false,
		},
		{
			name:     "Managed cluster with nil labels",
			labels:   nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &unstructured.Unstructured{}
			if tt.labels != nil {
				obj.SetLabels(tt.labels)
			}

			result := IsStandbyHub(obj)
			if result != tt.expected {
				t.Errorf("IsStandbyHub() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestShouldSyncSecretOrConfigMap(t *testing.T) {
	filter := NewHubHAResourceFilter()

	tests := []struct {
		name           string
		kind           string
		labels         map[string]string
		expectedResult bool
	}{
		{
			name: "Secret with cluster type label",
			kind: "Secret",
			labels: map[string]string{
				"cluster.open-cluster-management.io/type": "managed",
			},
			expectedResult: true,
		},
		{
			name: "ConfigMap with hive secret type label",
			kind: "ConfigMap",
			labels: map[string]string{
				"hive.openshift.io/secret-type": "kubeconfig",
			},
			expectedResult: true,
		},
		{
			name: "Secret with backup label",
			kind: "Secret",
			labels: map[string]string{
				"cluster.open-cluster-management.io/backup": "true",
			},
			expectedResult: true,
		},
		{
			name:           "Secret without required labels",
			kind:           "Secret",
			labels:         map[string]string{},
			expectedResult: false,
		},
		{
			name:           "ConfigMap without required labels",
			kind:           "ConfigMap",
			labels:         map[string]string{},
			expectedResult: false,
		},
		{
			name: "Secret with multiple labels including required one",
			kind: "Secret",
			labels: map[string]string{
				"cluster.open-cluster-management.io/type": "managed",
				"some.other.label":                        "value",
			},
			expectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &unstructured.Unstructured{}
			obj.SetKind(tt.kind)
			obj.SetLabels(tt.labels)

			result := filter.shouldSyncSecretOrConfigMap(obj)
			if result != tt.expectedResult {
				t.Errorf("shouldSyncSecretOrConfigMap() = %v, want %v for labels %v",
					result, tt.expectedResult, tt.labels)
			}
		})
	}
}

func BenchmarkShouldSyncResource(b *testing.B) {
	filter := NewHubHAResourceFilter()

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "policy.open-cluster-management.io/v1",
			"kind":       "Policy",
			"metadata": map[string]interface{}{
				"name":      "test-policy",
				"namespace": "default",
			},
		},
	}
	gvk := schema.GroupVersionKind{
		Group:   "policy.open-cluster-management.io",
		Version: "v1",
		Kind:    "Policy",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filter.ShouldSyncResource(obj, gvk)
	}
}
