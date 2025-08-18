// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package protocol

import (
	"context"
	"testing"
	"time"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	subv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func TestMulticlusterGlobalHubReconcilerStrimziResources(t *testing.T) {
	tests := []struct {
		name         string
		initObjects  []runtime.Object
		wantErr      bool
		requeueAfter time.Duration
	}{
		{
			name: "remove kafka resources",
			initObjects: []runtime.Object{
				&kafkav1beta2.Kafka{
					ObjectMeta: metav1.ObjectMeta{
						Name:      KafkaClusterName,
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
			wantErr:      false,
			requeueAfter: 5 * time.Second,
		},
		{
			name: "remove subscription and csv",
			initObjects: []runtime.Object{
				&subv1alpha1.Subscription{
					ObjectMeta: metav1.ObjectMeta{
						Name:      DefaultKafkaSubName,
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
			_ = kafkav1beta2.AddToScheme(scheme.Scheme)
			_ = subv1alpha1.AddToScheme(scheme.Scheme)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(tt.initObjects...).Build()
			kc := KafkaController{
				c: fakeClient,
				trans: &strimziTransporter{
					subName:          DefaultKafkaSubName,
					kafkaClusterName: "kafka",
				},
			}
			returnResult, err := kc.pruneStrimziResources(ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("Case:%v, MulticlusterGlobalHubReconciler.pruneStrimziResources() error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
			if returnResult.RequeueAfter != tt.requeueAfter {
				t.Errorf("Case:%v, MulticlusterGlobalHubReconciler.pruneStrimziResources() needRequeue = %v, wantRequeue %v", tt.name, returnResult.RequeueAfter, tt.requeueAfter)
			}
		})
	}
}
