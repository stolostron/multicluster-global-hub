package webhook

import (
	"context"
	"testing"
	"time"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	subv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

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
			name: "remove webhook resources when webhook needed for hosted cluster, and mgh deleting",
			mgh: &globalhubv1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: utils.GetDefaultNamespace(),
					Name:      "mgh",
					Annotations: map[string]string{
						"global-hub.open-cluster-management.io/import-cluster-in-hosted": "true",
					},
					Finalizers: []string{
						"fn",
					},
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
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
			name: "remove webhook resources when webhook is needed, but mgh deleting",
			mgh: &globalhubv1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: utils.GetDefaultNamespace(),
					Name:      "mgh",
					Annotations: map[string]string{
						"global-hub.open-cluster-management.io/import-cluster-in-hosted": "true",
					},
					Finalizers: []string{
						"fn",
					},
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			kafkav1beta2.AddToScheme(scheme.Scheme)
			subv1alpha1.AddToScheme(scheme.Scheme)
			addonv1alpha1.AddToScheme(scheme.Scheme)
			globalhubv1alpha4.AddToScheme(scheme.Scheme)
			config.SetImportClusterInHosted(tt.mgh)
			config.SetACMResourceReady(true)
			tt.initObjects = append(tt.initObjects, tt.mgh)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(tt.initObjects...).Build()
			wr := WebhookReconciler{
				c: fakeClient,
			}

			if _, err := wr.Reconcile(ctx, ctrl.Request{}); err != nil {
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
