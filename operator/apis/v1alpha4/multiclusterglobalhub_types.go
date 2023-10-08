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

package v1alpha4

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DataLayerType specifies the type of data layer that global hub stores and transports the data.
// +kubebuilder:validation:Enum:="largeScale"
type DataLayerType string

const (
	// // Native is a DataLayerType using kubernetes native storage and event subscription
	// Native DataLayerType = "native"
	// LargeScale is a DataLayerType using external high performance data storage and transport layer
	LargeScale DataLayerType = "largeScale"
)

// TransportFormatType specifies the type of data format based on kafka implementation.
// +kubebuilder:validation:Enum:="message";"cloudEvents"
type TransportFormatType string

const (
	KafkaMessage TransportFormatType = "message"
	CloudEvents  TransportFormatType = "cloudEvents"
)

// AvailabilityType ...
type AvailabilityType string

const (
	// HABasic stands up most app subscriptions with a replicaCount of 1
	HABasic AvailabilityType = "Basic"
	// HAHigh stands up most app subscriptions with a replicaCount of 2
	HAHigh AvailabilityType = "High"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName={mgh,mcgh}
// MulticlusterGlobalHub is the Schema for the multiclusterglobalhubs API
type MulticlusterGlobalHub struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:default={dataLayer: {postgres: {retention: "18m"}, kafka: {transportFormat: cloudEvents}}}
	// +kubebuilder:validation:Required
	Spec   MulticlusterGlobalHubSpec   `json:"spec,omitempty"`
	Status MulticlusterGlobalHubStatus `json:"status,omitempty"`
}

// MulticlusterGlobalHubSpec defines the desired state of MulticlusterGlobalHub
type MulticlusterGlobalHubSpec struct {

	// Specifies deployment replication for improved availability. Options are: Basic and High (default)
	// +kubebuilder:default:="High"
	AvailabilityConfig AvailabilityType `json:"availabilityConfig,omitempty"`
	// Pull policy of the multicluster global hub images
	// +optional
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// Pull secret of the multicluster global hub images
	// +optional
	ImagePullSecret string `json:"imagePullSecret,omitempty"`
	// Spec of NodeSelector
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// Tolerations causes all components to tolerate any taints.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// DataLayer can be configured to use a different data layer.
	// +kubebuilder:validation:Required
	// +kubebuilder:default={postgres: {retention: "18m"}, kafka: {transportFormat: cloudEvents}}
	DataLayer DataLayerConfig `json:"dataLayer"`
}

// DataLayerConfig is a discriminated union of data layer specific configuration.
type DataLayerConfig struct {
	// +kubebuilder:default={transportFormat: cloudEvents}
	Kafka KafkaConfig `json:"kafka,omitempty"`
	// +kubebuilder:default={retention: "18m"}
	Postgres PostgresConfig `json:"postgres,omitempty"`
}

// PostgresConfig defines the desired state of postgres
type PostgresConfig struct {
	// Retention is a duration string. Which defines how long to keep the data in the database.
	// Recommended minimum value is 1 month, default value is 18 months.
	// A duration string is a possibly signed sequence of decimal numbers, each with optional fraction and a unit suffix,
	// such as "1y6m". Valid time units are "m" and "y".
	// +kubebuilder:default:="18m"
	Retention string `json:"retention,omitempty"`

	// Specify the size for postgres persistent volume claim.
	// +optional
	StorageSize string `json:"storageSize,omitempty"`

	// Specify the storageClass for postgres persistent volume claim.
	// +optional
	StorageClass string `json:"storageClass,omitempty"`
}

// KafkaConfig defines the desired state of kafka
type KafkaConfig struct {
	// TransportFormat defines the transport format for kafka, which is either cloudEvents or kafka message
	// +kubebuilder:default:="cloudEvents"
	TransportFormat TransportFormatType `json:"transportFormat,omitempty"`
}

// MulticlusterGlobalHubStatus defines the observed state of MulticlusterGlobalHub
type MulticlusterGlobalHubStatus struct {
	// MulticlusterGlobalHubStatus defines the observed state of MulticlusterGlobalHub
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// MulticlusterGlobalHubList contains a list of MulticlusterGlobalHub
type MulticlusterGlobalHubList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MulticlusterGlobalHub `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MulticlusterGlobalHub{}, &MulticlusterGlobalHubList{})
}

func (mgh *MulticlusterGlobalHub) GetConditions() []metav1.Condition {
	return mgh.Status.Conditions
}

func (mgh *MulticlusterGlobalHub) SetConditions(conditions []metav1.Condition) {
	mgh.Status.Conditions = conditions
}
