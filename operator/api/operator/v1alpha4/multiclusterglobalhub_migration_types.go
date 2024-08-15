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
	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// MulticlusterGlobalHubMigration is global hub resource that you can migrate the managed clusters from one hub to another
type MulticlusterGlobalHubMigration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec specifies the desired state of multicluster global hub
	// +kubebuilder:default={dataLayer: {postgres: {retention: "18m"}}}
	Spec MulticlusterGlobalHubMigrationSpec `json:"spec,omitempty"`
	// Status specifies the observed state of multicluster global hub
	Status MulticlusterGlobalHubMigrationStatus `json:"status,omitempty"`
}

// MulticlusterGlobalHubMigrationSpec defines the desired state of migration
type MulticlusterGlobalHubMigrationSpec struct {
	// velero option - IncludedNamespaces is a slice of namespace names to include objects
	// from. If empty, all namespaces are included.
	IncludedNamespaces []string `json:"includedNamespaces,omitempty"`

	// From defines which hub cluster are the managed clusters from
	From string `json:"from,omitempty"`

	// To defines which hub cluster does the managed clusters migrate to
	To string `json:"to,omitempty"`

	// BackupLocation define backup storage location for restore
	BackupLocation *BackupLocation `json:"backupLocation,omitempty"`
}

// BackupLocation defines the configuration for restore
type BackupLocation struct {
	// +optional
	Name string `json:"name,omitempty"`
	// +optional
	Velero *velero.BackupStorageLocationSpec `json:"velero,omitempty"`
	// +optional
	CloudStorage *CloudStorageLocation `json:"bucket,omitempty"`
}

// CloudStorageLocation defines BackupStorageLocation using bucket referenced by CloudStorage CR.
type CloudStorageLocation struct {
	CloudStorageRef corev1.LocalObjectReference `json:"cloudStorageRef"`

	// config is for provider-specific configuration fields.
	// +optional
	Config map[string]string `json:"config,omitempty"`

	// credential contains the credential information intended to be used with this location
	// +optional
	Credential *corev1.SecretKeySelector `json:"credential,omitempty"`

	// default indicates this location is the default backup storage location.
	// +optional
	Default bool `json:"default,omitempty"`

	// backupSyncPeriod defines how frequently to sync backup API objects from object storage. A value of 0 disables sync.
	// +optional
	// +nullable
	BackupSyncPeriod *metav1.Duration `json:"backupSyncPeriod,omitempty"`

	// Prefix and CACert are copied from velero/pkg/apis/v1/backupstoragelocation_types.go under ObjectStorageLocation

	// Prefix is the path inside a bucket to use for Velero storage. Optional.
	// +optional
	Prefix string `json:"prefix,omitempty"`

	// CACert defines a CA bundle to use when verifying TLS connections to the provider.
	// +optional
	CACert []byte `json:"caCert,omitempty"`
}

// MulticlusterGlobalHubMigrationStatus defines the observed state of migration
type MulticlusterGlobalHubMigrationStatus struct {
	// Conditions represents the latest available observations of the current state
	// +operator-sdk:csv:customresourcedefinitions:type=status
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// MulticlusterGlobalHubMigrationList contains a list of migration
type MulticlusterGlobalHubMigrationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MulticlusterGlobalHub `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MulticlusterGlobalHubMigration{}, &MulticlusterGlobalHubMigrationList{})
}
