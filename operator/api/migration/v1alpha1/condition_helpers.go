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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FindMigrationCondition finds the MigrationCondition with the given type in the list.
// Returns nil if not found.
func FindMigrationCondition(conditions []MigrationCondition, condType string) *MigrationCondition {
	for i := range conditions {
		if conditions[i].Type == condType {
			return &conditions[i]
		}
	}
	return nil
}

// SetMigrationCondition sets a condition in the list, wrapping the standard metav1.Condition
// into a MigrationCondition. It updates LastUpdateTime whenever any content changes (status,
// reason, or message), and updates LastTransitionTime only when status changes (preserving
// the K8s convention). Returns true if anything changed.
func SetMigrationCondition(conditions *[]MigrationCondition, newCond metav1.Condition) bool {
	now := metav1.NewTime(time.Now())

	if conditions == nil {
		return false
	}

	existing := FindMigrationCondition(*conditions, newCond.Type)
	if existing == nil {
		// New condition: set both times to now
		if newCond.LastTransitionTime.IsZero() {
			newCond.LastTransitionTime = now
		}
		*conditions = append(*conditions, MigrationCondition{
			Condition:      newCond,
			LastUpdateTime: now,
		})
		return true
	}

	// Check if any content changed
	contentChanged := existing.Status != newCond.Status ||
		existing.Reason != newCond.Reason ||
		existing.Message != newCond.Message ||
		existing.ObservedGeneration != newCond.ObservedGeneration

	if !contentChanged {
		return false
	}

	// Status changed: update LastTransitionTime
	if existing.Status != newCond.Status {
		existing.LastTransitionTime = now
	}

	// Any content change: update LastUpdateTime
	existing.LastUpdateTime = now
	existing.Status = newCond.Status
	existing.Reason = newCond.Reason
	existing.Message = newCond.Message
	existing.ObservedGeneration = newCond.ObservedGeneration

	return true
}

// IsMigrationConditionTrue returns true if the condition with the given type is present
// and has status True.
func IsMigrationConditionTrue(conditions []MigrationCondition, condType string) bool {
	c := FindMigrationCondition(conditions, condType)
	if c == nil {
		return false
	}
	return c.Status == metav1.ConditionTrue
}

// RemoveMigrationCondition removes the condition with the given type from the list.
// Returns true if the condition was found and removed.
func RemoveMigrationCondition(conditions *[]MigrationCondition, condType string) bool {
	if conditions == nil {
		return false
	}
	for i := range *conditions {
		if (*conditions)[i].Type == condType {
			*conditions = append((*conditions)[:i], (*conditions)[i+1:]...)
			return true
		}
	}
	return false
}
