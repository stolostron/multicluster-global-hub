package prune

import (
	"context"
	"testing"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	subv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/hubofhubs/grafana"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/hubofhubs/transporter/protocol"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func TestPruneMetricsResources(t *testing.T) {
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
						Name:      grafana.DefaultAlertName,
						Labels: map[string]string{
							"global-hub.open-cluster-management.io/metrics-resource": "kafka",
						},
					},
					Data: map[string]string{
						grafana.AlertConfigMapKey: "test",
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
						Name:      grafana.DefaultAlertName,
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
						Name:      grafana.DefaultAlertName,
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
						Name:      grafana.DefaultAlertName,
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
			r := NewPruneReconciler(fakeClient)
			if err := r.MetricsResources(ctx); (err != nil) != tt.wantErr {
				t.Errorf("pruneMetricsResources() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMulticlusterGlobalHubReconcilerStrimziResources(t *testing.T) {
	tests := []struct {
		name        string
		initObjects []runtime.Object
		wantErr     bool
	}{
		{
			name: "remove kafka resources",
			initObjects: []runtime.Object{
				&kafkav1beta2.Kafka{
					ObjectMeta: metav1.ObjectMeta{
						Name:      protocol.KafkaClusterName,
						Namespace: utils.GetDefaultNamespace(),
					},
				},
				&kafkav1beta2.KafkaUser{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kafkauser",
						Namespace: utils.GetDefaultNamespace(),
					},
				},
				&kafkav1beta2.KafkaTopic{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kafkatopic",
						Namespace: utils.GetDefaultNamespace(),
					},
				},
			},
		},
		{
			name: "remove subscription and csv",
			initObjects: []runtime.Object{
				&subv1alpha1.Subscription{
					ObjectMeta: metav1.ObjectMeta{
						Name:      protocol.DefaultKafkaSubName,
						Namespace: utils.GetDefaultNamespace(),
					},
					Status: subv1alpha1.SubscriptionStatus{
						InstalledCSV: "kafka-0.40.0",
					},
				},
				&subv1alpha1.ClusterServiceVersion{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kafka-0.40.0",
						Namespace: utils.GetDefaultNamespace(),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			kafkav1beta2.AddToScheme(scheme.Scheme)
			subv1alpha1.AddToScheme(scheme.Scheme)

			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(tt.initObjects...).Build()
			r := NewPruneReconciler(fakeClient)
			if err := r.pruneStrimziResources(ctx); (err != nil) != tt.wantErr {
				t.Errorf("MulticlusterGlobalHubReconciler.pruneStrimziResources() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
