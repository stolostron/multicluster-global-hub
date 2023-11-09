/*
Copyright 2022.

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

package utils

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/condition"
)

var (
	cfg           *rest.Config
	runtimeClient client.Client
	ctx           context.Context
)

func Test_getAlertGPCcount(t *testing.T) {
	tests := []struct {
		name        string
		alert       []byte
		wantContact int
		wantGroup   int
		wantPolicy  int
		wantErr     bool
	}{
		{
			name:        "default alert",
			alert:       []byte("apiVersion: 1\ngroups:\n  - orgId: 1\n    name: Suspicious policy change\n    folder: Policy\n  - orgId: 1\n    name: Suspicious Cluster Compliance Status Change\n    folder: Policy"),
			wantContact: 0,
			wantGroup:   2,
			wantPolicy:  0,
			wantErr:     false,
		},
		{
			name: "error alert",
			alert: []byte(`
	apiVersion: 1
	contactPoints:
	- name: alerts-cu-webhook
		orgId: 1
		receivers:
		- disableResolveMessage: false
		type: email
		uid: 4e3bfe25-00cf-4173-b02b-16f077e539da`),
			wantContact: 0,
			wantGroup:   0,
			wantPolicy:  0,
			wantErr:     true,
		},
		{
			name: "merged alert",
			alert: []byte(`
apiVersion: 1
contactPoints:
- name: alerts-cu-webhook
  orgId: 1
  receivers:
  - disableResolveMessage: false
    type: email
    uid: 4e3bfe25-00cf-4173-b02b-16f077e539da
groups:
- folder: Policy
  name: Suspicious policy change
  orgId: 1
- folder: Policy
  name: Suspicious Cluster Compliance Status Change
  orgId: 1
- folder: Custom
  name: Suspicious policy change
  orgId: 1
- folder: Custom
  name: Suspicious Cluster Compliance Status Change
  orgId: 1
policies:
- orgId: 1
  receiver: alerts-cu-webhook`),
			wantContact: 1,
			wantGroup:   4,
			wantPolicy:  1,
			wantErr:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotGroup, gotPolicy, gotContact, err := GetAlertGPCcount(tt.alert)
			if (err != nil) != tt.wantErr {
				t.Errorf("getAlertGPCcount() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotGroup != tt.wantGroup {
				t.Errorf("getAlertGPCcount() got = %v, want %v", gotGroup, tt.wantGroup)
			}
			if gotPolicy != tt.wantPolicy {
				t.Errorf("getAlertGPCcount() got1 = %v, want %v", gotPolicy, tt.wantPolicy)
			}
			if gotContact != tt.wantContact {
				t.Errorf("getAlertGPCcount() got2 = %v, want %v", gotContact, tt.wantContact)
			}
		})
	}
}

func Test_isAlertCountEqual(t *testing.T) {
	tests := []struct {
		name    string
		a       []byte
		b       []byte
		want    bool
		wantErr bool
	}{
		{
			name: "two equal alert which has all fields",
			a: []byte(`
apiVersion: 1
contactPoints:
- name: alerts-cu-webhook
  orgId: 1
  receivers:
  - disableResolveMessage: false
    type: email
    uid: 4e3bfe25-00cf-4173-b02b-16f077e539da
groups:
- folder: Policy
  name: Suspicious policy change
  orgId: 1
- folder: Policy
  name: Suspicious Cluster Compliance Status Change
  orgId: 1
- folder: Custom
  name: Suspicious policy change
  orgId: 1
- folder: Custom
  name: Suspicious Cluster Compliance Status Change
  orgId: 1
policies:
- orgId: 1
  receiver: alerts-cu-webhook`),
			b: []byte(`
apiVersion: 1
contactPoints:
- name: alerts-cu-webhook
  orgId: 1
  receivers:
  - disableResolveMessage: false
    type: email
    uid: 4e3bfe25-00cf-4173-b02b-16f077e539da
groups:
- folder: Policy
  name: Suspicious policy change
  orgId: 1
- folder: Policy
  name: Suspicious Cluster Compliance Status Change
  orgId: 1
- folder: Custom
  name: Suspicious policy change
  orgId: 1
- folder: Custom
  name: Suspicious Cluster Compliance Status Change
  orgId: 1
policies:
- orgId: 1
  receiver: alerts-cu-webhook`),
			want:    true,
			wantErr: false,
		},
		{
			name: "two equal alert which has some fields",
			a: []byte(`
apiVersion: 1
groups:
- folder: Policy
  name: Suspicious policy change
  orgId: 1
- folder: Policy
  name: Suspicious Cluster Compliance Status Change
  orgId: 1
- folder: Custom
  name: Suspicious policy change
  orgId: 1
- folder: Custom
  name: Suspicious Cluster Compliance Status Change
  orgId: 1`),
			b: []byte(`
apiVersion: 1
groups:
- folder: Policy
  name: Suspicious policy change
  orgId: 1
- folder: Policy
  name: Suspicious Cluster Compliance Status Change
  orgId: 1
- folder: Custom
  name: Suspicious policy change
  orgId: 1
- folder: Custom
  name: Suspicious Cluster Compliance Status Change
  orgId: 1`),
			want:    true,
			wantErr: false,
		},
		{
			name: "error equal alert",
			a: []byte(`
apiVersion: 1
groups:
- folder: Policy
	name: Suspicious policy change
	orgId: 1
- folder: Policy
  name: Suspicious Cluster Compliance Status Change
  orgId: 1
- folder: Custom
  name: Suspicious policy change
  orgId: 1
- folder: Custom
  name: Suspicious Cluster Compliance Status Change
  orgId: 1`),
			b: []byte(`
apiVersion: 1
groups:
- folder: Policy
  name: Suspicious policy change
	orgId: 1
- folder: Policy
  name: Suspicious Cluster Compliance Status Change
  orgId: 1
- folder: Custom
  name: Suspicious policy change
  orgId: 1
- folder: Custom
  name: Suspicious Cluster Compliance Status Change
  orgId: 1`),
			want:    false,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IsAlertGPCcountEqual(tt.a, tt.b)
			if (err != nil) != tt.wantErr {
				t.Errorf("isAlertCountEqual() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("isAlertCountEqual() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWaitGlobalHubReady(t *testing.T) {
	now := metav1.Now()
	tests := []struct {
		name     string
		mgh      []runtime.Object
		wantErr  bool
		returned bool
	}{
		{
			name: "no mgh status",
			mgh: []runtime.Object{
				&globalhubv1alpha4.MulticlusterGlobalHub{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "default",
					},
					Spec: globalhubv1alpha4.MulticlusterGlobalHubSpec{},
				},
			},
			returned: false,
		},
		{
			name: "no mgh instance",
			mgh:  []runtime.Object{},

			returned: false,
		},
		{
			name: "mgh is deleting",
			mgh: []runtime.Object{
				&globalhubv1alpha4.MulticlusterGlobalHub{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test",
						Namespace:         "default",
						DeletionTimestamp: &now,
						Finalizers: []string{
							"test",
						},
					},
					Spec: globalhubv1alpha4.MulticlusterGlobalHubSpec{},
				},
			},
			returned: false,
		},
		{
			name: "ready mgh",
			mgh: []runtime.Object{
				&globalhubv1alpha4.MulticlusterGlobalHub{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "default",
					},
					Spec: globalhubv1alpha4.MulticlusterGlobalHubSpec{},
					Status: globalhubv1alpha4.MulticlusterGlobalHubStatus{
						Conditions: []metav1.Condition{
							{
								Type:   condition.CONDITION_TYPE_GLOBALHUB_READY,
								Status: metav1.ConditionTrue,
							},
						},
					},
				},
			},
			returned: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := runtime.NewScheme()
			globalhubv1alpha4.AddToScheme(s)

			runtimeClient := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(tt.mgh...).Build()
			returned := false
			go func() {
				WaitGlobalHubReady(context.Background(), runtimeClient, 1*time.Second)
				returned = true
			}()
			time.Sleep(time.Second * 2)
			if returned != tt.returned {
				t.Errorf("name:%v, expect returned:%v, actual returned: %v", tt.name, tt.returned, returned)
			}
		})
	}
}
