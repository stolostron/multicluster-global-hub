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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/condition"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
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
	config.SetMGHNamespacedName(types.NamespacedName{
		Namespace: "default",
		Name:      "test",
	})
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
				&v1alpha4.MulticlusterGlobalHub{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "default",
					},
					Spec: v1alpha4.MulticlusterGlobalHubSpec{},
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
				&v1alpha4.MulticlusterGlobalHub{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test",
						Namespace:         "default",
						DeletionTimestamp: &now,
						Finalizers: []string{
							"test",
						},
					},
					Spec: v1alpha4.MulticlusterGlobalHubSpec{},
				},
			},
			returned: false,
		},
		{
			name: "ready mgh",
			mgh: []runtime.Object{
				&v1alpha4.MulticlusterGlobalHub{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "default",
					},
					Spec: v1alpha4.MulticlusterGlobalHubSpec{},
					Status: v1alpha4.MulticlusterGlobalHubStatus{
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
			v1alpha4.AddToScheme(s)
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

func Test_GetResources(t *testing.T) {
	customCPURequest := "1m"
	customCPULimit := "2m"
	customMemoryRequest := "1Mi"
	customMemoryLimit := "2Mi"

	tests := []struct {
		name          string
		component     string
		advanced      func(resReq *v1alpha4.ResourceRequirements) *v1alpha4.AdvancedConfig
		cpuRequest    string
		cpuLimit      string
		memoryRequest string
		memoryLimit   string
		custom        bool
	}{
		{
			name:          "Test Grafana with default values",
			component:     constants.Grafana,
			cpuRequest:    constants.GrafanaCPURequest,
			cpuLimit:      constants.GrafanaCPULimit,
			memoryRequest: constants.GrafanaMemoryRequest,
			memoryLimit:   constants.GrafanaMemoryLimit,
		},
		{
			name:      "Test Grafana with customized values",
			component: constants.Grafana,
			advanced: func(resReq *v1alpha4.ResourceRequirements) *v1alpha4.AdvancedConfig {
				return &v1alpha4.AdvancedConfig{
					Grafana: &v1alpha4.CommonSpec{
						Resources: resReq,
					},
				}
			},
			custom: true,
		},
		{
			name:          "Test Postgres with default values",
			component:     constants.Postgres,
			cpuRequest:    constants.PostgresCPURequest,
			cpuLimit:      "0",
			memoryRequest: constants.PostgresMemoryRequest,
			memoryLimit:   constants.PostgresMemoryLimit,
		},
		{
			name:      "Test Postgres with customized values",
			component: constants.Postgres,
			advanced: func(resReq *v1alpha4.ResourceRequirements) *v1alpha4.AdvancedConfig {
				return &v1alpha4.AdvancedConfig{
					Postgres: &v1alpha4.CommonSpec{
						Resources: resReq,
					},
				}
			},
			custom: true,
		},
		{
			name:          "Test Agent with default values",
			component:     constants.Agent,
			cpuRequest:    constants.AgentCPURequest,
			cpuLimit:      "0",
			memoryRequest: constants.AgentMemoryRequest,
			memoryLimit:   constants.AgentMemoryLimit,
		},
		{
			name:      "Test Agent with customized values",
			component: constants.Agent,
			advanced: func(resReq *v1alpha4.ResourceRequirements) *v1alpha4.AdvancedConfig {
				return &v1alpha4.AdvancedConfig{
					Agent: &v1alpha4.CommonSpec{
						Resources: resReq,
					},
				}
			},
			custom: true,
		},
		{
			name:          "Test Manager with default values",
			component:     constants.Manager,
			cpuRequest:    constants.ManagerCPURequest,
			cpuLimit:      "0",
			memoryRequest: constants.ManagerMemoryRequest,
			memoryLimit:   constants.ManagerMemoryLimit,
		},
		{
			name:      "Test Manager with customized values",
			component: constants.Manager,
			advanced: func(resReq *v1alpha4.ResourceRequirements) *v1alpha4.AdvancedConfig {
				return &v1alpha4.AdvancedConfig{
					Manager: &v1alpha4.CommonSpec{
						Resources: resReq,
					},
				}
			},
			custom: true,
		},
		{
			name:          "Test Kafka with default values",
			component:     constants.Kafka,
			cpuRequest:    constants.KafkaCPURequest,
			cpuLimit:      "0",
			memoryRequest: constants.KafkaMemoryRequest,
			memoryLimit:   constants.KafkaMemoryLimit,
		},
		{
			name:      "Test Kafka with customized values",
			component: constants.Kafka,
			advanced: func(resReq *v1alpha4.ResourceRequirements) *v1alpha4.AdvancedConfig {
				return &v1alpha4.AdvancedConfig{
					Kafka: &v1alpha4.CommonSpec{
						Resources: resReq,
					},
				}
			},
			custom: true,
		},
		{
			name:          "Test Zookeeper with default values",
			component:     constants.Zookeeper,
			cpuRequest:    constants.ZookeeperCPURequest,
			cpuLimit:      "0",
			memoryRequest: constants.ZookeeperMemoryRequest,
			memoryLimit:   constants.ZookeeperMemoryLimit,
		},
		{
			name:      "Test Zookeeper with customized values",
			component: constants.Zookeeper,
			advanced: func(resReq *v1alpha4.ResourceRequirements) *v1alpha4.AdvancedConfig {
				return &v1alpha4.AdvancedConfig{
					Zookeeper: &v1alpha4.CommonSpec{
						Resources: resReq,
					},
				}
			},
			custom: true,
		},
	}

	resReq := &v1alpha4.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceName(corev1.ResourceCPU):    resource.MustParse(customCPULimit),
			corev1.ResourceName(corev1.ResourceMemory): resource.MustParse(customMemoryLimit),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceName(corev1.ResourceMemory): resource.MustParse(customMemoryRequest),
			corev1.ResourceName(corev1.ResourceCPU):    resource.MustParse(customCPURequest),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var advanced *v1alpha4.AdvancedConfig
			if tt.custom {
				advanced = tt.advanced(resReq)
				tt.cpuRequest = customCPURequest
				tt.cpuLimit = customCPULimit
				tt.memoryLimit = customMemoryLimit
				tt.memoryRequest = customMemoryRequest
			}
			res := GetResources(tt.component, advanced)
			if res.Limits.Cpu().String() != tt.cpuLimit {
				t.Errorf("expect cpu: %v, actual cpu: %v", tt.cpuLimit, res.Limits.Cpu().String())
			}
			if res.Limits.Memory().String() != tt.memoryLimit {
				t.Errorf("expect memory: %v, actual memory: %v", tt.memoryLimit, res.Limits.Memory().String())
			}
			if res.Requests.Memory().String() != tt.memoryRequest {
				t.Errorf("expect memory: %v, actual memory: %v", tt.memoryRequest, res.Requests.Memory().String())
			}
			if res.Requests.Cpu().String() != tt.cpuRequest {
				t.Errorf("expect cpu: %v, actual cpu: %v", tt.cpuRequest, res.Requests.Cpu().String())
			}
		})
	}
}
