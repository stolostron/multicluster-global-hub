package prune

import (
	"context"
	"testing"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	subv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/hubofhubs/grafana"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/hubofhubs/transporter/protocol"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
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
			var operatorConfig *config.OperatorConfig
			r := NewPruneReconciler(fakeClient, operatorConfig)
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
			var operatorConfig *config.OperatorConfig
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(tt.initObjects...).Build()
			r := NewPruneReconciler(fakeClient, operatorConfig)
			if err := r.pruneStrimziResources(ctx); (err != nil) != tt.wantErr {
				t.Errorf("MulticlusterGlobalHubReconciler.pruneStrimziResources() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWebhookResources(t *testing.T) {
	tests := []struct {
		name                 string
		initObjects          []runtime.Object
		mgh                  *globalhubv1alpha4.MulticlusterGlobalHub
		enableGlobalResource bool
		webhookItem          int
	}{
		{
			name: "remove webhook resources",
			mgh: &globalhubv1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: utils.GetDefaultNamespace(),
					Name:      "mgh",
					Annotations: map[string]string{
						"import-cluster-in-hosted": "false",
					},
				},
				Spec: globalhubv1alpha4.MulticlusterGlobalHubSpec{},
			},
			enableGlobalResource: false,
			webhookItem:          0,
			initObjects: []runtime.Object{
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "multicluster-global-hub-webhook",
						Namespace: utils.GetDefaultNamespace(),
						Labels: map[string]string{
							"global-hub.open-cluster-management.io/managed-by": "global-hub-operator",
							"service": "multicluster-global-hub-webhook",
						},
					},
				},
				&admissionregistrationv1.MutatingWebhookConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name: "multicluster-global-hub-mutator",
						Labels: map[string]string{
							"global-hub.open-cluster-management.io/managed-by": "global-hub-operator",
						},
					},
				},
			},
		},

		{
			name: "do not remove webhook resources because webhook needed for hosted cluster",
			mgh: &globalhubv1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: utils.GetDefaultNamespace(),
					Name:      "mgh",
					Annotations: map[string]string{
						"import-cluster-in-hosted": "true",
					},
				},
				Spec: globalhubv1alpha4.MulticlusterGlobalHubSpec{},
			},
			enableGlobalResource: false,
			webhookItem:          1,
			initObjects: []runtime.Object{
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "multicluster-global-hub-webhook",
						Namespace: utils.GetDefaultNamespace(),
						Labels: map[string]string{
							"global-hub.open-cluster-management.io/managed-by": "global-hub-operator",
							"service": "multicluster-global-hub-webhook",
						},
					},
				},
				&admissionregistrationv1.MutatingWebhookConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name: "multicluster-global-hub-mutator",
						Labels: map[string]string{
							"global-hub.open-cluster-management.io/managed-by": "global-hub-operator",
						},
					},
				},
			},
		},
		{
			name: "do not remove webhook resources because webhook needed for global resource",
			mgh: &globalhubv1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: utils.GetDefaultNamespace(),
					Name:      "mgh",
				},
				Spec: globalhubv1alpha4.MulticlusterGlobalHubSpec{},
			},
			enableGlobalResource: true,
			webhookItem:          1,
			initObjects: []runtime.Object{
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "multicluster-global-hub-webhook",
						Namespace: utils.GetDefaultNamespace(),
						Labels: map[string]string{
							"global-hub.open-cluster-management.io/managed-by": "global-hub-operator",
							"service": "multicluster-global-hub-webhook",
						},
					},
				},
				&admissionregistrationv1.MutatingWebhookConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name: "multicluster-global-hub-mutator",
						Labels: map[string]string{
							"global-hub.open-cluster-management.io/managed-by": "global-hub-operator",
						},
					},
				},
			},
		},
		{
			name: "do not remove webhook resources because webhook is needed",
			mgh: &globalhubv1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: utils.GetDefaultNamespace(),
					Name:      "mgh",
					Annotations: map[string]string{
						"import-cluster-in-hosted": "true",
					},
				},
				Spec: globalhubv1alpha4.MulticlusterGlobalHubSpec{},
			},
			enableGlobalResource: true,
			webhookItem:          1,
			initObjects: []runtime.Object{
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "multicluster-global-hub-webhook",
						Namespace: utils.GetDefaultNamespace(),
						Labels: map[string]string{
							"global-hub.open-cluster-management.io/managed-by": "global-hub-operator",
							"service": "multicluster-global-hub-webhook",
						},
					},
				},
				&admissionregistrationv1.MutatingWebhookConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name: "multicluster-global-hub-mutator",
						Labels: map[string]string{
							"global-hub.open-cluster-management.io/managed-by": "global-hub-operator",
						},
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
			addonv1alpha1.AddToScheme(scheme.Scheme)
			operatorConfig := &config.OperatorConfig{
				GlobalResourceEnabled: tt.enableGlobalResource,
			}
			config.SetImportClusterInHosted(tt.mgh)

			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(tt.initObjects...).Build()
			r := NewPruneReconciler(fakeClient, operatorConfig)
			if err := r.Reconcile(ctx, tt.mgh); err != nil {
				t.Errorf("MulticlusterGlobalHubReconciler.reconcile() error = %v", err)
			}
			listOpts := []client.ListOption{
				client.MatchingLabels(map[string]string{
					constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
				}),
			}
			webhookList := &admissionregistrationv1.MutatingWebhookConfigurationList{}
			if err := fakeClient.List(ctx, webhookList, listOpts...); err != nil {
				t.Errorf("Failed to list webhook config")
			}
			if len(webhookList.Items) != tt.webhookItem {
				t.Errorf("Name:%v, Existing webhookItems:%v, want webhook items:%v", tt.name, len(webhookList.Items), tt.webhookItem)
			}

			webhookServiceListOpts := []client.ListOption{
				client.MatchingLabels(map[string]string{
					constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
					"service":                        "multicluster-global-hub-webhook",
				}),
			}
			webhookServiceList := &corev1.ServiceList{}
			if err := fakeClient.List(ctx, webhookServiceList, webhookServiceListOpts...); err != nil {
				t.Errorf("Failed to list webhook service")
			}
			if len(webhookServiceList.Items) != tt.webhookItem {
				t.Errorf("Name:%v,Existing webhookServiceList:%v, want webhook items:%v", tt.name, len(webhookServiceList.Items), tt.webhookItem)
			}
		})
	}
}
