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
	PhasePending      = "Pending"
	PhaseValidating   = "Validating"
	PhaseInitializing = "Initializing"
	PhaseDeploying    = "Deploying"
	PhaseRegistering  = "Registering"
	PhaseRollbacking  = "Rollbacking"
	PhaseCleaning     = "Cleaning"
	PhaseCompleted    = "Completed"
	PhaseFailed       = "Failed"
)

// Migration Condition Types
const (
	ConditionTypeStarted     = "MigrationStarted"
	ConditionTypeValidated   = "ResourceValidated"
	ConditionTypeInitialized = "ResourceInitialized"
	ConditionTypeRegistered  = "ClusterRegistered"
	ConditionTypeDeployed    = "ResourceDeployed"
	ConditionTypeRolledBack  = "ResourceRolledBack"
	ConditionTypeCleaned     = "ResourceCleaned"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName={mcm}
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phase",description="The overall status of the Migration"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
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

type ConfigMeta struct {
	// StageTimeout defines the timeout duration for each migration stage
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	StageTimeout *metav1.Duration `json:"stageTimeout,omitempty"`
}

// ManagedClusterMigrationSpec defines the desired state of managedclustermigration
// +kubebuilder:validation:XValidation:rule="has(self.includedManagedClustersPlacementRef) != has(self.includedManagedClusters)",message="exactly one of includedManagedClustersPlacementRef or includedManagedClusters must be specified"
type ManagedClusterMigrationSpec struct {
	// IncludedManagedClusters is a list of managed clusters that you want to migrate.
	// It is mutually exclusive with IncludedManagedClustersPlacementRef.
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +optional
	IncludedManagedClusters []string `json:"includedManagedClusters,omitempty"`

	// IncludedManagedClustersPlacementRef is used to point to a specific placement
	// that determines which managed clusters are included in a migration operation.
	// It is mutually exclusive with IncludedManagedClusters.
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Included Managed Clusters Placement"
	// +optional
	IncludedManagedClustersPlacementRef string `json:"includedManagedClustersPlacementRef,omitempty"`

	// From defines which hub cluster the managed clusters are from
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	From string `json:"from"`

	// To defines which hub cluster the managed clusters migrate to
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	To string `json:"to"`

	// SupportedConfigs defines additional configuration options for the migration
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	SupportedConfigs *ConfigMeta `json:"supportedConfigs,omitempty"`
}

// ManagedClusterMigrationStatus defines the observed state of managedclustermigration
type ManagedClusterMigrationStatus struct {
	// Phase represents the current phase of the migration
	// +kubebuilder:validation:Enum=Pending;Validating;Initializing;Deploying;Registering;Rollbacking;Cleaning;Completed;Failed
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
