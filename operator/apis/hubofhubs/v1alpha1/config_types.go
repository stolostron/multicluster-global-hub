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
// Config is the Schema for the configs API
type Config struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ConfigSpec   `json:"spec,omitempty"`
	Status ConfigStatus `json:"status,omitempty"`
}

// ConfigSpec defines the desired state of Config
type ConfigSpec struct {
	// +kubebuilder:default:=full
	AggregationLevel AggregationLevel `json:"aggregationLevel,omitempty"` // full or minimal
	// +kubebuilder:default:=true
	EnableLocalPolicies bool `json:"enableLocalPolicies,omitempty"`
	// +required
	Kafka corev1.LocalObjectReference `json:"kafka,omitempty"`
	// +required
	PostgreSQL corev1.LocalObjectReference `json:"postgreSQL,omitempty"`
}

// ConfigStatus defines the observed state of Config
type ConfigStatus struct {
	// ConfigStatus defines the observed state of Config
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
// ConfigList contains a list of Config
type ConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Config `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Config{}, &ConfigList{})
}

func (config *Config) GetConditions() []metav1.Condition {
	return config.Status.Conditions
}

func (config *Config) SetConditions(conditions []metav1.Condition) {
	config.Status.Conditions = conditions
}
