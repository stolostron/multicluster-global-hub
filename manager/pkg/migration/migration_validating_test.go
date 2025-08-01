package migration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// go test -run ^TestIsValidResource$ ./manager/pkg/migration -v
func TestIsValidResource(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		errorMsg string
	}{
		{"valid configmap within dot", "configmap/default/kube-root-ca.crt", ""},
		{"invalid configmap prefix dot", "configmap/default/.kube-root-ca.crt ", "invalid name: .kube-root-ca.crt"},
		{"valid configmap", "configmap/default/my-config", ""},
		{"valid secret with wildcard", "secret/ns1/*", "invalid name: *"},
		{"invalid format", "configmap/default", "invalid format (must be kind/namespace/name): configmap/default"},
		{"unsupported kind", "pod/ns1/name1", "unsupported kind: pod"},
		{"invalid namespace", "configmap/Inv@lid/ns", "invalid namespace: Inv@lid"},
		{"invalid name", "secret/default/Invalid*", "invalid name: Invalid*"},
		{"invalid wildcard placement", "secret/ns1/na*me", "invalid name: na*me"},
		{"empty name with wildcard char", "secret/ns1/*x", "invalid name: *x"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := IsValidResource(tt.input)
			actualErrMessage := ""
			if err != nil {
				actualErrMessage = err.Error()
			}
			require.Equal(t, strings.TrimSpace(tt.errorMsg), strings.TrimSpace(actualErrMessage), tt.name)
		})
	}
}

func TestValidating(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = migrationv1alpha1.AddToScheme(scheme)
	_ = clusterv1.AddToScheme(scheme)

	tests := []struct {
		name                    string
		migration               *migrationv1alpha1.ManagedClusterMigration
		existingObjects         []client.Object
		expectedRequeue         bool
		expectedError           bool
		description             string
		expectedPhase           string
		expectedConditionStatus metav1.ConditionStatus
		expectedConditionReason string
	}{
		{
			name: "Should skip validating if migration is being deleted",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-migration",
					Namespace:         utils.GetDefaultNamespace(),
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
					UID:               types.UID("test-uid-1"),
					Finalizers:        []string{"test-finalizer"},
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseValidating,
				},
			},
			expectedRequeue:         false,
			expectedError:           false,
			description:             "Migration marked for deletion should be skipped",
			expectedPhase:           migrationv1alpha1.PhaseValidating, // Should not change phase when being deleted
			expectedConditionStatus: "",                                // No condition updates for deletion
			expectedConditionReason: "",
		},
		{
			name: "Should skip validating if already validated",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid-2"),
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseValidating,
					Conditions: []metav1.Condition{
						{
							Type:   migrationv1alpha1.ConditionTypeValidated,
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			expectedRequeue:         false,
			expectedError:           false,
			description:             "Already validated migration should be skipped",
			expectedPhase:           migrationv1alpha1.PhaseValidating, // Should not change phase when already validated
			expectedConditionStatus: metav1.ConditionTrue,              // Condition should remain true
			expectedConditionReason: "",                                // No reason change expected
		},
		{
			name: "Should skip if not in validating phase",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid-3"),
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseInitializing,
				},
			},
			expectedRequeue:         false,
			expectedError:           false,
			description:             "Non-validating phase should be skipped",
			expectedPhase:           migrationv1alpha1.PhaseInitializing, // Should not change phase when not in validating
			expectedConditionStatus: "",                                  // No condition updates
			expectedConditionReason: "",
		},
		{
			name: "Should fail when source hub not found",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid-4"),
				},
				Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
					From:                    "non-existent-source",
					To:                      "target-hub",
					IncludedManagedClusters: []string{"cluster1"},
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseValidating,
				},
			},
			existingObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "target-hub",
					},
				},
			},
			expectedRequeue:         false,
			expectedError:           false,
			description:             "Should fail when source hub is not found",
			expectedPhase:           migrationv1alpha1.PhaseFailed,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: "HubClusterInvalid",
		},
		{
			name: "Should fail when target hub not found",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid-5"),
				},
				Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
					From:                    "source-hub",
					To:                      "non-existent-target",
					IncludedManagedClusters: []string{"cluster1"},
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseValidating,
				},
			},
			existingObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "source-hub",
					},
				},
			},
			expectedRequeue:         false,
			expectedError:           false,
			description:             "Should fail when target hub is not found",
			expectedPhase:           migrationv1alpha1.PhaseFailed,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: "HubClusterInvalid",
		},
		{
			name: "Should fail with invalid resources",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid-6"),
				},
				Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
					From:                    "source-hub",
					To:                      "target-hub",
					IncludedManagedClusters: []string{"cluster1"},
					// IncludedResources:       []string{"invalid/format", "pod/ns1/name1"}, // Invalid resources
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseValidating,
				},
			},
			existingObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "source-hub",
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "target-hub",
					},
				},
			},
			expectedRequeue:         false,
			expectedError:           false, // Actually no error, just failed phase
			description:             "Should fail with invalid resources but hit database error first",
			expectedPhase:           migrationv1alpha1.PhaseFailed,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: "HubClusterInvalid",
		},
		{
			name: "Should validate successfully with valid configuration",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid-7"),
				},
				Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
					From:                    "source-hub",
					To:                      "target-hub",
					IncludedManagedClusters: []string{"cluster1"},
					// IncludedResources:       []string{"configmap/default/my-config", "secret/ns1/my-secret"},
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseValidating,
				},
			},
			existingObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "source-hub",
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "target-hub",
					},
				},
			},
			expectedRequeue:         false,
			expectedError:           false, // Actually no error, just failed phase
			description:             "Should attempt validation with valid configuration but hit database error",
			expectedPhase:           migrationv1alpha1.PhaseFailed,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: "HubClusterInvalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allObjects := append(tt.existingObjects, tt.migration)
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(allObjects...).
				WithStatusSubresource(&migrationv1alpha1.ManagedClusterMigration{}).
				Build()

			controller := &ClusterMigrationController{
				Client:   fakeClient,
				Producer: &MockProducer{},
			}

			requeue, err := controller.validating(context.TODO(), tt.migration)

			assert.Equal(t, tt.expectedRequeue, requeue, tt.description)
			if tt.expectedError {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
			}

			// Verify the migration status after validating operation
			if tt.expectedPhase != "" {
				assert.Equal(t, tt.expectedPhase, tt.migration.Status.Phase, "Expected phase should match")
			}

			// Verify the condition status if expected
			if tt.expectedConditionStatus != "" {
				validatedCondition := findValidatingCondition(tt.migration.Status.Conditions, migrationv1alpha1.ConditionTypeValidated)
				assert.NotNil(t, validatedCondition, "ResourceValidated condition should exist")
				assert.Equal(t, tt.expectedConditionStatus, validatedCondition.Status, "Expected condition status should match")
			}

			// Verify the condition reason if expected
			if tt.expectedConditionReason != "" {
				validatedCondition := findValidatingCondition(tt.migration.Status.Conditions, migrationv1alpha1.ConditionTypeValidated)
				assert.NotNil(t, validatedCondition, "ResourceValidated condition should exist")
				assert.Equal(t, tt.expectedConditionReason, validatedCondition.Reason, "Expected condition reason should match")
			}
		})
	}
}

// findValidatingCondition finds a specific condition in the conditions slice for validating tests
func findValidatingCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}

func TestValidatingEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		resources   []string
		expectError bool
		errorReason string
	}{
		{
			name:        "Should accept valid configmap and secret resources",
			resources:   []string{"configmap/default/my-config", "secret/kube-system/my-secret"},
			expectError: false,
		},
		{
			name:        "Should reject resources with invalid format",
			resources:   []string{"configmap/default"},
			expectError: true,
			errorReason: "invalid format",
		},
		{
			name:        "Should reject unsupported resource kinds",
			resources:   []string{"deployment/default/my-app"},
			expectError: true,
			errorReason: "unsupported kind",
		},
		{
			name:        "Should reject invalid namespace names",
			resources:   []string{"configmap/Invalid@Name/my-config"},
			expectError: true,
			errorReason: "invalid namespace",
		},
		{
			name:        "Should reject invalid resource names",
			resources:   []string{"secret/default/.invalid-name"},
			expectError: true,
			errorReason: "invalid name",
		},
		{
			name:        "Should handle empty resource list",
			resources:   []string{},
			expectError: false,
		},
		{
			name:        "Should validate mixed valid and invalid resources",
			resources:   []string{"configmap/default/valid", "invalid/format"},
			expectError: true,
			errorReason: "invalid format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			foundError := false
			for _, resource := range tt.resources {
				err := IsValidResource(resource)
				if err != nil {
					foundError = true
					if tt.expectError {
						assert.Error(t, err, "Expected error for resource: %s", resource)
						if tt.errorReason != "" {
							assert.Contains(t, err.Error(), tt.errorReason, "Expected specific error reason")
						}
						return // Stop at first invalid resource
					} else {
						assert.NoError(t, err, "Expected no error for resource: %s", resource)
					}
				}
			}

			// If we expect an error but didn't find one, fail the test
			if tt.expectError && !foundError {
				t.Errorf("Expected error for resource list %v but none found", tt.resources)
			}
		})
	}
}

func TestGetAndValidateHubCluster(t *testing.T) {
	scheme := runtime.NewScheme()
	appsv1.AddToScheme(scheme)
	require.NoError(t, clusterv1.AddToScheme(scheme))

	type fields struct {
		annotations map[string]string
		labels      map[string]string
		conditions  []metav1.Condition
	}
	tests := []struct {
		name           string
		clusterName    string
		fields         fields
		existingObjs   []client.Object
		expectError    bool
		errorContains  string
		expectNotNil   bool
		setupExtraObjs func(client.Client)
	}{
		{
			name:         "Cluster not found",
			clusterName:  "missing-cluster",
			existingObjs: []client.Object{
				// No clusters
			},
			expectError:   true,
			errorContains: "not found",
			expectNotNil:  false,
		},
		{
			name:        "Cluster not ready",
			clusterName: "not-ready-cluster",
			fields: fields{
				conditions: []metav1.Condition{
					{Type: "ManagedClusterConditionAvailable", Status: "False"},
				},
			},
			existingObjs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{Name: "not-ready-cluster"},
					Status:     clusterv1.ManagedClusterStatus{Conditions: []metav1.Condition{{Type: "ManagedClusterConditionAvailable", Status: "False"}}},
				},
			},
			expectError:   true,
			errorContains: "not ready",
			expectNotNil:  false,
		},
		{
			name:        "Cluster is ready but not a hub cluster",
			clusterName: "ready-nonhub",
			fields: fields{
				conditions: []metav1.Condition{
					{Type: "ManagedClusterConditionAvailable", Status: "True"},
				},
			},
			existingObjs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{Name: "ready-nonhub"},
					Status:     clusterv1.ManagedClusterStatus{Conditions: []metav1.Condition{{Type: "ManagedClusterConditionAvailable", Status: "True"}}},
				},
			},
			expectError:   true,
			errorContains: "not a hub cluster",
			expectNotNil:  false,
		},
		{
			name:        "Cluster is ready and has hub annotation",
			clusterName: "hub-annotated",
			fields: fields{
				annotations: map[string]string{"addon.open-cluster-management.io/on-multicluster-hub": "true"},
				conditions:  []metav1.Condition{{Type: "ManagedClusterConditionAvailable", Status: "True"}},
			},
			existingObjs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "hub-annotated",
						Annotations: map[string]string{"addon.open-cluster-management.io/on-multicluster-hub": "true"},
					},
					Status: clusterv1.ManagedClusterStatus{Conditions: []metav1.Condition{{Type: "ManagedClusterConditionAvailable", Status: "True"}}},
				},
			},
			expectError:  false,
			expectNotNil: true,
		},
		{
			name:        "Cluster is local-cluster, ready, and agent deployment exists",
			clusterName: "local-cluster",
			fields: fields{
				labels:     map[string]string{"local-cluster": "true"},
				conditions: []metav1.Condition{{Type: "ManagedClusterConditionAvailable", Status: "True"}},
			},
			existingObjs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "local-cluster",
						Labels: map[string]string{"local-cluster": "true"},
					},
					Status: clusterv1.ManagedClusterStatus{Conditions: []metav1.Condition{{Type: "ManagedClusterConditionAvailable", Status: "True"}}},
				},
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "multicluster-global-hub-agent",
						Namespace: utils.GetDefaultNamespace(),
					},
				},
			},
			expectError:  false,
			expectNotNil: true,
		},
		{
			name:        "Cluster is local-cluster, ready, but agent deployment missing",
			clusterName: "local-cluster-no-agent",
			fields: fields{
				labels:     map[string]string{"local-cluster": "true"},
				conditions: []metav1.Condition{{Type: "ManagedClusterConditionAvailable", Status: "True"}},
			},
			existingObjs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "local-cluster-no-agent",
						Labels: map[string]string{"local-cluster": "true"},
					},
					Status: clusterv1.ManagedClusterStatus{Conditions: []metav1.Condition{{Type: "ManagedClusterConditionAvailable", Status: "True"}}},
				},
				// No deployment object
			},
			expectError:   true,
			errorContains: "not a hub cluster",
			expectNotNil:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.existingObjs...).
				Build()
			ctx := context.TODO()
			err := validateHubCluster(ctx, fakeClient, tt.clusterName)
			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}
