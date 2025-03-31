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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Migration Phases
const (
	PhaseInitializing = "Initializing"
	PhaseValidating   = "Validating"
	PhaseMigrating    = "Migrating"
	PhaseCompleted    = "Completed"
	PhaseFailed       = "Failed"
)

// Condition Types
const (
	MigrationResourceInitialized = "ResourceInitialized"
	MigrationClusterRegistered   = "ClusterRegistered" // -> Phase: Migrating
	MigrationResourceDeployed    = "ResourceDeployed"  // -> Phase: Migrating
	MigrationCompleted           = "MigrationCompleted"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +operator-sdk:csv:customresourcedefinitions:resources={{Deployment,v1,multicluster-global-hub-manager}}
// ManagedClusterMigration is a global hub resource that allows you to migrate managed clusters from one hub to another
type ManagedClusterMigration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec specifies the desired state of managedclustermigration
	Spec ManagedClusterMigrationSpec `json:"spec,omitempty"`
	// Status specifies the observed state of managedclustermigration
	Status ManagedClusterMigrationStatus `json:"status,omitempty"`
}

// ManagedClusterMigrationSpec defines the desired state of managedclustermigration
type ManagedClusterMigrationSpec struct {
	// IncludedManagedClusters is a list of managed clusters that you want to migrate
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	IncludedManagedClusters []string `json:"includedManagedClusters,omitempty"`

	// IncludedResources is a list of resources that you want to migrate.
	// the format is kind.namespace/name. i.e.: configmap.multicluster-engine/cm1
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	IncludedResources []string `json:"includedResources,omitempty"`

	// From defines which hub cluster the managed clusters are from
	// +optional
	From string `json:"from,omitempty"`

	// To defines which hub cluster the managed clusters migrate to
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	To string `json:"to,omitempty"`
}

// ManagedClusterMigrationStatus defines the observed state of managedclustermigration
type ManagedClusterMigrationStatus struct {
	// Phase represents the current phase of the migration
	// +kubebuilder:validation:Enum=Initializing;Validating;Migrating;Completed;Failed
	// +operator-sdk:csv:customresourcedefinitions:type=status
	Phase string `json:"phase,omitempty"`

	// Conditions represents the latest available observations of the current state
	// +operator-sdk:csv:customresourcedefinitions:type=status
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// ManagedClusterMigrationList contains a list of migration
type ManagedClusterMigrationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ManagedClusterMigration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ManagedClusterMigration{}, &ManagedClusterMigrationList{})
}
