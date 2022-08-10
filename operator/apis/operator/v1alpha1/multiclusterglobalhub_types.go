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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AggregationLevel specifies the level of aggregation leaf hubs should do before sending the information
// +kubebuilder:validation:Enum=full;minimal
type AggregationLevel string

const (
	// Full is an AggregationLevel
	Full AggregationLevel = "full"
	// Minimal is an AggregationLevel
	Minimal AggregationLevel = "minimal"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +kubebuilder:resource:shortName=mgh
// MultiClusterGlobalHub is the Schema for the multiclusterglobalhubs API
type MultiClusterGlobalHub struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MultiClusterGlobalHubSpec   `json:"spec,omitempty"`
	Status MultiClusterGlobalHubStatus `json:"status,omitempty"`
}

// MultiClusterGlobalHubSpec defines the desired state of MultiClusterGlobalHub
type MultiClusterGlobalHubSpec struct {
	// +kubebuilder:default:=full
	AggregationLevel AggregationLevel `json:"aggregationLevel,omitempty"` // full or minimal
	// +kubebuilder:default:=true
	EnableLocalPolicies bool `json:"enableLocalPolicies,omitempty"`
	// +required
	Kafka corev1.LocalObjectReference `json:"kafka,omitempty"`
	// +required
	PostgreSQL corev1.LocalObjectReference `json:"postgreSQL,omitempty"`
}

// MultiClusterGlobalHubStatus defines the observed state of MultiClusterGlobalHub
type MultiClusterGlobalHubStatus struct {
	// MultiClusterGlobalHubStatus defines the observed state of MultiClusterGlobalHub
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
// MultiClusterGlobalHubList contains a list of MultiClusterGlobalHub
type MultiClusterGlobalHubList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MultiClusterGlobalHub `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MultiClusterGlobalHub{}, &MultiClusterGlobalHubList{})
}

func (mgh *MultiClusterGlobalHub) GetConditions() []metav1.Condition {
	return mgh.Status.Conditions
}

func (mgh *MultiClusterGlobalHub) SetConditions(conditions []metav1.Condition) {
	mgh.Status.Conditions = conditions
}
