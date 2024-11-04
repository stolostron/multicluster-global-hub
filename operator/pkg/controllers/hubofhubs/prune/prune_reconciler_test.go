package prune

import (
	"context"
	"reflect"
	"testing"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	subv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"open-cluster-management.io/api/addon/v1alpha1"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/grafana"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/transporter/protocol"
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
		wantRequeue bool
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
						Labels: map[string]string{
							constants.GlobalHubOwnerLabelKey: "global-hub",
						},
					},
				},
				&kafkav1beta2.KafkaTopic{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kafkatopic",
						Namespace: utils.GetDefaultNamespace(),
						Labels: map[string]string{
							constants.GlobalHubOwnerLabelKey: "global-hub",
						},
					},
				},
			},
		},
		{
			name: "remove kafka topics which has finalizer",
			initObjects: []runtime.Object{
				&kafkav1beta2.KafkaTopic{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kafkatopic",
						Namespace: utils.GetDefaultNamespace(),
						Finalizers: []string{
							"test-final",
						},
						Labels: map[string]string{
							constants.GlobalHubOwnerLabelKey: "global-hub",
						},
					},
				},
			},
			wantErr:     false,
			wantRequeue: true,
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
			needRequeue, err := r.pruneStrimziResources(ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("Case:%v, MulticlusterGlobalHubReconciler.pruneStrimziResources() error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
			if needRequeue != tt.wantRequeue {
				t.Errorf("Case:%v, MulticlusterGlobalHubReconciler.pruneStrimziResources() needRequeue = %v, wantRequeue %v", tt.name, needRequeue, tt.wantRequeue)
			}
		})
	}
}

func TestWebhookResources(t *testing.T) {
	tests := []struct {
		name        string
		initObjects []runtime.Object
		mgh         *globalhubv1alpha4.MulticlusterGlobalHub
		webhookItem int
	}{
		{
			name: "remove webhook resources",
			mgh: &globalhubv1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: utils.GetDefaultNamespace(),
					Name:      "mgh",
					Annotations: map[string]string{
						"global-hub.open-cluster-management.io/import-cluster-in-hosted": "false",
					},
				},
				Spec: globalhubv1alpha4.MulticlusterGlobalHubSpec{},
			},
			webhookItem: 0,
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
						"global-hub.open-cluster-management.io/import-cluster-in-hosted": "true",
					},
				},
				Spec: globalhubv1alpha4.MulticlusterGlobalHubSpec{},
			},
			webhookItem: 1,
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
						"global-hub.open-cluster-management.io/import-cluster-in-hosted": "true",
					},
				},
				Spec: globalhubv1alpha4.MulticlusterGlobalHubSpec{},
			},
			webhookItem: 1,
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

			config.SetImportClusterInHosted(tt.mgh)
			config.SetACMResourceReady(true)

			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(tt.initObjects...).Build()
			r := NewPruneReconciler(fakeClient)
			if _, err := r.Reconcile(ctx, tt.mgh); err != nil {
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

func TestPruneReconciler_hasManagedHub(t *testing.T) {
	tests := []struct {
		name    string
		cmas    []runtime.Object
		want    bool
		wantErr bool
	}{
		{
			name:    "no mca",
			cmas:    []runtime.Object{},
			want:    false,
			wantErr: false,
		},
		{
			name: "has mca",
			cmas: []runtime.Object{
				&addonv1alpha1.ManagedClusterAddOn{
					ObjectMeta: metav1.ObjectMeta{
						Name:      operatorconstants.GHManagedClusterAddonName,
						Namespace: "mh1",
					},
				},
			},
			want:    true,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addonv1alpha1.AddToScheme(scheme.Scheme)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(tt.cmas...).Build()
			r := NewPruneReconciler(fakeClient)
			ctx := context.Background()
			got, err := r.hasManagedHub(ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("PruneReconciler.hasManagedHub() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("PruneReconciler.hasManagedHub() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPruneReconciler_revertClusterManagementAddon(t *testing.T) {
	tests := []struct {
		name    string
		cmas    []runtime.Object
		wantErr bool
	}{
		{
			name:    "no cma",
			cmas:    []runtime.Object{},
			wantErr: false,
		},
		{
			name: "cma do not have placements",
			cmas: []runtime.Object{
				&v1alpha1.ClusterManagementAddOn{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "work-manager",
						Namespace: utils.GetDefaultNamespace(),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "cma do not have target placements",
			cmas: []runtime.Object{
				&v1alpha1.ClusterManagementAddOn{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "work-manager",
						Namespace: utils.GetDefaultNamespace(),
					},
					Spec: addonv1alpha1.ClusterManagementAddOnSpec{
						InstallStrategy: addonv1alpha1.InstallStrategy{
							Placements: []addonv1alpha1.PlacementStrategy{
								v1alpha1.PlacementStrategy{
									PlacementRef: v1alpha1.PlacementRef{
										Namespace: constants.GHDefaultNamespace,
										Name:      "global",
									},
									Configs: []v1alpha1.AddOnConfig{
										{
											ConfigReferent: v1alpha1.ConfigReferent{
												Name:      "global-hub",
												Namespace: constants.GHDefaultNamespace,
											},
											ConfigGroupResource: v1alpha1.ConfigGroupResource{
												Group:    "addon.open-cluster-management.io",
												Resource: "addondeploymentconfigs",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "cma have target placements",
			cmas: []runtime.Object{
				&v1alpha1.ClusterManagementAddOn{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "work-manager",
						Namespace: utils.GetDefaultNamespace(),
					},
					Spec: addonv1alpha1.ClusterManagementAddOnSpec{
						InstallStrategy: addonv1alpha1.InstallStrategy{
							Placements: []addonv1alpha1.PlacementStrategy{
								v1alpha1.PlacementStrategy{
									PlacementRef: v1alpha1.PlacementRef{
										Namespace: constants.GHDefaultNamespace,
										Name:      "non-local-cluster",
									},
									Configs: []v1alpha1.AddOnConfig{
										{
											ConfigReferent: v1alpha1.ConfigReferent{
												Name:      "global-hub",
												Namespace: constants.GHDefaultNamespace,
											},
											ConfigGroupResource: v1alpha1.ConfigGroupResource{
												Group:    "addon.open-cluster-management.io",
												Resource: "addondeploymentconfigs",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addonv1alpha1.AddToScheme(scheme.Scheme)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(tt.cmas...).Build()
			r := NewPruneReconciler(fakeClient)
			ctx := context.Background()
			if err := r.revertClusterManagementAddon(ctx); (err != nil) != tt.wantErr {
				t.Errorf("PruneReconciler.revertClusterManagementAddon() error = %v, wantErr %v", err, tt.wantErr)
			}
			cmaList := &v1alpha1.ClusterManagementAddOnList{}

			err := r.Client.List(ctx, cmaList)
			if err != nil {
				t.Errorf("Failed to list cma:%v", err)
			}
			for _, cma := range cmaList.Items {
				if !config.HostedAddonList.Has(cma.Name) {
					continue
				}
				for _, pl := range cma.Spec.InstallStrategy.Placements {
					if reflect.DeepEqual(pl.PlacementRef, config.GlobalHubHostedAddonPlacementStrategy.PlacementRef) {
						t.Errorf("Failed to revert cma")
					}
				}
			}
		})
	}
}
