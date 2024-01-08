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
// +operator-sdk:csv:customresourcedefinitions:resources={{Deployment,v1,multicluster-global-hub-operator}}
// MulticlusterGlobalHub defines the configuration for an instance of the multiCluster global hub
type MulticlusterGlobalHub struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:default={dataLayer: {postgres: {retention: "18m"}}}
	Spec   MulticlusterGlobalHubSpec   `json:"spec,omitempty"`
	Status MulticlusterGlobalHubStatus `json:"status,omitempty"`
}

// MulticlusterGlobalHubSpec defines the desired state of MulticlusterGlobalHub
type MulticlusterGlobalHubSpec struct {
	// Specifies deployment replication for improved availability. Options are: Basic and High (default)
	// +kubebuilder:default:="High"
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	AvailabilityConfig AvailabilityType `json:"availabilityConfig,omitempty"`
	// Pull policy of the multicluster global hub images
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +optional
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// Pull secret of the multicluster global hub images
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +optional
	ImagePullSecret string `json:"imagePullSecret,omitempty"`
	// Spec of NodeSelector
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// Tolerations causes all components to tolerate any taints.
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// DataLayer can be configured to use a different data layer.
	// +kubebuilder:default={postgres: {retention: "18m"}}
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	DataLayer DataLayerConfig `json:"dataLayer"`
	// Advanced configurations for global hub
	// +optional
	AdvancedConfig *AdvancedConfig `json:"advanced,omitempty"`
}

type AdvancedConfig struct {
	// The spec of grafana
	// +optional
	Grafana *CommonSpec `json:"grafana,omitempty"`

	// The spec of kafka
	// +optional
	Kafka *CommonSpec `json:"kafka,omitempty"`

	// The spec of zookeeper
	// +optional
	Zookeeper *CommonSpec `json:"zookeeper,omitempty"`

	// The spec of postgres
	// +optional
	Postgres *CommonSpec `json:"postgres,omitempty"`

	// The spec of global hub manager
	// +optional
	Manager *CommonSpec `json:"manager,omitempty"`

	// The spec of global hub agent
	// +optional
	Agent *CommonSpec `json:"agent,omitempty"`
}

type CommonSpec struct {
	// Compute Resources required by this component.
	// +optional
	Resources *ResourceRequirements `json:"resources,omitempty"`
}

// ResourceRequirements copied from corev1.ResourceRequirements
// We do not need to support ResourceClaim
type ResourceRequirements struct {
	// Limits describes the maximum amount of compute resources allowed.
	// More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// +optional
	Limits corev1.ResourceList `json:"limits,omitempty"`
	// Requests describes the minimum amount of compute resources required.
	// If Requests is omitted for a container, it defaults to Limits if that is explicitly specified,
	// otherwise to an implementation-defined value.
	// More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// +optional
	Requests corev1.ResourceList `json:"requests,omitempty"`
}

// DataLayerConfig is a discriminated union of data layer specific configuration.
type DataLayerConfig struct {
	// +optional
	Kafka KafkaConfig `json:"kafka,omitempty"`
	// +kubebuilder:default={retention: "18m"}
	Postgres PostgresConfig `json:"postgres,omitempty"`
	// Specify the storageClass for storage.
	// +optional
	StorageClass string `json:"storageClass,omitempty"`
}

// PostgresConfig defines the desired state of postgres
type PostgresConfig struct {
	// Retention is a duration string. Which defines how long to keep the data in the database.
	// Recommended minimum value is 1 month, default value is 18 months.
	// A duration string is a possibly signed sequence of decimal numbers, each with optional fraction and a unit suffix,
	// such as "1y6m". Valid time units are "m" and "y".
	// +kubebuilder:default:="18m"
	Retention string `json:"retention,omitempty"`

	// Specify the size for storage.
	// +optional
	StorageSize string `json:"storageSize,omitempty"`
}

// KafkaConfig defines the desired state of kafka
type KafkaConfig struct {
	// Specify the size for storage.
	// +optional
	StorageSize string `json:"storageSize,omitempty"`
}

// MulticlusterGlobalHubStatus defines the observed state of MulticlusterGlobalHub
type MulticlusterGlobalHubStatus struct {
	// MulticlusterGlobalHubStatus defines the observed state of MulticlusterGlobalHub
	// +operator-sdk:csv:customresourcedefinitions:type=status
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
