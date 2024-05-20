package hubofhubs

import (
	"context"
	"testing"

	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestMulticlusterGlobalHubReconciler_pruneMetricsResources(t *testing.T) {
	tests := []struct {
		name        string
		initObjects []runtime.Object
		wantErr     bool
	}{
		{
			name: "remove configmap",
			initObjects: []runtime.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "cm-1",
						Name:      defaultAlertName,
						Labels: map[string]string{
							"global-hub.open-cluster-management.io/metrics-resource": "kafka",
						},
					},
					Data: map[string]string{
						alertConfigMapKey: "test",
					},
				},
			},
		},
		{
			name: "remove servicemonitor",
			initObjects: []runtime.Object{
				&promv1.ServiceMonitor{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "sm-1",
						Name:      defaultAlertName,
						Labels: map[string]string{
							"global-hub.open-cluster-management.io/metrics-resource": "postgres",
						},
					},
				},
			},
		},
		{
			name: "remove podmonitor",
			initObjects: []runtime.Object{
				&promv1.PodMonitor{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "pm-1",
						Name:      defaultAlertName,
						Labels: map[string]string{
							"global-hub.open-cluster-management.io/metrics-resource": "enableMetrics",
						},
					},
				},
			},
		},
		{
			name: "remove prometheus rule",
			initObjects: []runtime.Object{
				&promv1.PrometheusRule{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "pr-1",
						Name:      defaultAlertName,
						Labels: map[string]string{
							"global-hub.open-cluster-management.io/metrics-resource": "enableMetrics",
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			corev1.AddToScheme(scheme.Scheme)
			promv1.AddToScheme(scheme.Scheme)

			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(tt.initObjects...).Build()
			r := &MulticlusterGlobalHubReconciler{
				Client: fakeClient,
				Scheme: scheme.Scheme,
			}
			if err := r.pruneMetricsResources(ctx); (err != nil) != tt.wantErr {
				t.Errorf("MulticlusterGlobalHubReconciler.pruneMetricsResources() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
