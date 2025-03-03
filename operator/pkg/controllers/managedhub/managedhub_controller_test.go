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

package managedhub

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
)

func TestManagedHubController_Reconcile(t *testing.T) {
	tests := []struct {
		name    string
		mghObj  *v1alpha4.MulticlusterGlobalHub
		wantErr bool
	}{
		{
			name:    "no mgh",
			mghObj:  nil,
			wantErr: false,
		},
		{
			name: "has 1 ready mgh",
			mghObj: &v1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "multicluster-global-hub",
				},
				Spec: v1alpha4.MulticlusterGlobalHubSpec{
					DataLayerSpec: v1alpha4.DataLayerSpec{},
				},
			},
			wantErr: false,
		},
		{
			name: "has 1 deleting mgh",
			mghObj: &v1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test",
					Namespace:         "multicluster-global-hub",
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
					Finalizers: []string{
						"fz",
					},
				},
				Spec: v1alpha4.MulticlusterGlobalHubSpec{
					DataLayerSpec: v1alpha4.DataLayerSpec{},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v1alpha4.AddToScheme(scheme.Scheme)
			clusterv1.AddToScheme(scheme.Scheme)
			addonapiv1alpha1.AddToScheme(scheme.Scheme)
			var fakeClient client.WithWatch
			if tt.mghObj == nil {
				fakeClient = fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects().Build()
			} else {
				fakeClient = fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(tt.mghObj).Build()
			}

			r := &ManagedHubController{
				c: fakeClient,
			}
			_, err := r.Reconcile(context.Background(), ctrl.Request{})
			if (err != nil) != tt.wantErr {
				t.Errorf("ManagedHubController.Reconcile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func TestManagedHubController_pruneManagedHubs(t *testing.T) {
	type fields struct {
		c client.Client
	}

	tests := []struct {
		name        string
		wantErr     bool
		initObjects []runtime.Object
	}{
		{
			name:    "no managedcluster",
			wantErr: false,
		},
		{
			name:    "only has local cluster",
			wantErr: false,
			initObjects: []runtime.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "local-cluster-rename",
						Labels: map[string]string{
							"local-cluster": "true",
						},
					},
				},
			},
		},
		{
			name:    "cluster has no annotation",
			wantErr: false,
			initObjects: []runtime.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "mc1",
					},
				},
			},
		},
		{
			name:    "cluster has globalhub annotation",
			wantErr: false,
			initObjects: []runtime.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "mc1",
						Annotations: map[string]string{
							operatorconstants.AnnotationONMulticlusterHub:       "true",
							operatorconstants.AnnotationPolicyONMulticlusterHub: "true",
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clusterv1.AddToScheme(scheme.Scheme)
			addonapiv1alpha1.AddToScheme(scheme.Scheme)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(tt.initObjects...).Build()

			r := &ManagedHubController{
				c: fakeClient,
			}
			if err := r.pruneManagedHubs(context.Background()); (err != nil) != tt.wantErr {
				t.Errorf("ManagedHubController.pruneManagedHubs() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
