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

	shared "github.com/stolostron/multicluster-global-hub/operator/api/operator/shared"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName={mgha,mcgha}
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phase",description="The overall status of the MulticlusterGlobalHubAgent"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// MulticlusterGlobalHubAgentSpec defines the desired state of MulticlusterGlobalHubAgent
type MulticlusterGlobalHubAgentSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// ImagePullPolicy specifies the pull policy of the multicluster global hub agent image
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:com.tectonic.ui:imagePullPolicy"}
	// +optional
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// ImagePullSecret specifies the pull secret of the multicluster global hub agent image
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
	// Compute Resources required by the global hub agent
	// +optional
	Resources *shared.ResourceRequirements `json:"resources,omitempty"`
	// TransportConfigSecretName specifies the secret which is used to connect to the global hub transport.
	// You can fetch the information using the following commands from the global hub environment:
	// cat <<EOF >./kafka.yaml
	// bootstrap.server: $(kubectl get kafka kafka -n "multicluster-global-hub" -o jsonpath='{.status.listeners[1].bootstrapServers}')
	// topic.status: gh-status.global-hub
	// ca.crt: $(kubectl get kafka kafka -n "multicluster-global-hub" -o jsonpath='{.status.listeners[1].certificates[0]}' | { if [[ "$OSTYPE" == "darwin"* ]]; then base64 -b 0; else base64 -w 0; fi; })
	// client.crt: $(kubectl get secret global-hub-kafka-user -n "multicluster-global-hub" -o jsonpath='{.data.user\.crt}')
	// client.key: $(kubectl get secret global-hub-kafka-user -n "multicluster-global-hub" -o jsonpath='{.data.user\.key}')
	// EOF
	// You can create the secret `kubectl create secret generic transport-config -n "multicluster-global-hub" --from-file=kafka.yaml="./kafka.yaml"`
	// +kubebuilder:default=transport-config
	TransportConfigSecretName string `json:"transportConfigSecretName,omitempty"`
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
