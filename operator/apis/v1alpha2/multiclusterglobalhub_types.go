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

package v1alpha2

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

// MessageCompressionType specifies the compression type of transport message between global hub and regional hubs
// +kubebuilder:validation:Enum=gzip;no-op
type MessageCompressionType string

const (
	// GzipCompressType is an MessageCompressionType
	GzipCompressType MessageCompressionType = "gzip"
	// NoopCompressType is an MessageCompressionType
	NoopCompressType MessageCompressionType = "no-op"
)

// DataLayerType specifies the type of data layer that global hub stores and transports the data.
// +kubebuilder:validation:Enum:="native";"largeScale"
type DataLayerType string

const (
	// Native is a DataLayerType using kubernetes native storage and event subscription
	Native DataLayerType = "native"
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

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=mgh
// MulticlusterGlobalHub is the Schema for the multiclusterglobalhubs API
type MulticlusterGlobalHub struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MulticlusterGlobalHubSpec   `json:"spec,omitempty"`
	Status MulticlusterGlobalHubStatus `json:"status,omitempty"`
}

// MulticlusterGlobalHubSpec defines the desired state of MulticlusterGlobalHub
type MulticlusterGlobalHubSpec struct {
	// +kubebuilder:default:=full
	AggregationLevel AggregationLevel `json:"aggregationLevel,omitempty"` // full or minimal
	// +kubebuilder:default:=gzip
	MessageCompressionType MessageCompressionType `json:"messageCompressionType,omitempty"` // gzip or no-op
	// +kubebuilder:default:=true
	EnableLocalPolicies bool `json:"enableLocalPolicies,omitempty"`
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
	// native: use the native data layer (default).
	// largeScale: large scale data layer served by kafka and postgres.
	// +kubebuilder:validation:Required
	DataLayer *DataLayerConfig `json:"dataLayer"`
}

// DataLayerConfig is a discriminated union of data layer specific configuration.
// +union
type DataLayerConfig struct {
	// +unionDiscriminator
	// +kubebuilder:validation:Required
	Type DataLayerType `json:"type"`

	// Native may use a syncer to sync data from the regional hub cluster to the global hub cluster.
	// The data is stored in the global hub kubernetes api server backed by etcd.
	// This is not for a large scale environment.
	// +optional
	Native *NativeConfig `json:"native,omitempty"`
	// LargeScale is to use kafka as transport layer and use postgres as data layer
	// This is for a large scale environment.
	// +optional
	LargeScale *LargeScaleConfig `json:"largeScale,omitempty"`
}

// NativeConfig is the config of the native data layer
type NativeConfig struct{}

// LargeScaleConfig is the config of large scale data layer
type LargeScaleConfig struct {
	// +optional
	Kafka *KafkaConfig `json:"kafka,omitempty"`
	// +optional
	Postgres corev1.LocalObjectReference `json:"postgres,omitempty"`
}

// KafkaConfig defines the desired state of kafka
type KafkaConfig struct {
	// +optional
	Name string `json:"name,omitempty"`
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
