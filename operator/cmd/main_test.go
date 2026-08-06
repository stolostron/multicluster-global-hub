package main

import (
	"context"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
)

func TestDetectOLMVersion(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	tests := []struct {
		name     string
		envVar   string
		crdName  string
		expected string
	}{
		{
			name:     "OLMv0 detected via env var",
			envVar:   "my-operator",
			expected: config.OLMVersionV0,
		},
		{
			name:     "OLMv1 detected via CRD",
			crdName:  "clusterextensions.olm.operatorframework.io",
			expected: config.OLMVersionV1,
		},
		{
			name:     "OLMv0 detected via CRD",
			crdName:  "subscriptions.operators.coreos.com",
			expected: config.OLMVersionV0,
		},
		{
			name:     "no OLM detected",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envVar != "" {
				t.Setenv("OPERATOR_CONDITION_NAME", tt.envVar)
			}

			var objs []runtime.Object
			if tt.crdName != "" {
				objs = append(objs, &apiextensionsv1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{Name: tt.crdName},
				})
			}

			c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()
			result, err := detectOLMVersion(context.Background(), c)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}
