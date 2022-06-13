// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package v1

import (
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

// HeartbeatIntervals defines heartbeat intervals for HoH and Leaf hub in seconds
type HeartbeatIntervals struct {
	HoHInSeconds     uint64 `default:"60" json:"hohInSeconds,omitempty"`
	LeafHubInSeconds uint64 `default:"60" json:"leafHubInSeconds,omitempty"`
}

// ConfigSpec defines the desired state of Config
type ConfigSpec struct {
	AggregationLevel    AggregationLevel   `json:"aggregationLevel,omitempty"` // full or minimal
	HeartbeatIntervals  HeartbeatIntervals `json:"heartbeatIntervals,omitempty"`
	EnableLocalPolicies bool               `json:"enableLocalPolicies,omitempty"`
}

// ConfigStatus defines the observed state of Config
type ConfigStatus struct{}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Config is the Schema for the configs API
type Config struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ConfigSpec   `json:"spec,omitempty"`
	Status ConfigStatus `json:"status,omitempty"`
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
