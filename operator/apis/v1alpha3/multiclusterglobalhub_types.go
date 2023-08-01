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

package v1alpha3

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

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName={mgh,mcgh}
// MulticlusterGlobalHub is the Schema for the multiclusterglobalhubs API
type MulticlusterGlobalHub struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MulticlusterGlobalHubSpec   `json:"spec,omitempty"`
	Status MulticlusterGlobalHubStatus `json:"status,omitempty"`
}

// MulticlusterGlobalHubSpec defines the desired state of MulticlusterGlobalHub
type MulticlusterGlobalHubSpec struct {
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
	// DataLayer can be configured to use a different data layer, only support largeScale now.
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

	// // Native may use a syncer to sync data from the managed hub cluster to the global hub cluster.
	// // The data is stored in the global hub kubernetes api server backed by etcd.
	// // This is not for a large scale environment.
	// // +optional
	// Native *NativeConfig `json:"native,omitempty"`

	// LargeScale is to use kafka as transport layer and use postgres as data layer
	// This is for a large scale environment.
	// +optional
	LargeScale *LargeScaleConfig `json:"largeScale,omitempty"`
}

// // NativeConfig is the config of the native data layer
// type NativeConfig struct{}

// LargeScaleConfig is the config of large scale data layer
type LargeScaleConfig struct {
	// +optional
	Kafka *KafkaConfig `json:"kafka,omitempty"`
	// +optional
	Postgres *PostgresConfig `json:"postgres,omitempty"`
}

// PostgresConfig defines the desired state of postgres
type PostgresConfig struct {
	// Retention is a a duration string. Which defines how long to keep the data in the database.
	// Recommended minimum value is 1 month, default value is 18 months.
	// A duration string is a possibly signed sequence of decimal numbers, each with optional fraction and a unit suffix,
	// such as "2y4m". Valid time units are "h", "d", "m" and "y".
	// +kubebuilder:default:="18m"
	Retention string `json:"retention,omitempty"`
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
