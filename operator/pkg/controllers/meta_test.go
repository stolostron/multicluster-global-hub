/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
)

var (
	name      = "mgh"
	namespace = "default"
)

func Test_updateMghStatus(t *testing.T) {
	tests := []struct {
		name         string
		mgh          *v1alpha4.MulticlusterGlobalHub
		desiredPhase v1alpha4.GlobalHubPhaseType
		isReady      bool
		reconcileErr error
	}{
		{
			name: "init mgh",
			mgh: &v1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
			},
			desiredPhase: v1alpha4.GlobalHubProgressing,
			isReady:      false,
		},
		{
			name: "init mgh",
			mgh: &v1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
			},
			desiredPhase: v1alpha4.GlobalHubProgressing,
			isReady:      false,
		},
		{
			name: "acm ready, components ready",
			mgh: &v1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Status: v1alpha4.MulticlusterGlobalHubStatus{
					Phase: v1alpha4.GlobalHubProgressing,
					Components: map[string]v1alpha4.StatusCondition{
						config.COMPONENTS_MANAGER_NAME: {
							Kind:               "Deployment",
							Name:               config.COMPONENTS_MANAGER_NAME,
							Type:               config.COMPONENTS_AVAILABLE,
							Status:             config.CONDITION_STATUS_TRUE,
							LastTransitionTime: metav1.Time{Time: time.Now()},
						},
						config.COMPONENTS_KAFKA_NAME: {
							Kind:               "Deployment",
							Name:               config.COMPONENTS_KAFKA_NAME,
							Type:               config.COMPONENTS_AVAILABLE,
							Status:             config.CONDITION_STATUS_TRUE,
							LastTransitionTime: metav1.Time{Time: time.Now()},
						},
						config.COMPONENTS_POSTGRES_NAME: {
							Kind:               "Statefulset",
							Name:               config.COMPONENTS_POSTGRES_NAME,
							Type:               config.COMPONENTS_AVAILABLE,
							Status:             config.CONDITION_STATUS_TRUE,
							LastTransitionTime: metav1.Time{Time: time.Now()},
						},
						config.COMPONENTS_GRAFANA_NAME: {
							Kind:               "Deployment",
							Name:               config.COMPONENTS_GRAFANA_NAME,
							Type:               config.COMPONENTS_AVAILABLE,
							Status:             config.CONDITION_STATUS_TRUE,
							LastTransitionTime: metav1.Time{Time: time.Now()},
						},
					},
				},
			},
			isReady:      true,
			desiredPhase: v1alpha4.GlobalHubRunning,
		},
		{
			name: "acm ready, grafana not ready",
			mgh: &v1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Status: v1alpha4.MulticlusterGlobalHubStatus{
					Phase: v1alpha4.GlobalHubProgressing,
					Components: map[string]v1alpha4.StatusCondition{
						config.COMPONENTS_MANAGER_NAME: {
							Kind:               "Deployment",
							Name:               config.COMPONENTS_MANAGER_NAME,
							Type:               config.COMPONENTS_AVAILABLE,
							Status:             config.CONDITION_STATUS_TRUE,
							LastTransitionTime: metav1.Time{Time: time.Now()},
						},
						config.COMPONENTS_KAFKA_NAME: {
							Kind:               "Deployment",
							Name:               config.COMPONENTS_KAFKA_NAME,
							Type:               config.COMPONENTS_AVAILABLE,
							Status:             config.CONDITION_STATUS_TRUE,
							LastTransitionTime: metav1.Time{Time: time.Now()},
						},
						config.COMPONENTS_POSTGRES_NAME: {
							Kind:               "Statefulset",
							Name:               config.COMPONENTS_POSTGRES_NAME,
							Type:               config.COMPONENTS_AVAILABLE,
							Status:             config.CONDITION_STATUS_TRUE,
							LastTransitionTime: metav1.Time{Time: time.Now()},
						},
						config.COMPONENTS_GRAFANA_NAME: {
							Kind:               "Deployment",
							Name:               config.COMPONENTS_GRAFANA_NAME,
							Type:               config.COMPONENTS_AVAILABLE,
							Status:             config.CONDITION_STATUS_FALSE,
							LastTransitionTime: metav1.Time{Time: time.Now()},
						},
					},
				},
			},
			isReady:      false,
			desiredPhase: v1alpha4.GlobalHubProgressing,
		},
		{
			name: "acm ready, grafana not ready, no need update",
			mgh: &v1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Status: v1alpha4.MulticlusterGlobalHubStatus{
					Phase: v1alpha4.GlobalHubProgressing,
					Conditions: []metav1.Condition{
						{
							Type:    config.CONDITION_TYPE_GLOBALHUB_READY,
							Status:  config.CONDITION_STATUS_FALSE,
							Reason:  config.CONDITION_REASON_GLOBALHUB_NOT_READY,
							Message: config.CONDITION_MESSAGE_GLOBALHUB_NOT_READY,
						},
					},
					Components: map[string]v1alpha4.StatusCondition{
						config.COMPONENTS_MANAGER_NAME: {
							Kind:               "Deployment",
							Name:               config.COMPONENTS_MANAGER_NAME,
							Type:               config.COMPONENTS_AVAILABLE,
							Status:             config.CONDITION_STATUS_TRUE,
							LastTransitionTime: metav1.Time{Time: time.Now()},
						},
						config.COMPONENTS_KAFKA_NAME: {
							Kind:               "Deployment",
							Name:               config.COMPONENTS_KAFKA_NAME,
							Type:               config.COMPONENTS_AVAILABLE,
							Status:             config.CONDITION_STATUS_TRUE,
							LastTransitionTime: metav1.Time{Time: time.Now()},
						},
						config.COMPONENTS_POSTGRES_NAME: {
							Kind:               "Statefulset",
							Name:               config.COMPONENTS_POSTGRES_NAME,
							Type:               config.COMPONENTS_AVAILABLE,
							Status:             config.CONDITION_STATUS_TRUE,
							LastTransitionTime: metav1.Time{Time: time.Now()},
						},
						config.COMPONENTS_GRAFANA_NAME: {
							Kind:               "Deployment",
							Name:               config.COMPONENTS_GRAFANA_NAME,
							Type:               config.COMPONENTS_AVAILABLE,
							Status:             config.CONDITION_STATUS_FALSE,
							LastTransitionTime: metav1.Time{Time: time.Now()},
						},
					},
				},
			},
			isReady:      false,
			desiredPhase: v1alpha4.GlobalHubProgressing,
		},
		{
			name: "reconcile error",
			mgh: &v1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
			},
			isReady:      false,
			desiredPhase: v1alpha4.GlobalHubError,
			reconcileErr: fmt.Errorf("reconcile error"),
		},
	}
	for _, tt := range tests {
		_ = v1alpha4.AddToScheme(scheme.Scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(tt.mgh).WithStatusSubresource(tt.mgh).Build()
		ctx := context.Background()
		config.SetACMResourceReady(true)

		t.Run(tt.name, func(t *testing.T) {
			if err := updateMGHReadyStatus(context.Background(), fakeClient, tt.mgh, tt.reconcileErr); err != nil {
				t.Errorf("updateMghStatus() error = %v", err)
			}

			curmgh := &v1alpha4.MulticlusterGlobalHub{}
			_ = fakeClient.Get(ctx, types.NamespacedName{
				Namespace: tt.mgh.GetNamespace(),
				Name:      tt.mgh.GetName(),
			}, curmgh)

			if curmgh.Status.Phase != tt.desiredPhase {
				t.Errorf("name: %v, Desired phase: %v, current phase: %v", tt.name, tt.desiredPhase, curmgh.Status.Phase)
			}
			if meta.IsStatusConditionTrue(curmgh.GetConditions(), config.CONDITION_TYPE_GLOBALHUB_READY) != tt.isReady {
				t.Errorf("name:%v, desired ready condition: %v, status: %v", tt.name, tt.isReady, curmgh.GetConditions())
			}
		})
	}
}

func Test_needUpdatePhase(t *testing.T) {
	tests := []struct {
		name  string
		mgh   *v1alpha4.MulticlusterGlobalHub
		acm   bool
		want  bool
		want1 v1alpha4.GlobalHubPhaseType
	}{
		{
			name: "no mgh status,  no components set",
			mgh: &v1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
			},
			acm:   true,
			want:  true,
			want1: v1alpha4.GlobalHubProgressing,
		},
		{
			name: "acm ready, components ready",
			mgh: &v1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Status: v1alpha4.MulticlusterGlobalHubStatus{
					Phase: v1alpha4.GlobalHubProgressing,
					Components: map[string]v1alpha4.StatusCondition{
						config.COMPONENTS_MANAGER_NAME: {
							Kind:               "Deployment",
							Name:               config.COMPONENTS_MANAGER_NAME,
							Type:               config.COMPONENTS_AVAILABLE,
							Status:             config.CONDITION_STATUS_TRUE,
							LastTransitionTime: metav1.Time{Time: time.Now()},
						},
						config.COMPONENTS_KAFKA_NAME: {
							Kind:               "Deployment",
							Name:               config.COMPONENTS_KAFKA_NAME,
							Type:               config.COMPONENTS_AVAILABLE,
							Status:             config.CONDITION_STATUS_TRUE,
							LastTransitionTime: metav1.Time{Time: time.Now()},
						},
						config.COMPONENTS_POSTGRES_NAME: {
							Kind:               "Statefulset",
							Name:               config.COMPONENTS_POSTGRES_NAME,
							Type:               config.COMPONENTS_AVAILABLE,
							Status:             config.CONDITION_STATUS_TRUE,
							LastTransitionTime: metav1.Time{Time: time.Now()},
						},
						config.COMPONENTS_GRAFANA_NAME: {
							Kind:               "Deployment",
							Name:               config.COMPONENTS_GRAFANA_NAME,
							Type:               config.COMPONENTS_AVAILABLE,
							Status:             config.CONDITION_STATUS_TRUE,
							LastTransitionTime: metav1.Time{Time: time.Now()},
						},
					},
				},
			},
			acm:   true,
			want:  true,
			want1: v1alpha4.GlobalHubRunning,
		},
		{
			name: "acm ready, grafana not ready",
			mgh: &v1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Status: v1alpha4.MulticlusterGlobalHubStatus{
					Phase: v1alpha4.GlobalHubProgressing,
					Components: map[string]v1alpha4.StatusCondition{
						config.COMPONENTS_MANAGER_NAME: {
							Kind:               "Deployment",
							Name:               config.COMPONENTS_MANAGER_NAME,
							Type:               config.COMPONENTS_AVAILABLE,
							Status:             config.CONDITION_STATUS_TRUE,
							LastTransitionTime: metav1.Time{Time: time.Now()},
						},
						config.COMPONENTS_KAFKA_NAME: {
							Kind:               "Deployment",
							Name:               config.COMPONENTS_KAFKA_NAME,
							Type:               config.COMPONENTS_AVAILABLE,
							Status:             config.CONDITION_STATUS_TRUE,
							LastTransitionTime: metav1.Time{Time: time.Now()},
						},
						config.COMPONENTS_POSTGRES_NAME: {
							Kind:               "Statefulset",
							Name:               config.COMPONENTS_POSTGRES_NAME,
							Type:               config.COMPONENTS_AVAILABLE,
							Status:             config.CONDITION_STATUS_TRUE,
							LastTransitionTime: metav1.Time{Time: time.Now()},
						},
						config.COMPONENTS_GRAFANA_NAME: {
							Kind:               "Deployment",
							Name:               config.COMPONENTS_GRAFANA_NAME,
							Type:               config.COMPONENTS_AVAILABLE,
							Status:             config.CONDITION_STATUS_FALSE,
							LastTransitionTime: metav1.Time{Time: time.Now()},
						},
					},
				},
			},
			acm:   true,
			want:  false,
			want1: v1alpha4.GlobalHubProgressing,
		},
		{
			name: "acm ready, inventory not ready",
			mgh: &v1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
					Annotations: map[string]string{
						constants.AnnotationMGHWithInventory: "true",
					},
				},
				Status: v1alpha4.MulticlusterGlobalHubStatus{
					Phase: v1alpha4.GlobalHubProgressing,
					Components: map[string]v1alpha4.StatusCondition{
						config.COMPONENTS_MANAGER_NAME: {
							Kind:               "Deployment",
							Name:               config.COMPONENTS_MANAGER_NAME,
							Type:               config.COMPONENTS_AVAILABLE,
							Status:             config.CONDITION_STATUS_TRUE,
							LastTransitionTime: metav1.Time{Time: time.Now()},
						},
						config.COMPONENTS_KAFKA_NAME: {
							Kind:               "Deployment",
							Name:               config.COMPONENTS_KAFKA_NAME,
							Type:               config.COMPONENTS_AVAILABLE,
							Status:             config.CONDITION_STATUS_TRUE,
							LastTransitionTime: metav1.Time{Time: time.Now()},
						},
						config.COMPONENTS_POSTGRES_NAME: {
							Kind:               "Statefulset",
							Name:               config.COMPONENTS_POSTGRES_NAME,
							Type:               config.COMPONENTS_AVAILABLE,
							Status:             config.CONDITION_STATUS_TRUE,
							LastTransitionTime: metav1.Time{Time: time.Now()},
						},
						config.COMPONENTS_GRAFANA_NAME: {
							Kind:               "Deployment",
							Name:               config.COMPONENTS_GRAFANA_NAME,
							Type:               config.COMPONENTS_AVAILABLE,
							Status:             config.CONDITION_STATUS_TRUE,
							LastTransitionTime: metav1.Time{Time: time.Now()},
						},
					},
				},
			},
			acm:   true,
			want:  false,
			want1: v1alpha4.GlobalHubProgressing,
		},
		{
			name: "acm ready, inventory ready",
			mgh: &v1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
					Annotations: map[string]string{
						constants.AnnotationMGHWithInventory: "true",
					},
				},
				Status: v1alpha4.MulticlusterGlobalHubStatus{
					Phase: v1alpha4.GlobalHubProgressing,
					Components: map[string]v1alpha4.StatusCondition{
						config.COMPONENTS_MANAGER_NAME: {
							Kind:               "Deployment",
							Name:               config.COMPONENTS_MANAGER_NAME,
							Type:               config.COMPONENTS_AVAILABLE,
							Status:             config.CONDITION_STATUS_TRUE,
							LastTransitionTime: metav1.Time{Time: time.Now()},
						},
						config.COMPONENTS_KAFKA_NAME: {
							Kind:               "Deployment",
							Name:               config.COMPONENTS_KAFKA_NAME,
							Type:               config.COMPONENTS_AVAILABLE,
							Status:             config.CONDITION_STATUS_TRUE,
							LastTransitionTime: metav1.Time{Time: time.Now()},
						},
						config.COMPONENTS_POSTGRES_NAME: {
							Kind:               "Statefulset",
							Name:               config.COMPONENTS_POSTGRES_NAME,
							Type:               config.COMPONENTS_AVAILABLE,
							Status:             config.CONDITION_STATUS_TRUE,
							LastTransitionTime: metav1.Time{Time: time.Now()},
						},
						config.COMPONENTS_GRAFANA_NAME: {
							Kind:               "Deployment",
							Name:               config.COMPONENTS_GRAFANA_NAME,
							Type:               config.COMPONENTS_AVAILABLE,
							Status:             config.CONDITION_STATUS_TRUE,
							LastTransitionTime: metav1.Time{Time: time.Now()},
						},
						config.COMPONENTS_INVENTORY_API_NAME: {
							Kind:               "Deployment",
							Name:               config.COMPONENTS_INVENTORY_API_NAME,
							Type:               config.COMPONENTS_AVAILABLE,
							Status:             config.CONDITION_STATUS_TRUE,
							LastTransitionTime: metav1.Time{Time: time.Now()},
						},
					},
				},
			},
			acm:   true,
			want:  true,
			want1: v1alpha4.GlobalHubRunning,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.SetACMResourceReady(tt.acm)
			got, got1 := needUpdatePhase(tt.mgh)
			if got != tt.want {
				t.Errorf("name:%v, needUpdatePhase() got = %v, want %v", tt.name, got, tt.want)
			}
			if !reflect.DeepEqual(got1, tt.want1) {
				t.Errorf("name:%v, needUpdatePhase() got1 = %v, want %v", tt.name, got1, tt.want1)
			}
		})
	}
}
