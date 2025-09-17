package migration

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
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
	_ = corev1.AddToScheme(scheme)

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
						Name:        "target-hub",
						Annotations: map[string]string{"addon.open-cluster-management.io/on-multicluster-hub": "true"},
					},
					Status: clusterv1.ManagedClusterStatus{
						Conditions: []metav1.Condition{
							{Type: "ManagedClusterConditionAvailable", Status: metav1.ConditionTrue},
						},
					},
				},
			},
			expectedRequeue:         false,
			expectedError:           true,
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
						Name:        "source-hub",
						Annotations: map[string]string{"addon.open-cluster-management.io/on-multicluster-hub": "true"},
					},
					Status: clusterv1.ManagedClusterStatus{
						Conditions: []metav1.Condition{
							{Type: "ManagedClusterConditionAvailable", Status: metav1.ConditionTrue},
						},
					},
				},
			},
			expectedRequeue:         false,
			expectedError:           true,
			description:             "Should fail when target hub is not found",
			expectedPhase:           migrationv1alpha1.PhaseFailed,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: "HubClusterInvalid",
		},
		{
			name: "Should wait for agent validation when clusters provided",
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
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseValidating,
				},
			},
			existingObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "source-hub",
						Annotations: map[string]string{"addon.open-cluster-management.io/on-multicluster-hub": "true"},
					},
					Status: clusterv1.ManagedClusterStatus{
						Conditions: []metav1.Condition{
							{Type: "ManagedClusterConditionAvailable", Status: metav1.ConditionTrue},
						},
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "target-hub",
						Annotations: map[string]string{"addon.open-cluster-management.io/on-multicluster-hub": "true"},
					},
					Status: clusterv1.ManagedClusterStatus{
						Conditions: []metav1.Condition{
							{Type: "ManagedClusterConditionAvailable", Status: metav1.ConditionTrue},
						},
					},
				},
			},
			expectedRequeue:         true, // Should requeue to wait for agent validation
			expectedError:           false,
			description:             "Should wait for agent validation when clusters are provided",
			expectedPhase:           migrationv1alpha1.PhaseValidating,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: "Waiting",
		},
		{
			name: "Should wait for agent when clusters provided and no previous status",
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
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseValidating,
				},
			},
			existingObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "source-hub",
						Annotations: map[string]string{"addon.open-cluster-management.io/on-multicluster-hub": "true"},
					},
					Status: clusterv1.ManagedClusterStatus{
						Conditions: []metav1.Condition{
							{Type: "ManagedClusterConditionAvailable", Status: metav1.ConditionTrue},
						},
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "target-hub",
						Annotations: map[string]string{"addon.open-cluster-management.io/on-multicluster-hub": "true"},
					},
					Status: clusterv1.ManagedClusterStatus{
						Conditions: []metav1.Condition{
							{Type: "ManagedClusterConditionAvailable", Status: metav1.ConditionTrue},
						},
					},
				},
			},
			expectedRequeue:         true, // Should requeue to wait for agent validation
			expectedError:           false,
			description:             "Should wait for agent when clusters provided and no previous status",
			expectedPhase:           migrationv1alpha1.PhaseValidating,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: "Waiting",
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
				Scheme:   scheme,
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
	require.NoError(t, appsv1.AddToScheme(scheme))
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
			errorContains: "is not a managed hub cluster",
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
			errorContains: "is not a managed hub cluster",
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

func TestGetClustersFromExistingConfigMap(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	namespace := utils.GetDefaultNamespace()
	configMapName := "test-migration"

	tests := []struct {
		name              string
		configMapName     string
		namespace         string
		existingConfigMap *corev1.ConfigMap
		expectedClusters  []string
		expectError       bool
		errorContains     string
	}{
		{
			name:             "ConfigMap does not exist",
			configMapName:    "non-existent",
			namespace:        namespace,
			expectedClusters: nil,
			expectError:      false,
		},
		{
			name:          "ConfigMap exists with valid clusters data",
			configMapName: configMapName,
			namespace:     namespace,
			existingConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapName,
					Namespace: namespace,
				},
				Data: map[string]string{
					"clusters": `["cluster1","cluster2","cluster3"]`,
				},
			},
			expectedClusters: []string{"cluster1", "cluster2", "cluster3"},
			expectError:      false,
		},
		{
			name:          "ConfigMap exists but no clusters key",
			configMapName: configMapName,
			namespace:     namespace,
			existingConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapName,
					Namespace: namespace,
				},
				Data: map[string]string{
					"other-key": "other-value",
				},
			},
			expectedClusters: nil,
			expectError:      false,
		},
		{
			name:          "ConfigMap exists with empty clusters array",
			configMapName: configMapName,
			namespace:     namespace,
			existingConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapName,
					Namespace: namespace,
				},
				Data: map[string]string{
					"clusters": `[]`,
				},
			},
			expectedClusters: []string{},
			expectError:      false,
		},
		{
			name:          "ConfigMap exists with invalid JSON",
			configMapName: configMapName,
			namespace:     namespace,
			existingConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapName,
					Namespace: namespace,
				},
				Data: map[string]string{
					"clusters": `invalid-json`,
				},
			},
			expectedClusters: nil,
			expectError:      true,
			errorContains:    "failed to unmarshal clusters",
		},
		{
			name:          "ConfigMap exists with single cluster",
			configMapName: configMapName,
			namespace:     namespace,
			existingConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapName,
					Namespace: namespace,
				},
				Data: map[string]string{
					"clusters": `["single-cluster"]`,
				},
			},
			expectedClusters: []string{"single-cluster"},
			expectError:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var existingObjects []client.Object
			if tt.existingConfigMap != nil {
				existingObjects = append(existingObjects, tt.existingConfigMap)
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(existingObjects...).
				Build()

			controller := &ClusterMigrationController{
				Client: fakeClient,
			}

			ctx := context.TODO()
			clusters, err := controller.getClustersFromExistingConfigMap(ctx, tt.configMapName, tt.namespace)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedClusters, clusters)
			}
		})
	}
}

// MockProducerWithError is a mock that always returns errors
type MockProducerWithError struct{}

func (m *MockProducerWithError) SendEvent(ctx context.Context, evt cloudevents.Event) error {
	return errors.New("failed to send migration event")
}

func (m *MockProducerWithError) Reconnect(config *transport.TransportInternalConfig, topic string) error {
	return nil
}

// TestGetMigrationClusters test removed - the getMigrationClusters method does not exist
// The functionality for getting clusters is tested through the validation tests above

// MockEventRecorder is a mock implementation of record.EventRecorder for testing
type MockEventRecorder struct {
	Events []MockEvent
}

type MockEvent struct {
	Object      runtime.Object
	EventType   string
	Reason      string
	MessageArgs []interface{}
}

func (m *MockEventRecorder) Eventf(object runtime.Object, eventType, reason, messageFmt string, args ...interface{}) {
	m.Events = append(m.Events, MockEvent{
		Object:      object,
		EventType:   eventType,
		Reason:      reason,
		MessageArgs: append([]interface{}{messageFmt}, args...),
	})
}

func (m *MockEventRecorder) Event(object runtime.Object, eventType, reason, message string) {
	m.Events = append(m.Events, MockEvent{
		Object:      object,
		EventType:   eventType,
		Reason:      reason,
		MessageArgs: []interface{}{message},
	})
}

func (m *MockEventRecorder) AnnotatedEventf(object runtime.Object, annotations map[string]string, eventType, reason, messageFmt string, args ...interface{}) {
	m.Eventf(object, eventType, reason, messageFmt, args...)
}

// TestHandleErrorList tests the handleErrorList function
func TestHandleErrorList(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = migrationv1alpha1.AddToScheme(scheme)

	tests := []struct {
		name                 string
		migration            *migrationv1alpha1.ManagedClusterMigration
		hub                  string
		phase                string
		clusterErrors        map[string]string
		expectedEventCount   int
		expectedEventType    string
		expectedEventReason  string
		setupClusterErrors   func(string, string, string, map[string]string)
		cleanupClusterErrors func()
	}{
		{
			name: "Should handle no cluster errors gracefully",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid-1"),
				},
			},
			hub:                "test-hub",
			phase:              migrationv1alpha1.PhaseValidating,
			clusterErrors:      nil,
			expectedEventCount: 0,
			setupClusterErrors: func(migrationId, hub, phase string, errors map[string]string) {
				// No setup needed for nil case
			},
			cleanupClusterErrors: func() {
				// No cleanup needed for nil case
			},
		},
		{
			name: "Should handle empty cluster errors map",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid-2"),
				},
			},
			hub:                "test-hub",
			phase:              migrationv1alpha1.PhaseValidating,
			clusterErrors:      map[string]string{},
			expectedEventCount: 0,
			setupClusterErrors: func(migrationId, hub, phase string, errors map[string]string) {
				AddMigrationStatus(migrationId)
				SetClusterErrorMessage(migrationId, hub, phase, errors)
			},
			cleanupClusterErrors: func() {
				RemoveMigrationStatus("test-uid-2")
			},
		},
		{
			name: "Should create events for single cluster error",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid-3"),
				},
			},
			hub:   "test-hub",
			phase: migrationv1alpha1.PhaseValidating,
			clusterErrors: map[string]string{
				"cluster1": "cluster validation failed: cluster not available",
			},
			expectedEventCount:  1,
			expectedEventType:   corev1.EventTypeWarning,
			expectedEventReason: "ValidationFailed",
			setupClusterErrors: func(migrationId, hub, phase string, errors map[string]string) {
				AddMigrationStatus(migrationId)
				SetClusterErrorMessage(migrationId, hub, phase, errors)
			},
			cleanupClusterErrors: func() {
				RemoveMigrationStatus("test-uid-3")
			},
		},
		{
			name: "Should create events for multiple cluster errors",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid-4"),
				},
			},
			hub:   "source-hub",
			phase: migrationv1alpha1.PhaseValidating,
			clusterErrors: map[string]string{
				"cluster1": "cluster validation failed: cluster not available",
				"cluster2": "cluster validation failed: hosted mode not supported",
				"cluster3": "cluster validation failed: local cluster cannot be migrated",
			},
			expectedEventCount:  3,
			expectedEventType:   corev1.EventTypeWarning,
			expectedEventReason: "ValidationFailed",
			setupClusterErrors: func(migrationId, hub, phase string, errors map[string]string) {
				AddMigrationStatus(migrationId)
				SetClusterErrorMessage(migrationId, hub, phase, errors)
			},
			cleanupClusterErrors: func() {
				RemoveMigrationStatus("test-uid-4")
			},
		},
		{
			name: "Should handle cluster errors in different phase",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid-5"),
				},
			},
			hub:   "target-hub",
			phase: migrationv1alpha1.PhaseInitializing,
			clusterErrors: map[string]string{
				"cluster1": "initialization failed: bootstrap secret creation failed",
				"cluster2": "initialization failed: klusterlet config creation failed",
			},
			expectedEventCount:  2,
			expectedEventType:   corev1.EventTypeWarning,
			expectedEventReason: "ValidationFailed",
			setupClusterErrors: func(migrationId, hub, phase string, errors map[string]string) {
				AddMigrationStatus(migrationId)
				SetClusterErrorMessage(migrationId, hub, phase, errors)
			},
			cleanupClusterErrors: func() {
				RemoveMigrationStatus("test-uid-5")
			},
		},
		{
			name: "Should handle special characters in error messages",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid-6"),
				},
			},
			hub:   "test-hub",
			phase: migrationv1alpha1.PhaseValidating,
			clusterErrors: map[string]string{
				"cluster-with-dash":       "validation failed: error with \"quotes\" and special chars: 100%",
				"cluster_with_underscore": "failed: error with 'single quotes' and unicode: ñáéíóú",
			},
			expectedEventCount:  2,
			expectedEventType:   corev1.EventTypeWarning,
			expectedEventReason: "ValidationFailed",
			setupClusterErrors: func(migrationId, hub, phase string, errors map[string]string) {
				AddMigrationStatus(migrationId)
				SetClusterErrorMessage(migrationId, hub, phase, errors)
			},
			cleanupClusterErrors: func() {
				RemoveMigrationStatus("test-uid-6")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockEventRecorder := &MockEventRecorder{
				Events: []MockEvent{},
			}

			controller := &ClusterMigrationController{
				EventRecorder: mockEventRecorder,
			}

			// Setup cluster errors using the test's setup function
			if tt.setupClusterErrors != nil {
				tt.setupClusterErrors(string(tt.migration.UID), tt.hub, tt.phase, tt.clusterErrors)
			}

			// Cleanup after test
			defer func() {
				if tt.cleanupClusterErrors != nil {
					tt.cleanupClusterErrors()
				}
			}()

			// Call the function under test
			controller.handleErrorList(tt.migration, tt.hub, tt.phase)

			// Verify the number of events generated
			assert.Equal(t, tt.expectedEventCount, len(mockEventRecorder.Events),
				"Expected %d events but got %d", tt.expectedEventCount, len(mockEventRecorder.Events))

			// Verify event properties if events were expected
			if tt.expectedEventCount > 0 {
				for i, event := range mockEventRecorder.Events {
					assert.Equal(t, tt.migration, event.Object,
						"Event %d should reference the migration object", i)
					assert.Equal(t, tt.expectedEventType, event.EventType,
						"Event %d should have correct event type", i)
					assert.Equal(t, tt.expectedEventReason, event.Reason,
						"Event %d should have correct reason", i)

					// Verify that the event message contains an error message from cluster errors
					if len(event.MessageArgs) > 0 {
						messageFormat := event.MessageArgs[0].(string)
						if len(event.MessageArgs) > 1 {
							// If there are args, it means Eventf was called, get the first arg as the actual message
							actualMessage := event.MessageArgs[1].(string)
							// Verify the actual message matches one of the expected cluster error messages
							found := false
							for _, expectedMsg := range tt.clusterErrors {
								if actualMessage == expectedMsg {
									found = true
									break
								}
							}
							assert.True(t, found, "Event message '%s' should match one of the cluster error messages", actualMessage)
						} else {
							// If no args, the message format itself should be in cluster errors
							found := false
							for _, expectedMsg := range tt.clusterErrors {
								if messageFormat == expectedMsg {
									found = true
									break
								}
							}
							assert.True(t, found, "Event message '%s' should match one of the cluster error messages", messageFormat)
						}
					}
				}
			}
		})
	}
}
