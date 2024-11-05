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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// MulticlusterGlobalHubAgentSpec defines the desired state of MulticlusterGlobalHubAgent
type MulticlusterGlobalHubAgentSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// ImagePullPolicy specifies the pull policy of the multicluster global hub images
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:com.tectonic.ui:imagePullPolicy"}
	// +optional
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// ImagePullSecret specifies the pull secret of the multicluster global hub images
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:io.kubernetes:Secret"}
	// +optional
	ImagePullSecret string `json:"imagePullSecret,omitempty"`
	// NodeSelector specifies the desired state of NodeSelector
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// Tolerations causes all components to tolerate any taints
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// Compute Resources required by this component
	// +optional
	Resources *ResourceRequirements `json:"resources,omitempty"`
}

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

// MulticlusterGlobalHubAgentStatus defines the observed state of MulticlusterGlobalHubAgent
type MulticlusterGlobalHubAgentStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// MulticlusterGlobalHubAgent is the Schema for the multiclusterglobalhubagents API
type MulticlusterGlobalHubAgent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MulticlusterGlobalHubAgentSpec   `json:"spec,omitempty"`
	Status MulticlusterGlobalHubAgentStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// MulticlusterGlobalHubAgentList contains a list of MulticlusterGlobalHubAgent
type MulticlusterGlobalHubAgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MulticlusterGlobalHubAgent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MulticlusterGlobalHubAgent{}, &MulticlusterGlobalHubAgentList{})
}
