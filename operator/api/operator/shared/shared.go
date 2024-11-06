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

// Package shared contains API Schema definitions for the operator API group
// +kubebuilder:object:generate=true
// +groupName=operator.open-cluster-management.io

package shared

import (
	corev1 "k8s.io/api/core/v1"
)

// ResourceRequirements copied from corev1.ResourceRequirements
// We do not need to support ResourceClaim
type ResourceRequirements struct {
	// Limits describes the maximum amount of compute resources allowed.
	// For more information, see: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// +optional
	Limits corev1.ResourceList `json:"limits,omitempty"`
	// Requests describes the minimum amount of compute resources required.
	// If requests are omitted for a container, it defaults to the specified limits.
	// If there are no specified limits, it defaults to an implementation-defined value.
	// For more information, see: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// +optional
	Requests corev1.ResourceList `json:"requests,omitempty"`
}
