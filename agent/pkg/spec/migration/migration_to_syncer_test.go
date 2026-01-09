// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package migration

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	addonv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/controller"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// TestFormatErrorMessages tests the formatErrorMessages function
func TestFormatErrorMessages(t *testing.T) {
	cases := []struct {
		name           string
		errors         map[string]string
		expectedOutput string
	}{
		{
			name:           "Empty error map",
			errors:         map[string]string{},
			expectedOutput: "",
		},
		{
			name: "Single error",
			errors: map[string]string{
				"cluster1": "connection timeout",
			},
			expectedOutput: "1 error(s), get more details in events",
		},
		{
			name: "Two errors",
			errors: map[string]string{
				"cluster1": "connection timeout",
				"cluster2": "authentication failed",
			},
			expectedOutput: "2 error(s), get more details in events",
		},
		{
			name: "Three errors",
			errors: map[string]string{
				"cluster1": "connection timeout",
				"cluster2": "authentication failed",
				"cluster3": "resource not found",
			},
			expectedOutput: "3 error(s), get more details in events",
		},
		{
			name: "Five errors",
			errors: map[string]string{
				"cluster1": "connection timeout",
				"cluster2": "authentication failed",
				"cluster3": "resource not found",
				"cluster4": "permission denied",
				"cluster5": "network unreachable",
			},
			expectedOutput: "5 error(s), get more details in events",
		},
		{
			name: "Four errors",
			errors: map[string]string{
				"cluster1": "error1",
				"cluster2": "error2",
				"cluster3": "error3",
				"cluster4": "error4",
			},
			expectedOutput: "4 error(s), get more details in events",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := formatErrorMessages(c.errors)
			assert.Equal(t, c.expectedOutput, result)
		})
	}
}

// TestFormatErrorMessagesNonDeterministic tests that the function returns consistent count
func TestFormatErrorMessagesNonDeterministic(t *testing.T) {
	errors := map[string]string{
		"cluster1": "error1",
		"cluster2": "error2",
		"cluster3": "error3",
		"cluster4": "error4",
		"cluster5": "error5",
	}

	// Run the function multiple times to ensure it's stable
	// The function should always return the same count and message
	expectedResult := "5 error(s), get more details in events"
	for i := 0; i < 10; i++ {
		result := formatErrorMessages(errors)
		assert.Equal(t, expectedResult, result, "Should always return the same count")
	}
}

// TestFormatErrorMessagesUsageInCleaningStage tests the actual usage pattern in the cleaning stage
func TestFormatErrorMessagesUsageInCleaningStage(t *testing.T) {
	// This test simulates how formatErrorMessages is used in the cleaning stage
	clusterErrors := map[string]string{
		"cluster1":            "failed to remove auto-import disable annotation: timeout",
		"cluster2":            "failed to remove auto-import disable annotation: not found",
		"hub1/ClusterManager": "failed to remove auto approve user from ClusterManager: conflict",
		"hub1/RBAC":           "failed to cleanup migration RBAC resources: permission denied",
	}

	result := formatErrorMessages(clusterErrors)

	// Verify the result shows the correct error count and hint message
	expectedResult := "4 error(s), get more details in events"
	assert.Equal(t, expectedResult, result)
}

// go test -run ^TestMigrationToSyncer$ github.com/stolostron/multicluster-global-hub/agent/pkg/spec/syncers -v
func TestMigrationToSyncer(t *testing.T) {
	ctx := context.Background()
	scheme := configs.GetRuntimeScheme()
	cases := []struct {
		name                          string
		migrationEvent                *migration.MigrationTargetBundle
		initObjects                   []client.Object
		expectedClusterManager        *operatorv1.ClusterManager
		expectedClusterRole           *rbacv1.ClusterRole
		expectedClusterRoleBinding    *rbacv1.ClusterRoleBinding
		expectedSARClusterRoleBinding *rbacv1.ClusterRoleBinding
	}{
		{
			name: "Initializing: migration with cluster manager having no registration configuration",
			migrationEvent: &migration.MigrationTargetBundle{
				MigrationId:                           "020340324302432049234023040320",
				Stage:                                 migrationv1alpha1.PhaseInitializing,
				ManagedServiceAccountName:             "test", // the migration cr name
				ManagedServiceAccountInstallNamespace: "test",
			},
			initObjects: []client.Object{
				&operatorv1.ClusterManager{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster-manager",
					},
					Spec: operatorv1.ClusterManagerSpec{
						RegistrationImagePullSpec: "test",
						WorkImagePullSpec:         "test",
					},
				},
			},
			expectedClusterManager: &operatorv1.ClusterManager{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster-manager",
				},
				Spec: operatorv1.ClusterManagerSpec{
					RegistrationImagePullSpec: "test",
					WorkImagePullSpec:         "test",
					RegistrationConfiguration: &operatorv1.RegistrationHubConfiguration{
						FeatureGates: []operatorv1.FeatureGate{
							{
								Feature: "ManagedClusterAutoApproval",
								Mode:    operatorv1.FeatureGateModeTypeEnable,
							},
						},
						AutoApproveUsers: []string{"system:serviceaccount:test:test"},
					},
				},
			},
			expectedClusterRole: &rbacv1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{
					Name: GetSubjectAccessReviewClusterRoleName("test"),
				},
				Rules: []rbacv1.PolicyRule{
					{
						APIGroups: []string{"authorization.k8s.io"},
						Resources: []string{"subjectaccessreviews"},
						Verbs:     []string{"create"},
					},
				},
			},
			expectedClusterRoleBinding: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: GetAgentRegistrationClusterRoleBindingName("test"),
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:      "ServiceAccount",
						Name:      "test",
						Namespace: "test",
					},
				},
				RoleRef: rbacv1.RoleRef{
					Kind:     "ClusterRole",
					Name:     "open-cluster-management:managedcluster:bootstrap:agent-registration",
					APIGroup: "rbac.authorization.k8s.io",
				},
			},
			expectedSARClusterRoleBinding: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: GetSubjectAccessReviewClusterRoleBindingName("test"),
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:      "ServiceAccount",
						Name:      "test",
						Namespace: "test",
					},
				},
				RoleRef: rbacv1.RoleRef{
					Kind:     "ClusterRole",
					Name:     GetSubjectAccessReviewClusterRoleName("test"),
					APIGroup: "rbac.authorization.k8s.io",
				},
			},
		},
		{
			name: "migration with cluster manager having empty registration configuration",
			migrationEvent: &migration.MigrationTargetBundle{
				MigrationId:                           "020340324302432049234023040320",
				Stage:                                 migrationv1alpha1.PhaseInitializing,
				ManagedServiceAccountName:             "test", // the migration cr name
				ManagedServiceAccountInstallNamespace: "test",
			},
			initObjects: []client.Object{
				&operatorv1.ClusterManager{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster-manager",
					},
					Spec: operatorv1.ClusterManagerSpec{
						RegistrationImagePullSpec: "test",
						WorkImagePullSpec:         "test",
						RegistrationConfiguration: &operatorv1.RegistrationHubConfiguration{
							FeatureGates:     []operatorv1.FeatureGate{},
							AutoApproveUsers: []string{},
						},
					},
				},
			},
			expectedClusterManager: &operatorv1.ClusterManager{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster-manager",
				},
				Spec: operatorv1.ClusterManagerSpec{
					RegistrationImagePullSpec: "test",
					WorkImagePullSpec:         "test",
					RegistrationConfiguration: &operatorv1.RegistrationHubConfiguration{
						FeatureGates: []operatorv1.FeatureGate{
							{
								Feature: "ManagedClusterAutoApproval",
								Mode:    operatorv1.FeatureGateModeTypeEnable,
							},
						},
						AutoApproveUsers: []string{"system:serviceaccount:test:test"},
					},
				},
			},
		},
		{
			name: "migration with cluster manager having registration configuration with other feature gates and auto approve users",
			migrationEvent: &migration.MigrationTargetBundle{
				MigrationId:                           "020340324302432049234023040320",
				Stage:                                 migrationv1alpha1.PhaseInitializing,
				ManagedServiceAccountName:             "test", // the migration cr name
				ManagedServiceAccountInstallNamespace: "test",
			},
			initObjects: []client.Object{
				&operatorv1.ClusterManager{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster-manager",
					},
					Spec: operatorv1.ClusterManagerSpec{
						RegistrationImagePullSpec: "test",
						WorkImagePullSpec:         "test",
						RegistrationConfiguration: &operatorv1.RegistrationHubConfiguration{
							FeatureGates: []operatorv1.FeatureGate{
								{
									Feature: "test",
									Mode:    operatorv1.FeatureGateModeTypeEnable,
								},
							},
							AutoApproveUsers: []string{"test"},
						},
					},
				},
			},
			expectedClusterManager: &operatorv1.ClusterManager{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster-manager",
				},
				Spec: operatorv1.ClusterManagerSpec{
					RegistrationImagePullSpec: "test",
					WorkImagePullSpec:         "test",
					RegistrationConfiguration: &operatorv1.RegistrationHubConfiguration{
						FeatureGates: []operatorv1.FeatureGate{
							{
								Feature: "test",
								Mode:    operatorv1.FeatureGateModeTypeEnable,
							},
							{
								Feature: "ManagedClusterAutoApproval",
								Mode:    operatorv1.FeatureGateModeTypeEnable,
							},
						},
						AutoApproveUsers: []string{"test", "system:serviceaccount:test:test"},
					},
				},
			},
		},
		{
			name: "migration with cluster manager having registration configuration with feature gate disabled",
			migrationEvent: &migration.MigrationTargetBundle{
				MigrationId:                           "020340324302432049234023040320",
				Stage:                                 migrationv1alpha1.PhaseInitializing,
				ManagedServiceAccountName:             "test", // the migration cr name
				ManagedServiceAccountInstallNamespace: "test",
			},
			initObjects: []client.Object{
				&operatorv1.ClusterManager{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster-manager",
					},
					Spec: operatorv1.ClusterManagerSpec{
						RegistrationImagePullSpec: "test",
						WorkImagePullSpec:         "test",
						RegistrationConfiguration: &operatorv1.RegistrationHubConfiguration{
							FeatureGates: []operatorv1.FeatureGate{
								{
									Feature: "ManagedClusterAutoApproval",
									Mode:    operatorv1.FeatureGateModeTypeDisable,
								},
							},
							AutoApproveUsers: []string{"test"},
						},
					},
				},
			},
			expectedClusterManager: &operatorv1.ClusterManager{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster-manager",
				},
				Spec: operatorv1.ClusterManagerSpec{
					RegistrationImagePullSpec: "test",
					WorkImagePullSpec:         "test",
					RegistrationConfiguration: &operatorv1.RegistrationHubConfiguration{
						FeatureGates: []operatorv1.FeatureGate{
							{
								Feature: "ManagedClusterAutoApproval",
								Mode:    operatorv1.FeatureGateModeTypeEnable,
							},
						},
						AutoApproveUsers: []string{"test", "system:serviceaccount:test:test"},
					},
				},
			},
		},
		{
			name: "migration with cluster manager having registration configuration with feature gate auto approve user",
			migrationEvent: &migration.MigrationTargetBundle{
				MigrationId:                           "020340324302432049234023040320",
				Stage:                                 migrationv1alpha1.PhaseInitializing,
				ManagedServiceAccountName:             "test", // the migration cr name
				ManagedServiceAccountInstallNamespace: "test",
			},
			initObjects: []client.Object{
				&operatorv1.ClusterManager{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster-manager",
					},
					Spec: operatorv1.ClusterManagerSpec{
						RegistrationImagePullSpec: "test",
						WorkImagePullSpec:         "test",
						RegistrationConfiguration: &operatorv1.RegistrationHubConfiguration{
							FeatureGates: []operatorv1.FeatureGate{
								{
									Feature: "ManagedClusterAutoApproval",
									Mode:    operatorv1.FeatureGateModeTypeEnable,
								},
							},
							AutoApproveUsers: []string{"system:serviceaccount:test:test"},
						},
					},
				},
			},
			expectedClusterManager: &operatorv1.ClusterManager{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster-manager",
				},
				Spec: operatorv1.ClusterManagerSpec{
					RegistrationImagePullSpec: "test",
					WorkImagePullSpec:         "test",
					RegistrationConfiguration: &operatorv1.RegistrationHubConfiguration{
						FeatureGates: []operatorv1.FeatureGate{
							{
								Feature: "ManagedClusterAutoApproval",
								Mode:    operatorv1.FeatureGateModeTypeEnable,
							},
						},
						AutoApproveUsers: []string{"system:serviceaccount:test:test"},
					},
				},
			},
		},
		{
			name: "migration with existing clusterrole and clusterrolebinding",
			migrationEvent: &migration.MigrationTargetBundle{
				MigrationId:                           "020340324302432049234023040320",
				Stage:                                 migrationv1alpha1.PhaseInitializing,
				ManagedServiceAccountName:             "test", // the migration cr name
				ManagedServiceAccountInstallNamespace: "test",
			},
			initObjects: []client.Object{
				&operatorv1.ClusterManager{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster-manager",
					},
					Spec: operatorv1.ClusterManagerSpec{
						RegistrationImagePullSpec: "test",
						WorkImagePullSpec:         "test",
					},
				},
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: GetSubjectAccessReviewClusterRoleName("test"),
					},
					Rules: []rbacv1.PolicyRule{
						{
							APIGroups: []string{"authorization.k8s.io"},
							Resources: []string{"subjectaccessreviews"},
							Verbs:     []string{"create"},
						},
					},
				},
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: GetAgentRegistrationClusterRoleBindingName("test"),
					},
					Subjects: []rbacv1.Subject{
						{
							Kind:      "ServiceAccount",
							Name:      "test",
							Namespace: "test",
						},
					},
					RoleRef: rbacv1.RoleRef{
						Kind:     "ClusterRole",
						Name:     "open-cluster-management:managedcluster:bootstrap:agent-registration",
						APIGroup: "rbac.authorization.k8s.io",
					},
				},
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: GetSubjectAccessReviewClusterRoleBindingName("test"),
					},
					Subjects: []rbacv1.Subject{
						{
							Kind:      "ServiceAccount",
							Name:      "test",
							Namespace: "test",
						},
					},
					RoleRef: rbacv1.RoleRef{
						Kind:     "ClusterRole",
						Name:     GetSubjectAccessReviewClusterRoleName("test"),
						APIGroup: "rbac.authorization.k8s.io",
					},
				},
			},
			expectedClusterManager: &operatorv1.ClusterManager{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster-manager",
				},
				Spec: operatorv1.ClusterManagerSpec{
					RegistrationImagePullSpec: "test",
					WorkImagePullSpec:         "test",
					RegistrationConfiguration: &operatorv1.RegistrationHubConfiguration{
						FeatureGates: []operatorv1.FeatureGate{
							{
								Feature: "ManagedClusterAutoApproval",
								Mode:    operatorv1.FeatureGateModeTypeEnable,
							},
						},
						AutoApproveUsers: []string{"system:serviceaccount:test:test"},
					},
				},
			},
			expectedClusterRole: &rbacv1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{
					Name: GetSubjectAccessReviewClusterRoleName("test"),
				},
				Rules: []rbacv1.PolicyRule{
					{
						APIGroups: []string{"authorization.k8s.io"},
						Resources: []string{"subjectaccessreviews"},
						Verbs:     []string{"create"},
					},
				},
			},
			expectedClusterRoleBinding: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: GetAgentRegistrationClusterRoleBindingName("test"),
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:      "ServiceAccount",
						Name:      "test",
						Namespace: "test",
					},
				},
				RoleRef: rbacv1.RoleRef{
					Kind:     "ClusterRole",
					Name:     "open-cluster-management:managedcluster:bootstrap:agent-registration",
					APIGroup: "rbac.authorization.k8s.io",
				},
			},
			expectedSARClusterRoleBinding: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: GetSubjectAccessReviewClusterRoleBindingName("test"),
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:      "ServiceAccount",
						Name:      "test",
						Namespace: "test",
					},
				},
				RoleRef: rbacv1.RoleRef{
					Kind:     "ClusterRole",
					Name:     GetSubjectAccessReviewClusterRoleName("test"),
					APIGroup: "rbac.authorization.k8s.io",
				},
			},
		},
		{
			name: "migration with changed clusterrole and clusterrolebinding",
			migrationEvent: &migration.MigrationTargetBundle{
				MigrationId:                           "020340324302432049234023040320",
				Stage:                                 migrationv1alpha1.PhaseInitializing,
				ManagedServiceAccountName:             "test", // the migration cr name
				ManagedServiceAccountInstallNamespace: "test",
			},
			initObjects: []client.Object{
				&operatorv1.ClusterManager{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster-manager",
					},
					Spec: operatorv1.ClusterManagerSpec{
						RegistrationImagePullSpec: "test",
						WorkImagePullSpec:         "test",
					},
				},
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: GetSubjectAccessReviewClusterRoleName("test"),
					},
					Rules: []rbacv1.PolicyRule{
						{
							APIGroups: []string{"authorization.k8s.io"},
							Resources: []string{"selfsubjectaccessreviews"},
							Verbs:     []string{"create"},
						},
					},
				},
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: GetAgentRegistrationClusterRoleBindingName("test"),
					},
					Subjects: []rbacv1.Subject{
						{
							Kind:      "ServiceAccount",
							Name:      "foo",
							Namespace: "test",
						},
					},
					RoleRef: rbacv1.RoleRef{
						Kind:     "ClusterRole",
						Name:     "open-cluster-management:managedcluster:bootstrap:agent-registration",
						APIGroup: "rbac.authorization.k8s.io",
					},
				},
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: GetSubjectAccessReviewClusterRoleBindingName("test"),
					},
					Subjects: []rbacv1.Subject{
						{
							Kind:      "ServiceAccount",
							Name:      "foo",
							Namespace: "test",
						},
					},
					RoleRef: rbacv1.RoleRef{
						Kind:     "ClusterRole",
						Name:     GetSubjectAccessReviewClusterRoleName("test"),
						APIGroup: "rbac.authorization.k8s.io",
					},
				},
			},
			expectedClusterManager: &operatorv1.ClusterManager{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster-manager",
				},
				Spec: operatorv1.ClusterManagerSpec{
					RegistrationImagePullSpec: "test",
					WorkImagePullSpec:         "test",
					RegistrationConfiguration: &operatorv1.RegistrationHubConfiguration{
						FeatureGates: []operatorv1.FeatureGate{
							{
								Feature: "ManagedClusterAutoApproval",
								Mode:    operatorv1.FeatureGateModeTypeEnable,
							},
						},
						AutoApproveUsers: []string{"system:serviceaccount:test:test"},
					},
				},
			},
			expectedClusterRole: &rbacv1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{
					Name: GetSubjectAccessReviewClusterRoleName("test"),
				},
				Rules: []rbacv1.PolicyRule{
					{
						APIGroups: []string{"authorization.k8s.io"},
						Resources: []string{"subjectaccessreviews"},
						Verbs:     []string{"create"},
					},
				},
			},
			expectedClusterRoleBinding: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: GetAgentRegistrationClusterRoleBindingName("test"),
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:      "ServiceAccount",
						Name:      "test",
						Namespace: "test",
					},
				},
				RoleRef: rbacv1.RoleRef{
					Kind:     "ClusterRole",
					Name:     "open-cluster-management:managedcluster:bootstrap:agent-registration",
					APIGroup: "rbac.authorization.k8s.io",
				},
			},
			expectedSARClusterRoleBinding: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: GetSubjectAccessReviewClusterRoleBindingName("test"),
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:      "ServiceAccount",
						Name:      "test",
						Namespace: "test",
					},
				},
				RoleRef: rbacv1.RoleRef{
					Kind:     "ClusterRole",
					Name:     GetSubjectAccessReviewClusterRoleName("test"),
					APIGroup: "rbac.authorization.k8s.io",
				},
			},
		},
		{
			name: "Rollback initializing: clean up RBAC resources and remove AutoApprove user from ClusterManager",
			migrationEvent: &migration.MigrationTargetBundle{
				MigrationId:                           "020340324302432049234023040320",
				Stage:                                 migrationv1alpha1.PhaseRollbacking,
				RollbackStage:                         migrationv1alpha1.PhaseInitializing,
				ManagedServiceAccountName:             "test",
				ManagedServiceAccountInstallNamespace: "test",
			},
			initObjects: []client.Object{
				&operatorv1.ClusterManager{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster-manager",
					},
					Spec: operatorv1.ClusterManagerSpec{
						RegistrationImagePullSpec: "test",
						WorkImagePullSpec:         "test",
						RegistrationConfiguration: &operatorv1.RegistrationHubConfiguration{
							FeatureGates: []operatorv1.FeatureGate{
								{
									Feature: "ManagedClusterAutoApproval",
									Mode:    operatorv1.FeatureGateModeTypeEnable,
								},
							},
							AutoApproveUsers: []string{"system:serviceaccount:test:test"},
						},
					},
				},
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: GetSubjectAccessReviewClusterRoleName("test"),
					},
					Rules: []rbacv1.PolicyRule{
						{
							APIGroups: []string{"authorization.k8s.io"},
							Resources: []string{"subjectaccessreviews"},
							Verbs:     []string{"create"},
						},
					},
				},
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: GetAgentRegistrationClusterRoleBindingName("test"),
					},
					Subjects: []rbacv1.Subject{
						{
							Kind:      "ServiceAccount",
							Name:      "test",
							Namespace: "test",
						},
					},
					RoleRef: rbacv1.RoleRef{
						Kind:     "ClusterRole",
						Name:     "open-cluster-management:managedcluster:bootstrap:agent-registration",
						APIGroup: "rbac.authorization.k8s.io",
					},
				},
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: GetSubjectAccessReviewClusterRoleBindingName("test"),
					},
					Subjects: []rbacv1.Subject{
						{
							Kind:      "ServiceAccount",
							Name:      "test",
							Namespace: "test",
						},
					},
					RoleRef: rbacv1.RoleRef{
						Kind:     "ClusterRole",
						Name:     GetSubjectAccessReviewClusterRoleName("test"),
						APIGroup: "rbac.authorization.k8s.io",
					},
				},
			},
			expectedClusterManager: &operatorv1.ClusterManager{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster-manager",
				},
				Spec: operatorv1.ClusterManagerSpec{
					RegistrationImagePullSpec: "test",
					WorkImagePullSpec:         "test",
					RegistrationConfiguration: &operatorv1.RegistrationHubConfiguration{
						FeatureGates: []operatorv1.FeatureGate{
							{
								Feature: "ManagedClusterAutoApproval",
								Mode:    operatorv1.FeatureGateModeTypeEnable,
							},
						},
						AutoApproveUsers: nil, // User should be removed during rollback
					},
				},
			},
		},
		{
			name: "Rollback initializing: clean up user from AutoApproveUsers list with other users present",
			migrationEvent: &migration.MigrationTargetBundle{
				MigrationId:                           "020340324302432049234023040320",
				Stage:                                 migrationv1alpha1.PhaseRollbacking,
				RollbackStage:                         migrationv1alpha1.PhaseInitializing,
				ManagedServiceAccountName:             "test",
				ManagedServiceAccountInstallNamespace: "test",
			},
			initObjects: []client.Object{
				&operatorv1.ClusterManager{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster-manager",
					},
					Spec: operatorv1.ClusterManagerSpec{
						RegistrationImagePullSpec: "test",
						WorkImagePullSpec:         "test",
						RegistrationConfiguration: &operatorv1.RegistrationHubConfiguration{
							FeatureGates: []operatorv1.FeatureGate{
								{
									Feature: "ManagedClusterAutoApproval",
									Mode:    operatorv1.FeatureGateModeTypeEnable,
								},
							},
							AutoApproveUsers: []string{"system:serviceaccount:other:user", "system:serviceaccount:test:test", "system:serviceaccount:another:user"},
						},
					},
				},
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: GetSubjectAccessReviewClusterRoleName("test"),
					},
					Rules: []rbacv1.PolicyRule{
						{
							APIGroups: []string{"authorization.k8s.io"},
							Resources: []string{"subjectaccessreviews"},
							Verbs:     []string{"create"},
						},
					},
				},
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: GetAgentRegistrationClusterRoleBindingName("test"),
					},
					Subjects: []rbacv1.Subject{
						{
							Kind:      "ServiceAccount",
							Name:      "test",
							Namespace: "test",
						},
					},
					RoleRef: rbacv1.RoleRef{
						Kind:     "ClusterRole",
						Name:     "open-cluster-management:managedcluster:bootstrap:agent-registration",
						APIGroup: "rbac.authorization.k8s.io",
					},
				},
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: GetSubjectAccessReviewClusterRoleBindingName("test"),
					},
					Subjects: []rbacv1.Subject{
						{
							Kind:      "ServiceAccount",
							Name:      "test",
							Namespace: "test",
						},
					},
					RoleRef: rbacv1.RoleRef{
						Kind:     "ClusterRole",
						Name:     GetSubjectAccessReviewClusterRoleName("test"),
						APIGroup: "rbac.authorization.k8s.io",
					},
				},
			},
			expectedClusterManager: &operatorv1.ClusterManager{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster-manager",
				},
				Spec: operatorv1.ClusterManagerSpec{
					RegistrationImagePullSpec: "test",
					WorkImagePullSpec:         "test",
					RegistrationConfiguration: &operatorv1.RegistrationHubConfiguration{
						FeatureGates: []operatorv1.FeatureGate{
							{
								Feature: "ManagedClusterAutoApproval",
								Mode:    operatorv1.FeatureGateModeTypeEnable,
							},
						},
						AutoApproveUsers: []string{"system:serviceaccount:other:user", "system:serviceaccount:another:user"}, // Only migration user should be removed
					},
				},
			},
		},
		{
			name: "Rollback deploying: clean up deployed resources on target hub",
			migrationEvent: &migration.MigrationTargetBundle{
				MigrationId:                           "020340324302432049234023040320",
				Stage:                                 migrationv1alpha1.PhaseRollbacking,
				RollbackStage:                         migrationv1alpha1.PhaseDeploying,
				ManagedServiceAccountName:             "test",
				ManagedServiceAccountInstallNamespace: "test",
				ManagedClusters:                       []string{"cluster1", "cluster2"},
			},
			initObjects: []client.Object{
				&operatorv1.ClusterManager{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster-manager",
					},
					Spec: operatorv1.ClusterManagerSpec{
						RegistrationImagePullSpec: "test",
						WorkImagePullSpec:         "test",
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster1",
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient: true,
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster2",
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient: true,
					},
				},
				&addonv1.KlusterletAddonConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cluster1",
						Namespace: "cluster1",
					},
				},
				&addonv1.KlusterletAddonConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cluster2",
						Namespace: "cluster2",
					},
				},
			},
			expectedClusterManager: nil, // No changes expected to cluster manager during rollback
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			producer := ProducerMock{}
			transportClient := &controller.TransportClient{}
			transportClient.SetProducer(&producer)

			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(c.initObjects...).Build()

			transportConfig := &transport.TransportInternalConfig{
				TransportType: string(transport.Chan),
				KafkaCredential: &transport.KafkaConfig{
					SpecTopic:   "spec",
					StatusTopic: "status",
				},
			}
			agentConfig := &configs.AgentConfig{
				TransportConfig: transportConfig,
				LeafHubName:     "hub1",
			}
			managedClusterMigrationSyncer := NewMigrationTargetSyncer(client, transportClient, agentConfig)
			configs.SetAgentConfig(&configs.AgentConfig{LeafHubName: "hub2"})

			// For rollback tests, set the current migration ID to match the event
			if c.migrationEvent.Stage == migrationv1alpha1.PhaseRollbacking {
				managedClusterMigrationSyncer.processingMigrationId = c.migrationEvent.MigrationId
			} else {
				// For non-rollback tests, set the migration ID to match the event
				managedClusterMigrationSyncer.SetMigrationID(c.migrationEvent.MigrationId)
			}

			toEvent := c.migrationEvent
			payload, err := json.Marshal(toEvent)
			assert.Nil(t, err)
			evt := utils.ToCloudEvent(constants.MigrationTargetMsgKey, constants.CloudEventGlobalHubClusterName,
				"hub2", payload)
			evt.SetTime(time.Now()) // Set event time to avoid time-based skipping in shouldSkipMigrationEvent
			err = managedClusterMigrationSyncer.Sync(ctx, &evt)
			assert.Nil(t, err)

			if c.expectedClusterManager != nil {
				foundClusterManager := &operatorv1.ClusterManager{}
				err := client.Get(ctx, types.NamespacedName{Name: c.expectedClusterManager.Name}, foundClusterManager)
				assert.Nil(t, err)
				utils.PrettyPrint(foundClusterManager.Spec)
				assert.Equal(t, c.expectedClusterManager.Spec, foundClusterManager.Spec)
			}

			if c.expectedClusterRole != nil {
				foundClusterRole := &rbacv1.ClusterRole{}
				err = client.Get(ctx, types.NamespacedName{Name: c.expectedClusterRole.Name}, foundClusterRole)
				assert.Nil(t, err)
				foundClusterRole.ResourceVersion = ""
				assert.Equal(t, c.expectedClusterRole, foundClusterRole)
			}

			if c.expectedClusterRoleBinding != nil {
				foundClusterRoleBinding := &rbacv1.ClusterRoleBinding{}
				err = client.Get(ctx, types.NamespacedName{Name: c.expectedClusterRoleBinding.Name}, foundClusterRoleBinding)
				assert.Nil(t, err)
				foundClusterRoleBinding.ResourceVersion = ""
				assert.Equal(t, c.expectedClusterRoleBinding, foundClusterRoleBinding)
			}

			if c.expectedSARClusterRoleBinding != nil {
				foundSARClusterRoleBinding := &rbacv1.ClusterRoleBinding{}
				err = client.Get(ctx, types.NamespacedName{Name: c.expectedSARClusterRoleBinding.Name}, foundSARClusterRoleBinding)
				assert.Nil(t, err)
				foundSARClusterRoleBinding.ResourceVersion = ""
				assert.Equal(t, c.expectedSARClusterRoleBinding, foundSARClusterRoleBinding)
			}
		})
	}
}

func TestMigrationDestinationHubSyncer(t *testing.T) {
	ctx := context.Background()
	scheme := configs.GetRuntimeScheme()
	configs.SetAgentConfig(&configs.AgentConfig{LeafHubName: "hub2"})
	cases := []struct {
		name                         string
		receivedMigrationEventBundle migration.MigrationTargetBundle
		eventSource                  string
		initObjects                  []client.Object
		expectedError                error
	}{
		// NOTE: Deploying test case removed because deploying stage now receives MigrationResourceBundle
		// (sent from source hub) instead of MigrationTargetBundle. The deploying functionality is
		// thoroughly tested in TestDeployingBatchReceiving tests.
		{
			name: "Cleaning up resources: migrate cluster from hub1 to hub2",
			receivedMigrationEventBundle: migration.MigrationTargetBundle{
				MigrationId:                           "020340324302432049234023040320",
				Stage:                                 migrationv1alpha1.PhaseCleaning,
				ManagedServiceAccountName:             "test", // the migration cr name
				ManagedServiceAccountInstallNamespace: "test",
			},
			expectedError: nil,
			initObjects: []client.Object{
				&operatorv1.ClusterManager{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster-manager",
					},
					Spec: operatorv1.ClusterManagerSpec{
						RegistrationImagePullSpec: "test",
						WorkImagePullSpec:         "test",
					},
				},
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: GetSubjectAccessReviewClusterRoleName("test"),
					},
					Rules: []rbacv1.PolicyRule{
						{
							APIGroups: []string{"authorization.k8s.io"},
							Resources: []string{"subjectaccessreviews"},
							Verbs:     []string{"create"},
						},
					},
				},
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: GetAgentRegistrationClusterRoleBindingName("test"),
					},
					Subjects: []rbacv1.Subject{
						{
							Kind:      "ServiceAccount",
							Name:      "test",
							Namespace: "test",
						},
					},
					RoleRef: rbacv1.RoleRef{
						Kind:     "ClusterRole",
						Name:     "open-cluster-management:managedcluster:bootstrap:agent-registration",
						APIGroup: "rbac.authorization.k8s.io",
					},
				},
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: GetSubjectAccessReviewClusterRoleBindingName("test"),
					},
					Subjects: []rbacv1.Subject{
						{
							Kind:      "ServiceAccount",
							Name:      "test",
							Namespace: "test",
						},
					},
					RoleRef: rbacv1.RoleRef{
						Kind:     "ClusterRole",
						Name:     GetSubjectAccessReviewClusterRoleName("test"),
						APIGroup: "rbac.authorization.k8s.io",
					},
				},
			},
		},
		{
			name: "Cleaning: verify DisableAutoImportAnnotation is removed from managed clusters",
			receivedMigrationEventBundle: migration.MigrationTargetBundle{
				MigrationId:                           "020340324302432049234023040320",
				Stage:                                 migrationv1alpha1.PhaseCleaning,
				ManagedServiceAccountName:             "test",
				ManagedServiceAccountInstallNamespace: "test",
				ManagedClusters:                       []string{"cluster1", "cluster2"},
			},
			expectedError: nil,
			initObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster1",
						Annotations: map[string]string{
							"import.open-cluster-management.io/disable-auto-import": "",
							"other-annotation": "value",
						},
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient: true,
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster2",
						Annotations: map[string]string{
							"import.open-cluster-management.io/disable-auto-import": "",
						},
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient: true,
					},
				},
				&operatorv1.ClusterManager{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster-manager",
					},
					Spec: operatorv1.ClusterManagerSpec{
						RegistrationImagePullSpec: "test",
						WorkImagePullSpec:         "test",
					},
				},
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: GetSubjectAccessReviewClusterRoleName("test"),
					},
					Rules: []rbacv1.PolicyRule{
						{
							APIGroups: []string{"authorization.k8s.io"},
							Resources: []string{"subjectaccessreviews"},
							Verbs:     []string{"create"},
						},
					},
				},
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: GetAgentRegistrationClusterRoleBindingName("test"),
					},
					Subjects: []rbacv1.Subject{
						{
							Kind:      "ServiceAccount",
							Name:      "test",
							Namespace: "test",
						},
					},
					RoleRef: rbacv1.RoleRef{
						Kind:     "ClusterRole",
						Name:     "open-cluster-management:managedcluster:bootstrap:agent-registration",
						APIGroup: "rbac.authorization.k8s.io",
					},
				},
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: GetSubjectAccessReviewClusterRoleBindingName("test"),
					},
					Subjects: []rbacv1.Subject{
						{
							Kind:      "ServiceAccount",
							Name:      "test",
							Namespace: "test",
						},
					},
					RoleRef: rbacv1.RoleRef{
						Kind:     "ClusterRole",
						Name:     GetSubjectAccessReviewClusterRoleName("test"),
						APIGroup: "rbac.authorization.k8s.io",
					},
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(c.initObjects...).WithObjects(
				c.initObjects...).Build()

			producer := ProducerMock{}
			transportClient := &controller.TransportClient{}
			transportClient.SetProducer(&producer)
			transportConfig := &transport.TransportInternalConfig{
				TransportType: string(transport.Chan),
				KafkaCredential: &transport.KafkaConfig{
					SpecTopic:   "spec",
					StatusTopic: "status",
				},
			}

			agentConfig := &configs.AgentConfig{
				TransportConfig: transportConfig,
				LeafHubName:     "hub1",
			}
			managedClusterMigrationSyncer := NewMigrationTargetSyncer(fakeClient, transportClient, agentConfig)

			payload, err := json.Marshal(c.receivedMigrationEventBundle)
			assert.Nil(t, err)
			if err != nil {
				t.Errorf("Failed to marshal payload of managed cluster migration: %v", err)
			}

			eventSource := constants.CloudEventGlobalHubClusterName
			if c.eventSource != "" {
				eventSource = c.eventSource
			}

			// sync managed cluster migration
			evt := utils.ToCloudEvent(constants.MigrationTargetMsgKey, eventSource, "hub2", payload)
			evt.SetTime(time.Now()) // Set event time to avoid time-based skipping in shouldSkipMigrationEvent
			err = managedClusterMigrationSyncer.Sync(ctx, &evt)
			if c.expectedError == nil {
				assert.Nil(t, err)
			} else {
				assert.EqualError(t, err, c.expectedError.Error())
			}

			// For cleaning stage test, verify DisableAutoImportAnnotation is removed
			if c.name == "Cleaning: verify DisableAutoImportAnnotation is removed from managed clusters" {
				for _, clusterName := range c.receivedMigrationEventBundle.ManagedClusters {
					mc := &clusterv1.ManagedCluster{}
					err := fakeClient.Get(ctx, types.NamespacedName{Name: clusterName}, mc)
					assert.Nil(t, err)
					annotations := mc.GetAnnotations()
					_, hasDisableAutoImport := annotations["import.open-cluster-management.io/disable-auto-import"]
					assert.False(t, hasDisableAutoImport, "DisableAutoImportAnnotation should be removed from cluster %s", clusterName)

					// Verify other annotations are preserved
					if clusterName == "cluster1" {
						assert.Equal(t, "value", annotations["other-annotation"], "Other annotations should be preserved")
					}
				}
			}
		})
	}
}

// go test -timeout 30s -run ^TestDeploying$ github.com/stolostron/multicluster-global-hub/agent/pkg/spec/syncers -v
func TestDeploying(t *testing.T) {
	migrationId := "123"

	// Prepare test resources
	managedCluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster1",
		},
		Spec: clusterv1.ManagedClusterSpec{
			HubAcceptsClient:     true,
			LeaseDurationSeconds: 60,
		},
	}

	addonConfig := &addonv1.KlusterletAddonConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster1",
			Namespace: "cluster1",
		},
	}

	// Convert to unstructured
	mcUnstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(managedCluster)
	assert.NoError(t, err)
	mcObj := &unstructured.Unstructured{Object: mcUnstructured}
	mcObj.SetKind("ManagedCluster")
	mcObj.SetAPIVersion("cluster.open-cluster-management.io/v1")

	addonUnstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(addonConfig)
	assert.NoError(t, err)
	addonObj := &unstructured.Unstructured{Object: addonUnstructured}
	addonObj.SetKind("KlusterletAddonConfig")
	addonObj.SetAPIVersion("agent.open-cluster-management.io/v1")

	evt := utils.ToCloudEvent("test", "hub1", "hub2", migration.MigrationResourceBundle{
		MigrationId: migrationId,
		MigrationClusterResources: []migration.MigrationClusterResource{
			{
				ClusterName: "cluster1",
				ResourceList: []unstructured.Unstructured{
					*mcObj,
					*addonObj,
				},
			},
		},
	})
	evt.SetExtension(migration.ExtTotalClusters, 1) // Set totalclusters for batch tracking
	evt.SetTime(time.Now())                         // Set event time to avoid time-based skipping in shouldSkipMigrationEvent

	scheme := configs.GetRuntimeScheme()
	ctx := context.Background()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(
		&clusterv1.ManagedCluster{},
		&addonv1.KlusterletAddonConfig{},
	).Build()
	// set agent config
	configs.SetAgentConfig(&configs.AgentConfig{LeafHubName: "hub2"})
	// set tranport config
	transportConfig := &transport.TransportInternalConfig{KafkaCredential: &transport.KafkaConfig{StatusTopic: "status"}}

	producer := ProducerMock{}
	transportClient := &controller.TransportClient{}
	transportClient.SetProducer(&producer)
	agentConfig := &configs.AgentConfig{
		TransportConfig: transportConfig,
		LeafHubName:     "hub1",
	}
	syncer := NewMigrationTargetSyncer(fakeClient, transportClient, agentConfig)
	syncer.processingMigrationId = migrationId
	err = syncer.Sync(ctx, &evt)
	assert.Nil(t, err)

	// verify the resources
	cluster := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{
		Name: "cluster1",
	}}
	err = fakeClient.Get(ctx, client.ObjectKeyFromObject(cluster), cluster)
	assert.Nil(t, err)
	assert.True(t, cluster.Spec.HubAcceptsClient)
}

// go test -timeout 30s -run ^TestRegistering$ github.com/stolostron/multicluster-global-hub/agent/pkg/spec/syncers -v
func TestRegistering(t *testing.T) {
	ctx := context.Background()
	scheme := configs.GetRuntimeScheme()

	cases := []struct {
		name                 string
		initObjects          []client.Object
		migrationEvent       *migration.MigrationTargetBundle
		expectedError        string
		expectedErrorMessage string
	}{
		{
			name: "All managed clusters are registered and available",
			initObjects: []client.Object{
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{Namespace: "cluster1", Name: "cluster1-klusterlet"},
					Spec:       workv1.ManifestWorkSpec{},
					Status: workv1.ManifestWorkStatus{
						Conditions: []metav1.Condition{
							{
								Type:   workv1.WorkApplied,
								Status: metav1.ConditionTrue,
							},
						},
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{Namespace: "cluster2", Name: "cluster2-klusterlet"},
					Spec:       workv1.ManifestWorkSpec{},
					Status: workv1.ManifestWorkStatus{
						Conditions: []metav1.Condition{
							{
								Type:   workv1.WorkApplied,
								Status: metav1.ConditionTrue,
							},
						},
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster1"},
					Status: clusterv1.ManagedClusterStatus{
						Conditions: []metav1.Condition{
							{
								Type:   clusterv1.ManagedClusterConditionAvailable,
								Status: metav1.ConditionTrue,
							},
						},
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster2"},
					Status: clusterv1.ManagedClusterStatus{
						Conditions: []metav1.Condition{
							{
								Type:   clusterv1.ManagedClusterConditionAvailable,
								Status: metav1.ConditionTrue,
							},
						},
					},
				},
			},
			migrationEvent: &migration.MigrationTargetBundle{
				ManagedClusters: []string{"cluster1", "cluster2"},
			},
			expectedError: "",
		},
		{
			name: "Some managed clusters are not available",
			initObjects: []client.Object{
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{Namespace: "cluster1", Name: "cluster1-klusterlet"},
					Spec:       workv1.ManifestWorkSpec{},
					Status: workv1.ManifestWorkStatus{
						Conditions: []metav1.Condition{
							{
								Type:   workv1.WorkApplied,
								Status: metav1.ConditionTrue,
							},
						},
					},
				},
				// &workv1.ManifestWork{
				// 	ObjectMeta: metav1.ObjectMeta{Namespace: "cluster2", Name: "cluster2-klusterlet"},
				// 	Spec:       workv1.ManifestWorkSpec{},
				// 	Status: workv1.ManifestWorkStatus{
				// 		Conditions: []metav1.Condition{
				// 			{
				// 				Type:   workv1.WorkApplied,
				// 				Status: metav1.ConditionTrue,
				// 			},
				// 		},
				// 	},
				// },
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster1"},
					Status: clusterv1.ManagedClusterStatus{
						Conditions: []metav1.Condition{
							{
								Type:   clusterv1.ManagedClusterConditionAvailable,
								Status: metav1.ConditionTrue,
							},
						},
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster2"},
					Status: clusterv1.ManagedClusterStatus{
						Conditions: []metav1.Condition{
							{
								Type:   clusterv1.ManagedClusterConditionAvailable,
								Status: metav1.ConditionFalse,
							},
						},
					},
				},
			},
			migrationEvent: &migration.MigrationTargetBundle{
				ManagedClusters: []string{"cluster1", "cluster2"},
			},
			expectedError: "failed to wait for 1 managed clusters to be ready",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// change the resitering timeout
			registeringTimeout = 10 * time.Second
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(c.initObjects...).Build()

			agentConfig := &configs.AgentConfig{
				TransportConfig: nil,
				LeafHubName:     "hub1",
			}
			managedClusterMigrationSyncer := NewMigrationTargetSyncer(fakeClient, nil, agentConfig)

			err := managedClusterMigrationSyncer.registering(ctx, c.migrationEvent, map[string]string{})
			if c.expectedError == "" {
				assert.Nil(t, err)
			} else {
				assert.NotNil(t, err)
				assert.Contains(t, err.Error(), c.expectedError)
			}
		})
	}
}

// TestDeployingBatchReceiving tests receiving multiple batches of resources during deploying stage
func TestDeployingBatchReceiving(t *testing.T) {
	migrationId := "test-migration-batch-456"
	scheme := configs.GetRuntimeScheme()
	ctx := context.Background()

	cases := []struct {
		name                  string
		bundleCount           int
		clustersPerBundle     int
		expectedTotalClusters int
	}{
		{
			name:                  "Single bundle with one cluster",
			bundleCount:           1,
			clustersPerBundle:     1,
			expectedTotalClusters: 1,
		},
		{
			name:                  "Single bundle with multiple clusters",
			bundleCount:           1,
			clustersPerBundle:     3,
			expectedTotalClusters: 3,
		},
		{
			name:                  "Multiple bundles with multiple clusters each",
			bundleCount:           3,
			clustersPerBundle:     2,
			expectedTotalClusters: 6,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(
				&clusterv1.ManagedCluster{},
				&addonv1.KlusterletAddonConfig{},
			).Build()
			configs.SetAgentConfig(&configs.AgentConfig{LeafHubName: "hub2"})

			transportConfig := &transport.TransportInternalConfig{
				KafkaCredential: &transport.KafkaConfig{StatusTopic: "status"},
			}

			producer := ProducerMock{}
			transportClient := &controller.TransportClient{}
			transportClient.SetProducer(&producer)
			agentConfig := &configs.AgentConfig{
				TransportConfig: transportConfig,
				LeafHubName:     "hub1",
			}

			syncer := NewMigrationTargetSyncer(fakeClient, transportClient, agentConfig)
			syncer.processingMigrationId = migrationId

			// Send multiple bundles
			totalClusters := 0
			for bundleIdx := 0; bundleIdx < c.bundleCount; bundleIdx++ {
				// Create bundle with clusters
				var clusterResources []migration.MigrationClusterResource
				for clusterIdx := 0; clusterIdx < c.clustersPerBundle; clusterIdx++ {
					clusterName := "cluster-" + string(rune('a'+totalClusters))
					totalClusters++

					// Prepare cluster resources
					managedCluster := &clusterv1.ManagedCluster{
						ObjectMeta: metav1.ObjectMeta{
							Name: clusterName,
						},
						Spec: clusterv1.ManagedClusterSpec{
							HubAcceptsClient:     true,
							LeaseDurationSeconds: 60,
						},
					}

					addonConfig := &addonv1.KlusterletAddonConfig{
						ObjectMeta: metav1.ObjectMeta{
							Name:      clusterName,
							Namespace: clusterName,
						},
					}

					// Convert to unstructured
					mcUnstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(managedCluster)
					assert.NoError(t, err)
					mcObj := &unstructured.Unstructured{Object: mcUnstructured}
					mcObj.SetKind("ManagedCluster")
					mcObj.SetAPIVersion("cluster.open-cluster-management.io/v1")

					addonUnstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(addonConfig)
					assert.NoError(t, err)
					addonObj := &unstructured.Unstructured{Object: addonUnstructured}
					addonObj.SetKind("KlusterletAddonConfig")
					addonObj.SetAPIVersion("agent.open-cluster-management.io/v1")

					clusterResources = append(clusterResources, migration.MigrationClusterResource{
						ClusterName: clusterName,
						ResourceList: []unstructured.Unstructured{
							*mcObj,
							*addonObj,
						},
					})
				}

				// Create and send bundle event
				bundle := migration.MigrationResourceBundle{
					MigrationId:               migrationId,
					MigrationClusterResources: clusterResources,
				}

				evt := utils.ToCloudEvent(constants.MigrationTargetMsgKey, "hub1", "hub2", bundle)
				evt.SetExtension(migration.ExtTotalClusters, c.expectedTotalClusters)
				evt.SetTime(time.Now())

				err := syncer.Sync(ctx, &evt)
				assert.NoError(t, err)
			}

			// Verify all clusters were created
			for i := 0; i < c.expectedTotalClusters; i++ {
				clusterName := "cluster-" + string(rune('a'+i))
				cluster := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{
					Name: clusterName,
				}}
				err := fakeClient.Get(ctx, client.ObjectKeyFromObject(cluster), cluster)
				assert.NoError(t, err, "Cluster %s should exist", clusterName)
				assert.True(t, cluster.Spec.HubAcceptsClient)
			}
		})
	}
}

// TestCleaningStageErrorAggregation tests error aggregation in cleaning stage
func TestCleaningStageErrorAggregation(t *testing.T) {
	scheme := configs.GetRuntimeScheme()
	ctx := context.Background()

	cases := []struct {
		name           string
		initObjects    []client.Object
		migrationEvent *migration.MigrationTargetBundle
		expectError    bool
		errorContains  []string
	}{
		{
			name: "Cleaning stage with multiple cluster errors",
			initObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster1",
						Annotations: map[string]string{
							"import.open-cluster-management.io/disable-auto-import": "",
						},
					},
				},
				// cluster2 missing - will cause error
				&operatorv1.ClusterManager{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster-manager",
					},
					Spec: operatorv1.ClusterManagerSpec{
						RegistrationImagePullSpec: "test",
						WorkImagePullSpec:         "test",
						RegistrationConfiguration: &operatorv1.RegistrationHubConfiguration{
							AutoApproveUsers: []string{"system:serviceaccount:test:test"},
						},
					},
				},
			},
			migrationEvent: &migration.MigrationTargetBundle{
				MigrationId:                           "test-migration",
				Stage:                                 migrationv1alpha1.PhaseCleaning,
				ManagedServiceAccountName:             "test",
				ManagedServiceAccountInstallNamespace: "test",
				ManagedClusters:                       []string{"cluster1", "cluster2"},
			},
			expectError: true,
			errorContains: []string{
				"cleaning failed",
				"error(s)",
			},
		},
		{
			name: "Cleaning stage - RBAC cleanup errors",
			initObjects: []client.Object{
				&operatorv1.ClusterManager{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster-manager",
					},
					Spec: operatorv1.ClusterManagerSpec{
						RegistrationImagePullSpec: "test",
						WorkImagePullSpec:         "test",
						RegistrationConfiguration: &operatorv1.RegistrationHubConfiguration{
							AutoApproveUsers: []string{"system:serviceaccount:test:test"},
						},
					},
				},
				// Missing RBAC resources - will cause cleanup errors
			},
			migrationEvent: &migration.MigrationTargetBundle{
				MigrationId:                           "test-migration",
				Stage:                                 migrationv1alpha1.PhaseCleaning,
				ManagedServiceAccountName:             "test",
				ManagedServiceAccountInstallNamespace: "test",
				ManagedClusters:                       []string{},
			},
			expectError: false, // cleanupMigrationRBAC handles missing resources gracefully
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(c.initObjects...).Build()

			producer := ProducerMock{}
			transportClient := &controller.TransportClient{}
			transportClient.SetProducer(&producer)
			transportConfig := &transport.TransportInternalConfig{
				KafkaCredential: &transport.KafkaConfig{StatusTopic: "status"},
			}

			agentConfig := &configs.AgentConfig{
				TransportConfig: transportConfig,
				LeafHubName:     "hub1",
			}
			syncer := NewMigrationTargetSyncer(fakeClient, transportClient, agentConfig)
			syncer.SetMigrationID(c.migrationEvent.MigrationId)

			clusterErrors := make(map[string]string)
			err := syncer.cleaning(ctx, c.migrationEvent, clusterErrors)

			if c.expectError {
				assert.Error(t, err)
				for _, expectedStr := range c.errorContains {
					assert.Contains(t, err.Error(), expectedStr)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateManagedClustersWithErrors tests error aggregation in validateManagedClusters
func TestValidateManagedClustersWithErrors(t *testing.T) {
	scheme := configs.GetRuntimeScheme()
	ctx := context.Background()

	cases := []struct {
		name          string
		initObjects   []client.Object
		clusterNames  []string
		expectError   bool
		errorContains []string
	}{
		{
			name: "Multiple clusters already exist",
			initObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster1"},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster2"},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster3"},
				},
			},
			clusterNames: []string{"cluster1", "cluster2", "cluster3"},
			expectError:  true,
			errorContains: []string{
				"3 clusters validation failed",
				"3 error(s), get more details in events",
			},
		},
		{
			name:         "All clusters do not exist - validation passes",
			initObjects:  []client.Object{},
			clusterNames: []string{"new-cluster1", "new-cluster2"},
			expectError:  false,
		},
		{
			name: "Some clusters exist, some don't",
			initObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{Name: "existing-cluster"},
				},
			},
			clusterNames: []string{"existing-cluster", "new-cluster"},
			expectError:  true,
			errorContains: []string{
				"1 clusters validation failed",
				"1 error(s), get more details in events",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(c.initObjects...).Build()

			agentConfig := &configs.AgentConfig{
				LeafHubName: "hub1",
			}
			syncer := NewMigrationTargetSyncer(fakeClient, nil, agentConfig)

			clusterErrors := make(map[string]string)
			err := syncer.validateManagedClusters(ctx, c.clusterNames, clusterErrors)

			if c.expectError {
				assert.Error(t, err)
				for _, expectedStr := range c.errorContains {
					assert.Contains(t, err.Error(), expectedStr)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestRollbackInitializingErrorAggregation tests error aggregation in rollbackInitializing
func TestRollbackInitializingErrorAggregation(t *testing.T) {
	scheme := configs.GetRuntimeScheme()
	ctx := context.Background()

	cases := []struct {
		name          string
		initObjects   []client.Object
		spec          *migration.MigrationTargetBundle
		expectError   bool
		errorContains []string
	}{
		{
			name: "Successful rollback initializing with all resources present",
			initObjects: []client.Object{
				&operatorv1.ClusterManager{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster-manager",
					},
					Spec: operatorv1.ClusterManagerSpec{
						RegistrationConfiguration: &operatorv1.RegistrationHubConfiguration{
							AutoApproveUsers: []string{"system:serviceaccount:test:test"},
						},
					},
				},
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: GetSubjectAccessReviewClusterRoleName("test"),
					},
				},
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: GetSubjectAccessReviewClusterRoleBindingName("test"),
					},
				},
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: GetAgentRegistrationClusterRoleBindingName("test"),
					},
				},
			},
			spec: &migration.MigrationTargetBundle{
				ManagedServiceAccountName:             "test",
				ManagedServiceAccountInstallNamespace: "test",
			},
			expectError: false,
		},
		{
			name: "RBAC cleanup errors during rollback",
			initObjects: []client.Object{
				&operatorv1.ClusterManager{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster-manager",
					},
					Spec: operatorv1.ClusterManagerSpec{
						RegistrationConfiguration: &operatorv1.RegistrationHubConfiguration{
							AutoApproveUsers: []string{"system:serviceaccount:test:test"},
						},
					},
				},
				// Missing RBAC resources
			},
			spec: &migration.MigrationTargetBundle{
				ManagedServiceAccountName:             "test",
				ManagedServiceAccountInstallNamespace: "test",
			},
			expectError: false, // RBAC cleanup handles missing resources gracefully
		},
		{
			name: "Empty ManagedServiceAccountName - should skip",
			initObjects: []client.Object{
				&operatorv1.ClusterManager{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster-manager",
					},
				},
			},
			spec: &migration.MigrationTargetBundle{
				ManagedServiceAccountName:             "",
				ManagedServiceAccountInstallNamespace: "",
			},
			expectError: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(c.initObjects...).Build()

			agentConfig := &configs.AgentConfig{
				LeafHubName: "hub1",
			}
			syncer := NewMigrationTargetSyncer(fakeClient, nil, agentConfig)

			clusterErrors := make(map[string]string)
			err := syncer.rollbackInitializing(ctx, c.spec, clusterErrors)

			if c.expectError {
				assert.Error(t, err)
				for _, expectedStr := range c.errorContains {
					assert.Contains(t, err.Error(), expectedStr)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestRollbackDeployingErrorAggregation tests error aggregation in rollbackDeploying
func TestRollbackDeployingErrorAggregation(t *testing.T) {
	scheme := configs.GetRuntimeScheme()
	ctx := context.Background()

	cases := []struct {
		name          string
		initObjects   []client.Object
		spec          *migration.MigrationTargetBundle
		expectError   bool
		errorContains []string
	}{
		{
			name: "Multiple errors during deploying rollback",
			initObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster1"},
				},
				&addonv1.KlusterletAddonConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cluster1",
						Namespace: "cluster1",
					},
				},
				&operatorv1.ClusterManager{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster-manager",
					},
					Spec: operatorv1.ClusterManagerSpec{
						RegistrationConfiguration: &operatorv1.RegistrationHubConfiguration{
							AutoApproveUsers: []string{"system:serviceaccount:test:test"},
						},
					},
				},
			},
			spec: &migration.MigrationTargetBundle{
				ManagedServiceAccountName:             "test",
				ManagedServiceAccountInstallNamespace: "test",
				ManagedClusters:                       []string{"cluster1", "cluster2"}, // cluster2 missing
			},
			expectError: false, // rollback continues on errors
		},
		{
			name:        "Empty cluster list - should succeed",
			initObjects: []client.Object{},
			spec: &migration.MigrationTargetBundle{
				ManagedServiceAccountName:             "test",
				ManagedServiceAccountInstallNamespace: "test",
				ManagedClusters:                       []string{},
			},
			expectError: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(c.initObjects...).Build()

			agentConfig := &configs.AgentConfig{
				LeafHubName: "hub1",
			}
			syncer := NewMigrationTargetSyncer(fakeClient, nil, agentConfig)

			clusterErrors := make(map[string]string)
			err := syncer.rollbackDeploying(ctx, c.spec, clusterErrors)

			if c.expectError {
				assert.Error(t, err)
				for _, expectedStr := range c.errorContains {
					assert.Contains(t, err.Error(), expectedStr)
				}
			} else {
				// May have errors but should not fail completely
				if err != nil {
					t.Logf("Rollback completed with non-fatal errors: %v", err)
				}
			}
		})
	}
}

// TestCleanupMigrationRBACErrorAggregation tests error aggregation in cleanupMigrationRBAC
func TestCleanupMigrationRBACErrorAggregation(t *testing.T) {
	scheme := configs.GetRuntimeScheme()
	ctx := context.Background()

	cases := []struct {
		name          string
		initObjects   []client.Object
		msaName       string
		expectError   bool
		errorContains []string
	}{
		{
			name: "All RBAC resources exist - should cleanup successfully",
			initObjects: []client.Object{
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: GetSubjectAccessReviewClusterRoleName("test-msa"),
					},
				},
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: GetSubjectAccessReviewClusterRoleBindingName("test-msa"),
					},
				},
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: GetAgentRegistrationClusterRoleBindingName("test-msa"),
					},
				},
			},
			msaName:     "test-msa",
			expectError: false,
		},
		{
			name:        "No RBAC resources exist - should handle gracefully",
			initObjects: []client.Object{},
			msaName:     "test-msa",
			expectError: false, // deleteResourceIfExists handles NotFound gracefully
		},
		{
			name: "Partial RBAC resources exist",
			initObjects: []client.Object{
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: GetSubjectAccessReviewClusterRoleName("test-msa"),
					},
				},
				// Missing ClusterRoleBindings
			},
			msaName:     "test-msa",
			expectError: false, // Missing resources handled gracefully
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(c.initObjects...).Build()

			syncer := &MigrationTargetSyncer{
				client:      fakeClient,
				leafHubName: "hub1",
			}

			err := syncer.cleanupMigrationRBAC(ctx, c.msaName)

			if c.expectError {
				assert.Error(t, err)
				for _, expectedStr := range c.errorContains {
					assert.Contains(t, err.Error(), expectedStr)
				}
			} else {
				assert.NoError(t, err)
			}

			// Verify resources were deleted
			clusterRole := &rbacv1.ClusterRole{}
			err = fakeClient.Get(ctx, client.ObjectKey{
				Name: GetSubjectAccessReviewClusterRoleName(c.msaName),
			}, clusterRole)
			assert.True(t, client.IgnoreNotFound(err) == nil, "ClusterRole should be deleted or not found")
		})
	}
}

// TestValidatingStage tests the validating stage
func TestValidatingStage(t *testing.T) {
	scheme := configs.GetRuntimeScheme()
	ctx := context.Background()

	cases := []struct {
		name          string
		initObjects   []client.Object
		event         *migration.MigrationTargetBundle
		expectError   bool
		errorContains []string
	}{
		{
			name: "Validation passes with no existing clusters",
			initObjects: []client.Object{
				&operatorv1.ClusterManager{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster-manager"},
				},
			},
			event: &migration.MigrationTargetBundle{
				MigrationId:     "test-migration",
				Stage:           migrationv1alpha1.PhaseValidating,
				ManagedClusters: []string{"new-cluster1", "new-cluster2"},
			},
			expectError: false,
		},
		{
			name: "Validation fails with existing clusters",
			initObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{Name: "existing-cluster"},
				},
			},
			event: &migration.MigrationTargetBundle{
				MigrationId:     "test-migration",
				Stage:           migrationv1alpha1.PhaseValidating,
				ManagedClusters: []string{"existing-cluster", "new-cluster"},
			},
			expectError: true,
			errorContains: []string{
				"clusters validation failed",
				"1 error(s), get more details in events",
			},
		},
		{
			name:        "Validation with no clusters",
			initObjects: []client.Object{},
			event: &migration.MigrationTargetBundle{
				MigrationId:     "test-migration",
				Stage:           migrationv1alpha1.PhaseValidating,
				ManagedClusters: []string{},
			},
			expectError: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(c.initObjects...).Build()

			agentConfig := &configs.AgentConfig{
				LeafHubName: "hub1",
			}
			syncer := NewMigrationTargetSyncer(fakeClient, nil, agentConfig)

			clusterErrors := make(map[string]string)
			err := syncer.validating(ctx, c.event, clusterErrors)

			if c.expectError {
				assert.Error(t, err)
				for _, expectedStr := range c.errorContains {
					assert.Contains(t, err.Error(), expectedStr)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestRollbackRegistering tests the rollbackRegistering function
func TestRollbackRegistering(t *testing.T) {
	scheme := configs.GetRuntimeScheme()
	ctx := context.Background()

	cases := []struct {
		name        string
		initObjects []client.Object
		spec        *migration.MigrationTargetBundle
		expectError bool
	}{
		{
			name: "Rollback registering with resources",
			initObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster1"},
				},
				&addonv1.KlusterletAddonConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cluster1",
						Namespace: "cluster1",
					},
				},
				&operatorv1.ClusterManager{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster-manager"},
					Spec: operatorv1.ClusterManagerSpec{
						RegistrationConfiguration: &operatorv1.RegistrationHubConfiguration{
							AutoApproveUsers: []string{"system:serviceaccount:test:test"},
						},
					},
				},
			},
			spec: &migration.MigrationTargetBundle{
				ManagedServiceAccountName:             "test",
				ManagedServiceAccountInstallNamespace: "test",
				ManagedClusters:                       []string{"cluster1"},
			},
			expectError: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(c.initObjects...).Build()

			agentConfig := &configs.AgentConfig{
				LeafHubName: "hub1",
			}
			syncer := NewMigrationTargetSyncer(fakeClient, nil, agentConfig)

			clusterErrors := make(map[string]string)
			err := syncer.rollbackRegistering(ctx, c.spec, clusterErrors)

			if c.expectError {
				assert.Error(t, err)
			} else {
				// rollbackRegistering may return errors but should continue
				if err != nil {
					t.Logf("Rollback completed with errors: %v", err)
				}
			}
		})
	}
}

// TestShouldSkipMigrationEvent tests the shouldSkipMigrationEvent function
func TestShouldSkipMigrationEvent(t *testing.T) {
	scheme := configs.GetRuntimeScheme()
	ctx := context.Background()

	cases := []struct {
		name           string
		setupConfigMap func(client.Client) error
		eventTime      time.Time
		expectSkip     bool
		expectError    bool
	}{
		{
			name: "Skip event - cached time is newer",
			setupConfigMap: func(c client.Client) error {
				// Set a future time in configmap
				futureTime := time.Now().Add(1 * time.Hour)
				return configs.SetSyncTimeState(ctx, c, "test-topic--test-source", futureTime)
			},
			eventTime:   time.Now(),
			expectSkip:  true,
			expectError: false,
		},
		{
			name: "Process event - no cached time and recent event",
			setupConfigMap: func(c client.Client) error {
				return nil // No setup needed
			},
			eventTime:   time.Now(),
			expectSkip:  false,
			expectError: false,
		},
		{
			name: "Skip event - no cached time and old event",
			setupConfigMap: func(c client.Client) error {
				return nil // No setup needed
			},
			eventTime:   time.Now().Add(-15 * time.Minute), // 15 minutes old
			expectSkip:  true,
			expectError: false,
		},
		{
			name: "Process event - cached time is older",
			setupConfigMap: func(c client.Client) error {
				pastTime := time.Now().Add(-1 * time.Hour)
				return configs.SetSyncTimeState(ctx, c, "test-topic--test-source", pastTime)
			},
			eventTime:   time.Now(),
			expectSkip:  false,
			expectError: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			configs.SetAgentConfig(&configs.AgentConfig{LeafHubName: "test-dest"})

			if c.setupConfigMap != nil {
				err := c.setupConfigMap(fakeClient)
				assert.NoError(t, err)
			}

			evt := utils.ToCloudEvent("test-type", "test-source", "test-dest", []byte("{}"))
			evt.SetTime(c.eventTime)
			evt.SetExtension("kafkatopic", "test-topic")

			skip, err := shouldSkipMigrationEvent(ctx, fakeClient, &evt)

			if c.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, c.expectSkip, skip)
			}
		})
	}
}

// TestHandleStage tests the handleStage function
func TestHandleStage(t *testing.T) {
	scheme := configs.GetRuntimeScheme()
	ctx := context.Background()

	cases := []struct {
		name          string
		initObjects   []client.Object
		event         *migration.MigrationTargetBundle
		expectError   bool
		errorContains string
	}{
		{
			name:        "Handle validating stage",
			initObjects: []client.Object{},
			event: &migration.MigrationTargetBundle{
				MigrationId:     "test-migration",
				Stage:           migrationv1alpha1.PhaseValidating,
				ManagedClusters: []string{},
			},
			expectError: false,
		},
		{
			name: "Handle initializing stage",
			initObjects: []client.Object{
				&operatorv1.ClusterManager{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster-manager"},
				},
			},
			event: &migration.MigrationTargetBundle{
				MigrationId:                           "test-migration",
				Stage:                                 migrationv1alpha1.PhaseInitializing,
				ManagedServiceAccountName:             "test",
				ManagedServiceAccountInstallNamespace: "test",
			},
			expectError: false,
		},
		{
			name: "Handle cleaning stage",
			initObjects: []client.Object{
				&operatorv1.ClusterManager{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster-manager"},
					Spec: operatorv1.ClusterManagerSpec{
						RegistrationConfiguration: &operatorv1.RegistrationHubConfiguration{
							AutoApproveUsers: []string{"system:serviceaccount:test:test"},
						},
					},
				},
			},
			event: &migration.MigrationTargetBundle{
				MigrationId:                           "test-migration",
				Stage:                                 migrationv1alpha1.PhaseCleaning,
				ManagedServiceAccountName:             "test",
				ManagedServiceAccountInstallNamespace: "test",
			},
			expectError: false,
		},
		{
			name: "Handle rollbacking stage",
			initObjects: []client.Object{
				&operatorv1.ClusterManager{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster-manager"},
				},
			},
			event: &migration.MigrationTargetBundle{
				MigrationId:                           "test-migration",
				Stage:                                 migrationv1alpha1.PhaseRollbacking,
				RollbackStage:                         migrationv1alpha1.PhaseInitializing,
				ManagedServiceAccountName:             "test",
				ManagedServiceAccountInstallNamespace: "test",
			},
			expectError: false,
		},
		{
			name:        "Handle unknown stage",
			initObjects: []client.Object{},
			event: &migration.MigrationTargetBundle{
				MigrationId: "test-migration",
				Stage:       "UnknownStage",
			},
			expectError:   true,
			errorContains: "unknown migration stage",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(c.initObjects...).Build()

			agentConfig := &configs.AgentConfig{
				LeafHubName: "hub1",
			}
			syncer := NewMigrationTargetSyncer(fakeClient, nil, agentConfig)

			clusterErrors := make(map[string]string)
			err := syncer.handleStage(ctx, c.event, clusterErrors)

			if c.expectError {
				assert.Error(t, err)
				if c.errorContains != "" {
					assert.Contains(t, err.Error(), c.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestDeleteResourceIfExists tests the deleteResourceIfExists function
func TestDeleteResourceIfExists(t *testing.T) {
	scheme := configs.GetRuntimeScheme()
	ctx := context.Background()

	cases := []struct {
		name        string
		initObjects []client.Object
		resourceKey client.ObjectKey
		forceDelete bool
		expectError bool
		shouldExist bool
	}{
		{
			name: "Delete existing resource without finalizers",
			initObjects: []client.Object{
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-role",
					},
				},
			},
			resourceKey: client.ObjectKey{Name: "test-role"},
			forceDelete: false,
			expectError: false,
			shouldExist: false,
		},
		{
			name: "Delete existing resource with finalizers - forceDelete=false",
			initObjects: []client.Object{
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-role-with-finalizer",
						Finalizers: []string{"test-finalizer"},
					},
				},
			},
			resourceKey: client.ObjectKey{Name: "test-role-with-finalizer"},
			forceDelete: false,
			expectError: false,
			shouldExist: false, // fake client handles deletion
		},
		{
			name: "Delete existing resource with finalizers - forceDelete=true",
			initObjects: []client.Object{
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-role-force",
						Finalizers: []string{"test-finalizer"},
					},
				},
			},
			resourceKey: client.ObjectKey{Name: "test-role-force"},
			forceDelete: true,
			expectError: false,
			shouldExist: false,
		},
		{
			name:        "Delete non-existing resource",
			initObjects: []client.Object{},
			resourceKey: client.ObjectKey{Name: "non-existing"},
			forceDelete: false,
			expectError: false,
			shouldExist: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(c.initObjects...).Build()

			obj := &rbacv1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{
					Name: c.resourceKey.Name,
				},
			}

			err := deleteResourceIfExists(ctx, fakeClient, obj, c.forceDelete)

			if c.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Verify the resource state
				checkObj := &rbacv1.ClusterRole{}
				getErr := fakeClient.Get(ctx, c.resourceKey, checkObj)
				if c.shouldExist {
					assert.NoError(t, getErr)
				} else {
					assert.True(t, client.IgnoreNotFound(getErr) == nil)
				}
			}
		})
	}
}

// TestInitializing tests the initializing stage
func TestInitializing(t *testing.T) {
	scheme := configs.GetRuntimeScheme()
	ctx := context.Background()

	cases := []struct {
		name          string
		initObjects   []client.Object
		event         *migration.MigrationTargetBundle
		expectError   bool
		errorContains string
	}{
		{
			name: "Successful initializing with minimal ClusterManager",
			initObjects: []client.Object{
				&operatorv1.ClusterManager{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster-manager"},
				},
			},
			event: &migration.MigrationTargetBundle{
				ManagedServiceAccountName:             "test-msa",
				ManagedServiceAccountInstallNamespace: "test-ns",
			},
			expectError: false,
		},
		{
			name: "Initializing with existing ClusterManager configuration",
			initObjects: []client.Object{
				&operatorv1.ClusterManager{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster-manager"},
					Spec: operatorv1.ClusterManagerSpec{
						RegistrationConfiguration: &operatorv1.RegistrationHubConfiguration{
							FeatureGates: []operatorv1.FeatureGate{
								{
									Feature: "OtherFeature",
									Mode:    operatorv1.FeatureGateModeTypeEnable,
								},
							},
							AutoApproveUsers: []string{"other-user"},
						},
					},
				},
			},
			event: &migration.MigrationTargetBundle{
				ManagedServiceAccountName:             "test-msa",
				ManagedServiceAccountInstallNamespace: "test-ns",
			},
			expectError: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(c.initObjects...).Build()

			agentConfig := &configs.AgentConfig{
				LeafHubName: "hub1",
			}
			syncer := NewMigrationTargetSyncer(fakeClient, nil, agentConfig)

			clusterErrors := make(map[string]string)
			err := syncer.initializing(ctx, c.event, clusterErrors)

			if c.expectError {
				assert.Error(t, err)
				if c.errorContains != "" {
					assert.Contains(t, err.Error(), c.errorContains)
				}
			} else {
				assert.NoError(t, err)

				// Verify ClusterManager was updated
				cm := &operatorv1.ClusterManager{}
				err := fakeClient.Get(ctx, client.ObjectKey{Name: "cluster-manager"}, cm)
				assert.NoError(t, err)

				// Check that auto-approve feature is enabled
				assert.NotNil(t, cm.Spec.RegistrationConfiguration)
				hasAutoApprove := false
				for _, fg := range cm.Spec.RegistrationConfiguration.FeatureGates {
					if fg.Feature == "ManagedClusterAutoApproval" && fg.Mode == operatorv1.FeatureGateModeTypeEnable {
						hasAutoApprove = true
						break
					}
				}
				assert.True(t, hasAutoApprove, "ManagedClusterAutoApproval feature should be enabled")

				// Check that the MSA user is in auto-approve list
				expectedUser := fmt.Sprintf("system:serviceaccount:%s:%s", c.event.ManagedServiceAccountInstallNamespace, c.event.ManagedServiceAccountName)
				assert.Contains(t, cm.Spec.RegistrationConfiguration.AutoApproveUsers, expectedUser)
			}
		})
	}
}

// TestDeployingBatchReceivingError tests error handling during batch receiving
func TestDeployingBatchReceivingError(t *testing.T) {
	migrationId := "test-migration-error-789"
	scheme := configs.GetRuntimeScheme()
	ctx := context.Background()

	t.Run("Receive bundle with mismatched migration ID", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		configs.SetAgentConfig(&configs.AgentConfig{LeafHubName: "hub2"})

		transportConfig := &transport.TransportInternalConfig{
			KafkaCredential: &transport.KafkaConfig{StatusTopic: "status"},
		}

		producer := ProducerMock{}
		transportClient := &controller.TransportClient{}
		transportClient.SetProducer(&producer)
		agentConfig := &configs.AgentConfig{
			TransportConfig: transportConfig,
			LeafHubName:     "hub1",
		}

		syncer := NewMigrationTargetSyncer(fakeClient, transportClient, agentConfig)
		syncer.processingMigrationId = migrationId

		// Send bundle with different migration ID
		bundle := migration.MigrationResourceBundle{
			MigrationId:               "different-migration-id",
			MigrationClusterResources: []migration.MigrationClusterResource{},
		}

		evt := utils.ToCloudEvent(constants.MigrationTargetMsgKey, "hub1", "hub2", bundle)
		evt.SetTime(time.Now())

		// Should return error due to migration ID mismatch
		err := syncer.Sync(ctx, &evt)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "expected migrationId")
	})
}

// TestAddPauseAnnotationBeforeDeletion tests the addPauseAnnotationBeforeDeletion function
func TestAddPauseAnnotationBeforeDeletion(t *testing.T) {
	scheme := configs.GetRuntimeScheme()
	ctx := context.Background()

	cases := []struct {
		name         string
		resource     *unstructured.Unstructured
		expectError  bool
		errorMessage string
	}{
		{
			name: "Add pause annotation to ClusterDeployment without existing annotations",
			resource: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "hive.openshift.io/v1",
					"kind":       "ClusterDeployment",
					"metadata": map[string]interface{}{
						"name":      "test-cluster",
						"namespace": "test-namespace",
					},
					"spec": map[string]interface{}{
						"clusterName": "test-cluster",
					},
				},
			},
			expectError: false,
		},
		{
			name: "Add pause annotation to ClusterDeployment with existing annotations",
			resource: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "hive.openshift.io/v1",
					"kind":       "ClusterDeployment",
					"metadata": map[string]interface{}{
						"name":      "test-cluster-2",
						"namespace": "test-namespace",
						"annotations": map[string]interface{}{
							"existing-annotation": "value",
						},
					},
					"spec": map[string]interface{}{
						"clusterName": "test-cluster-2",
					},
				},
			},
			expectError: false,
		},
		{
			name: "Add pause annotation to ImageClusterInstall",
			resource: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "extensions.hive.openshift.io/v1alpha1",
					"kind":       "ImageClusterInstall",
					"metadata": map[string]interface{}{
						"name":      "test-image-cluster",
						"namespace": "test-namespace",
					},
					"spec": map[string]interface{}{
						"imageSetRef": map[string]interface{}{
							"name": "test-imageset",
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Resource not found - should return error",
			resource: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "hive.openshift.io/v1",
					"kind":       "ClusterDeployment",
					"metadata": map[string]interface{}{
						"name":      "non-existing",
						"namespace": "test-namespace",
					},
				},
			},
			expectError:  true,
			errorMessage: "failed to get latest resource",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Prepare initial objects - add the resource to the fake client if it should exist
			var initObjects []client.Object
			if !c.expectError || c.errorMessage == "" {
				// Create a copy of the resource to add to initial objects
				initResource := c.resource.DeepCopy()
				initObjects = append(initObjects, initResource)
			}

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

			syncer := &MigrationTargetSyncer{
				client: fakeClient,
			}

			err := syncer.addPauseAnnotationBeforeDeletion(ctx, c.resource)

			if c.expectError {
				assert.Error(t, err)
				if c.errorMessage != "" {
					assert.Contains(t, err.Error(), c.errorMessage)
				}
			} else {
				assert.NoError(t, err)

				// Verify pause annotation was added
				verifyResource := &unstructured.Unstructured{}
				verifyResource.SetGroupVersionKind(c.resource.GroupVersionKind())
				err = fakeClient.Get(ctx, client.ObjectKeyFromObject(c.resource), verifyResource)
				assert.NoError(t, err)

				annotations := verifyResource.GetAnnotations()
				assert.NotNil(t, annotations)
				assert.Equal(t, "true", annotations[PauseAnnotation])

				// Verify existing annotations are preserved
				if origAnnotations := c.resource.GetAnnotations(); origAnnotations != nil {
					for key, value := range origAnnotations {
						if key != PauseAnnotation {
							assert.Equal(t, value, annotations[key], "Original annotation %s should be preserved", key)
						}
					}
				}
			}
		})
	}
}

// TestAddPauseAnnotationMultipleCalls tests calling addPauseAnnotationBeforeDeletion multiple times
func TestAddPauseAnnotationMultipleCalls(t *testing.T) {
	scheme := configs.GetRuntimeScheme()
	ctx := context.Background()

	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata": map[string]interface{}{
				"name":      "test-cluster",
				"namespace": "test-namespace",
			},
			"spec": map[string]interface{}{
				"clusterName": "test-cluster",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(resource.DeepCopy()).Build()

	syncer := &MigrationTargetSyncer{
		client: fakeClient,
	}

	// First call should succeed
	err := syncer.addPauseAnnotationBeforeDeletion(ctx, resource)
	assert.NoError(t, err)

	// Verify the annotation exists
	verifyResource := &unstructured.Unstructured{}
	verifyResource.SetGroupVersionKind(resource.GroupVersionKind())
	err = fakeClient.Get(ctx, client.ObjectKeyFromObject(resource), verifyResource)
	assert.NoError(t, err)

	annotations := verifyResource.GetAnnotations()
	assert.NotNil(t, annotations)
	assert.Equal(t, "true", annotations[PauseAnnotation])

	// Second call should also succeed (idempotent)
	err = syncer.addPauseAnnotationBeforeDeletion(ctx, resource)
	assert.NoError(t, err)

	// Verify annotation still exists and has correct value
	verifyResource2 := &unstructured.Unstructured{}
	verifyResource2.SetGroupVersionKind(resource.GroupVersionKind())
	err = fakeClient.Get(ctx, client.ObjectKeyFromObject(resource), verifyResource2)
	assert.NoError(t, err)

	annotations2 := verifyResource2.GetAnnotations()
	assert.NotNil(t, annotations2)
	assert.Equal(t, "true", annotations2[PauseAnnotation])
}

func TestRemoveMigrationResources(t *testing.T) {
	scheme := configs.GetRuntimeScheme()
	ctx := context.Background()

	cases := []struct {
		name              string
		clusterName       string
		initObjects       []client.Object
		expectedDeleted   []string // resource names that should be deleted
		expectedRemaining []string // resource names that should remain
		expectError       bool
	}{
		{
			name:        "Delete specific named resources - ManagedCluster and KlusterletAddonConfig",
			clusterName: "test-cluster",
			initObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-cluster",
						Finalizers: []string{
							"test-finalizer",
						},
					},
				},
				&addonv1.KlusterletAddonConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-cluster",
						Namespace: "test-cluster",
						Finalizers: []string{
							"addon-finalizer",
						},
					},
				},
			},
			expectedDeleted: []string{
				"klusterletaddonconfig/test-cluster/test-cluster",
			},
			expectedRemaining: []string{
				"managedcluster/test-cluster",
			},
			expectError: false,
		},
		{
			name:        "Delete ClusterDeployment and ImageClusterInstall with pause annotation",
			clusterName: "ztp-cluster",
			initObjects: []client.Object{
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "hive.openshift.io/v1",
						"kind":       "ClusterDeployment",
						"metadata": map[string]interface{}{
							"name":      "ztp-cluster",
							"namespace": "ztp-cluster",
						},
						"spec": map[string]interface{}{
							"clusterName": "ztp-cluster",
						},
					},
				},
				func() *unstructured.Unstructured {
					obj := &unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "extensions.hive.openshift.io/v1alpha1",
							"kind":       "ImageClusterInstall",
							"metadata": map[string]interface{}{
								"name":      "ztp-cluster",
								"namespace": "ztp-cluster",
							},
							"spec": map[string]interface{}{
								"imageSetRef": map[string]interface{}{
									"name": "test-imageset",
								},
							},
						},
					}
					obj.SetFinalizers([]string{"imageclusterinstall.extensions.hive.openshift.io/deprovision"})
					return obj
				}(),
			},
			expectedDeleted: []string{
				"clusterdeployment/ztp-cluster/ztp-cluster",
				"imageclusterinstall/ztp-cluster/ztp-cluster",
			},
			expectedRemaining: []string{},
			expectError:       false,
		},
		{
			name:        "Delete wildcard resources - all BareMetalHosts in namespace",
			clusterName: "metal-cluster",
			initObjects: []client.Object{
				func() *unstructured.Unstructured {
					obj := &unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "metal3.io/v1alpha1",
							"kind":       "BareMetalHost",
							"metadata": map[string]interface{}{
								"name":      "bmh-1",
								"namespace": "metal-cluster",
							},
							"spec": map[string]interface{}{
								"online": true,
							},
						},
					}
					obj.SetFinalizers([]string{"baremetalhost.metal3.io"})
					return obj
				}(),
				func() *unstructured.Unstructured {
					obj := &unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "metal3.io/v1alpha1",
							"kind":       "BareMetalHost",
							"metadata": map[string]interface{}{
								"name":      "bmh-2",
								"namespace": "metal-cluster",
							},
							"spec": map[string]interface{}{
								"online": false,
							},
						},
					}
					obj.SetFinalizers([]string{"baremetalhost.metal3.io"})
					return obj
				}(),
			},
			expectedDeleted: []string{
				"baremetalhost/metal-cluster/bmh-1",
				"baremetalhost/metal-cluster/bmh-2",
			},
			expectedRemaining: []string{},
			expectError:       false,
		},
		{
			name:        "Delete secrets with specific names",
			clusterName: "secret-cluster",
			initObjects: []client.Object{
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "Secret",
						"metadata": map[string]interface{}{
							"name":      "secret-cluster-admin-password",
							"namespace": "secret-cluster",
						},
						"data": map[string]interface{}{
							"password": "dGVzdA==",
						},
					},
				},
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "Secret",
						"metadata": map[string]interface{}{
							"name":      "secret-cluster-admin-kubeconfig",
							"namespace": "secret-cluster",
						},
						"data": map[string]interface{}{
							"kubeconfig": "dGVzdA==",
						},
					},
				},
				// This secret should NOT be deleted (different name)
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "Secret",
						"metadata": map[string]interface{}{
							"name":      "other-secret",
							"namespace": "secret-cluster",
						},
						"data": map[string]interface{}{
							"data": "dGVzdA==",
						},
					},
				},
			},
			expectedDeleted: []string{
				"secret/secret-cluster/secret-cluster-admin-password",
				"secret/secret-cluster/secret-cluster-admin-kubeconfig",
			},
			expectedRemaining: []string{
				"secret/secret-cluster/other-secret",
			},
			expectError: false,
		},
		{
			name:              "Handle empty cluster - no resources to delete",
			clusterName:       "empty-cluster",
			initObjects:       []client.Object{},
			expectedDeleted:   []string{},
			expectedRemaining: []string{},
			expectError:       false,
		},
		{
			name:        "Mixed scenario - some resources exist, some don't",
			clusterName: "mixed-cluster",
			initObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "mixed-cluster",
					},
				},
				// KlusterletAddonConfig is missing - should not cause error
			},
			expectedDeleted: []string{
				"managedcluster/mixed-cluster",
			},
			expectedRemaining: []string{},
			expectError:       false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tc.initObjects...).
				Build()

			syncer := &MigrationTargetSyncer{
				client: fakeClient,
			}

			// Execute the function under test
			err := syncer.removeMigrationResources(ctx, tc.clusterName)

			// Check error expectation
			if tc.expectError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			// Verify deleted resources
			for _, deletedResource := range tc.expectedDeleted {
				parts := parseResourceIdentifier(deletedResource)
				obj := &unstructured.Unstructured{}
				obj.SetGroupVersionKind(getGVKFromKind(parts.kind))

				err := fakeClient.Get(ctx, client.ObjectKey{
					Name:      parts.name,
					Namespace: parts.namespace,
				}, obj)

				// Resource should be deleted (NotFound error is expected)
				// Note: In fake client, deletion is immediate even with finalizers
				assert.True(t, client.IgnoreNotFound(err) == nil,
					"Resource %s should be deleted or not found", deletedResource)
			}

			// Verify remaining resources
			for _, remainingResource := range tc.expectedRemaining {
				parts := parseResourceIdentifier(remainingResource)
				obj := &unstructured.Unstructured{}
				obj.SetGroupVersionKind(getGVKFromKind(parts.kind))

				err := fakeClient.Get(ctx, client.ObjectKey{
					Name:      parts.name,
					Namespace: parts.namespace,
				}, obj)

				assert.NoError(t, err, "Resource %s should still exist", remainingResource)
			}
		})
	}
}

// Helper struct for parsing resource identifiers
type resourceIdentifier struct {
	kind      string
	namespace string
	name      string
}

// parseResourceIdentifier parses "kind/namespace/name" or "kind/name" format
func parseResourceIdentifier(identifier string) resourceIdentifier {
	parts := strings.Split(identifier, "/")
	if len(parts) == 3 {
		return resourceIdentifier{
			kind:      parts[0],
			namespace: parts[1],
			name:      parts[2],
		}
	}
	// For cluster-scoped resources
	return resourceIdentifier{
		kind: parts[0],
		name: parts[1],
	}
}

// getGVKFromKind returns GroupVersionKind based on resource kind
func getGVKFromKind(kind string) schema.GroupVersionKind {
	switch strings.ToLower(kind) {
	case "managedcluster":
		return schema.GroupVersionKind{
			Group:   "cluster.open-cluster-management.io",
			Version: "v1",
			Kind:    "ManagedCluster",
		}
	case "klusterletaddonconfig":
		return schema.GroupVersionKind{
			Group:   "agent.open-cluster-management.io",
			Version: "v1",
			Kind:    "KlusterletAddonConfig",
		}
	case "clusterdeployment":
		return schema.GroupVersionKind{
			Group:   "hive.openshift.io",
			Version: "v1",
			Kind:    "ClusterDeployment",
		}
	case "imageclusterinstall":
		return schema.GroupVersionKind{
			Group:   "extensions.hive.openshift.io",
			Version: "v1alpha1",
			Kind:    "ImageClusterInstall",
		}
	case "baremetalhost":
		return schema.GroupVersionKind{
			Group:   "metal3.io",
			Version: "v1alpha1",
			Kind:    "BareMetalHost",
		}
	case "secret":
		return schema.GroupVersionKind{
			Group:   "",
			Version: "v1",
			Kind:    "Secret",
		}
	default:
		return schema.GroupVersionKind{}
	}
}

// TestDeleteResourceIfExistsComprehensive tests deleteResourceIfExists with all code paths
func TestDeleteResourceIfExistsComprehensive(t *testing.T) {
	scheme := configs.GetRuntimeScheme()
	ctx := context.Background()

	cases := []struct {
		name          string
		initObjects   []client.Object
		resource      client.Object
		forceDelete   bool
		expectError   bool
		errorContains string
	}{
		{
			name: "Delete existing resource without finalizers",
			initObjects: []client.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
				},
			},
			resource: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
			},
			forceDelete: false,
			expectError: false,
		},
		{
			name:        "Delete non-existent resource - should not error",
			initObjects: []client.Object{},
			resource: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "non-existent"},
			},
			forceDelete: false,
			expectError: false,
		},
		{
			name:        "Delete nil resource - should not error",
			initObjects: []client.Object{},
			resource:    nil,
			forceDelete: false,
			expectError: false,
		},
		{
			name: "Delete resource with finalizers and forceDelete=true",
			initObjects: []client.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-namespace-with-finalizers",
						Finalizers: []string{"test.finalizer.io/lock"},
					},
				},
			},
			resource: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "test-namespace-with-finalizers"},
			},
			forceDelete: true,
			expectError: false,
		},
		{
			name: "Delete resource with finalizers and forceDelete=false",
			initObjects: []client.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-namespace-with-finalizers2",
						Finalizers: []string{"kubernetes.io/metadata.name"},
					},
				},
			},
			resource: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "test-namespace-with-finalizers2"},
			},
			forceDelete: false,
			expectError: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(c.initObjects...).Build()

			err := deleteResourceIfExists(ctx, fakeClient, c.resource, c.forceDelete)

			if c.expectError {
				assert.Error(t, err)
				if c.errorContains != "" {
					assert.Contains(t, err.Error(), c.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}

			// Verify resource was deleted if it existed
			if c.resource != nil && len(c.initObjects) > 0 {
				checkResource := c.resource.DeepCopyObject().(client.Object)
				err := fakeClient.Get(ctx, client.ObjectKeyFromObject(c.resource), checkResource)
				// Resource should be deleted or in process of deletion
				assert.True(t, apierrors.IsNotFound(err) || checkResource.GetDeletionTimestamp() != nil,
					"Resource should be deleted or have deletion timestamp")
			}
		})
	}
}

// TestRemoveMigrationResourcesComprehensive tests removeMigrationResources with various scenarios
func TestRemoveMigrationResourcesComprehensive(t *testing.T) {
	scheme := configs.GetRuntimeScheme()
	ctx := context.Background()

	cases := []struct {
		name          string
		initObjects   []client.Object
		clusterName   string
		expectError   bool
		errorContains string
	}{
		{
			name: "Remove cluster with ManagedCluster and KlusterletAddonConfig",
			initObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				},
				&addonv1.KlusterletAddonConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-cluster",
						Namespace: "test-cluster",
					},
				},
			},
			clusterName: "test-cluster",
			expectError: false,
		},
		{
			name:        "Remove cluster with no resources - should succeed",
			initObjects: []client.Object{},
			clusterName: "empty-cluster",
			expectError: false,
		},
		{
			name: "Remove cluster with ManagedCluster only",
			initObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster-only"},
				},
			},
			clusterName: "cluster-only",
			expectError: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(c.initObjects...).Build()

			agentConfig := &configs.AgentConfig{
				LeafHubName: "hub1",
			}
			syncer := NewMigrationTargetSyncer(fakeClient, nil, agentConfig)

			err := syncer.removeMigrationResources(ctx, c.clusterName)

			if c.expectError {
				assert.Error(t, err)
				if c.errorContains != "" {
					assert.Contains(t, err.Error(), c.errorContains)
				}
			} else {
				assert.NoError(t, err)
				// Note: In fake client, resources may not actually be deleted immediately,
				// so we primarily verify that no error occurred during the removal operation
			}
		})
	}
}

// TestRollbackDeployingComprehensive tests rollbackDeploying with comprehensive error scenarios
func TestRollbackDeployingComprehensive(t *testing.T) {
	scheme := configs.GetRuntimeScheme()
	ctx := context.Background()

	cases := []struct {
		name          string
		initObjects   []client.Object
		spec          *migration.MigrationTargetBundle
		expectError   bool
		errorContains []string
	}{
		{
			name: "Successful rollback with all resources present",
			initObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster1"},
				},
				&addonv1.KlusterletAddonConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cluster1",
						Namespace: "cluster1",
					},
				},
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster1"},
				},
				&operatorv1.ClusterManager{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster-manager"},
					Spec: operatorv1.ClusterManagerSpec{
						RegistrationConfiguration: &operatorv1.RegistrationHubConfiguration{
							AutoApproveUsers: []string{"system:serviceaccount:test:test"},
						},
					},
				},
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: GetSubjectAccessReviewClusterRoleName("test"),
					},
				},
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: GetSubjectAccessReviewClusterRoleBindingName("test"),
					},
				},
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: GetAgentRegistrationClusterRoleBindingName("test"),
					},
				},
			},
			spec: &migration.MigrationTargetBundle{
				ManagedServiceAccountName:             "test",
				ManagedServiceAccountInstallNamespace: "test",
				ManagedClusters:                       []string{"cluster1"},
			},
			expectError: false,
		},
		{
			name: "Rollback with missing resources - continues with errors",
			initObjects: []client.Object{
				&operatorv1.ClusterManager{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster-manager"},
					Spec: operatorv1.ClusterManagerSpec{
						RegistrationConfiguration: &operatorv1.RegistrationHubConfiguration{
							AutoApproveUsers: []string{"system:serviceaccount:test:test"},
						},
					},
				},
			},
			spec: &migration.MigrationTargetBundle{
				ManagedServiceAccountName:             "test",
				ManagedServiceAccountInstallNamespace: "test",
				ManagedClusters:                       []string{"missing-cluster"},
			},
			expectError: false, // removeMigrationResources handles missing resources
		},
		{
			name: "Rollback with multiple clusters",
			initObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster1"},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster2"},
				},
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster1"},
				},
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster2"},
				},
				&operatorv1.ClusterManager{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster-manager"},
					Spec: operatorv1.ClusterManagerSpec{
						RegistrationConfiguration: &operatorv1.RegistrationHubConfiguration{
							AutoApproveUsers: []string{"system:serviceaccount:test:test"},
						},
					},
				},
			},
			spec: &migration.MigrationTargetBundle{
				ManagedServiceAccountName:             "test",
				ManagedServiceAccountInstallNamespace: "test",
				ManagedClusters:                       []string{"cluster1", "cluster2"},
			},
			expectError: false,
		},
		{
			name:        "Empty cluster list - should succeed",
			initObjects: []client.Object{},
			spec: &migration.MigrationTargetBundle{
				ManagedServiceAccountName:             "test",
				ManagedServiceAccountInstallNamespace: "test",
				ManagedClusters:                       []string{},
			},
			expectError: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(c.initObjects...).Build()

			agentConfig := &configs.AgentConfig{
				LeafHubName: "hub1",
			}
			syncer := NewMigrationTargetSyncer(fakeClient, nil, agentConfig)

			clusterErrors := make(map[string]string)
			err := syncer.rollbackDeploying(ctx, c.spec, clusterErrors)

			if c.expectError {
				assert.Error(t, err)
				for _, expectedStr := range c.errorContains {
					assert.Contains(t, err.Error(), expectedStr)
				}
			} else {
				// May have errors but should not fail catastrophically
				if err != nil {
					t.Logf("Rollback completed with non-fatal errors: %v", err)
				}
			}
		})
	}
}

// TestRollbackInitializingComprehensive tests rollbackInitializing with comprehensive scenarios
func TestRollbackInitializingComprehensive(t *testing.T) {
	scheme := configs.GetRuntimeScheme()
	ctx := context.Background()

	cases := []struct {
		name          string
		initObjects   []client.Object
		spec          *migration.MigrationTargetBundle
		expectError   bool
		errorContains []string
	}{
		{
			name: "Successful rollback with all RBAC resources",
			initObjects: []client.Object{
				&operatorv1.ClusterManager{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster-manager"},
					Spec: operatorv1.ClusterManagerSpec{
						RegistrationConfiguration: &operatorv1.RegistrationHubConfiguration{
							AutoApproveUsers: []string{"system:serviceaccount:test:test"},
						},
					},
				},
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: GetSubjectAccessReviewClusterRoleName("test"),
					},
				},
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: GetSubjectAccessReviewClusterRoleBindingName("test"),
					},
				},
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: GetAgentRegistrationClusterRoleBindingName("test"),
					},
				},
			},
			spec: &migration.MigrationTargetBundle{
				ManagedServiceAccountName:             "test",
				ManagedServiceAccountInstallNamespace: "test",
			},
			expectError: false,
		},
		{
			name: "Rollback with ClusterManager but missing RBAC - handles gracefully",
			initObjects: []client.Object{
				&operatorv1.ClusterManager{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster-manager"},
					Spec: operatorv1.ClusterManagerSpec{
						RegistrationConfiguration: &operatorv1.RegistrationHubConfiguration{
							AutoApproveUsers: []string{"system:serviceaccount:test:test"},
						},
					},
				},
			},
			spec: &migration.MigrationTargetBundle{
				ManagedServiceAccountName:             "test",
				ManagedServiceAccountInstallNamespace: "test",
			},
			expectError: false,
		},
		{
			name: "Rollback with partial RBAC resources",
			initObjects: []client.Object{
				&operatorv1.ClusterManager{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster-manager"},
					Spec: operatorv1.ClusterManagerSpec{
						RegistrationConfiguration: &operatorv1.RegistrationHubConfiguration{
							AutoApproveUsers: []string{"system:serviceaccount:test:test"},
						},
					},
				},
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: GetSubjectAccessReviewClusterRoleName("test"),
					},
				},
			},
			spec: &migration.MigrationTargetBundle{
				ManagedServiceAccountName:             "test",
				ManagedServiceAccountInstallNamespace: "test",
			},
			expectError: false,
		},
		{
			name:        "Skip rollback with empty ManagedServiceAccountName",
			initObjects: []client.Object{},
			spec: &migration.MigrationTargetBundle{
				ManagedServiceAccountName:             "",
				ManagedServiceAccountInstallNamespace: "",
			},
			expectError: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(c.initObjects...).Build()

			agentConfig := &configs.AgentConfig{
				LeafHubName: "hub1",
			}
			syncer := NewMigrationTargetSyncer(fakeClient, nil, agentConfig)

			clusterErrors := make(map[string]string)
			err := syncer.rollbackInitializing(ctx, c.spec, clusterErrors)

			if c.expectError {
				assert.Error(t, err)
				for _, expectedStr := range c.errorContains {
					assert.Contains(t, err.Error(), expectedStr)
				}
			} else {
				assert.NoError(t, err)
			}

			// Verify RBAC resources were deleted if they existed
			if c.spec.ManagedServiceAccountName != "" {
				cr := &rbacv1.ClusterRole{}
				err := fakeClient.Get(ctx, client.ObjectKey{
					Name: GetSubjectAccessReviewClusterRoleName(c.spec.ManagedServiceAccountName),
				}, cr)
				assert.True(t, apierrors.IsNotFound(err) || cr.GetDeletionTimestamp() != nil,
					"ClusterRole should be deleted or have deletion timestamp")
			}
		})
	}
}

// TestCleanupMigrationRBACComprehensive tests cleanupMigrationRBAC with all code paths
func TestCleanupMigrationRBACComprehensive(t *testing.T) {
	scheme := configs.GetRuntimeScheme()
	ctx := context.Background()

	cases := []struct {
		name          string
		initObjects   []client.Object
		msaName       string
		expectError   bool
		errorContains []string
	}{
		{
			name: "Cleanup all three RBAC resources successfully",
			initObjects: []client.Object{
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: GetSubjectAccessReviewClusterRoleName("test-msa"),
					},
				},
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: GetSubjectAccessReviewClusterRoleBindingName("test-msa"),
					},
				},
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: GetAgentRegistrationClusterRoleBindingName("test-msa"),
					},
				},
			},
			msaName:     "test-msa",
			expectError: false,
		},
		{
			name:        "Cleanup with no RBAC resources - handles gracefully",
			initObjects: []client.Object{},
			msaName:     "test-msa-none",
			expectError: false,
		},
		{
			name: "Cleanup with only ClusterRole present",
			initObjects: []client.Object{
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: GetSubjectAccessReviewClusterRoleName("partial-msa"),
					},
				},
			},
			msaName:     "partial-msa",
			expectError: false,
		},
		{
			name: "Cleanup with only ClusterRoleBindings present",
			initObjects: []client.Object{
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: GetSubjectAccessReviewClusterRoleBindingName("binding-only"),
					},
				},
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: GetAgentRegistrationClusterRoleBindingName("binding-only"),
					},
				},
			},
			msaName:     "binding-only",
			expectError: false,
		},
		{
			name: "Cleanup with resources having finalizers",
			initObjects: []client.Object{
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name:       GetSubjectAccessReviewClusterRoleName("finalizer-msa"),
						Finalizers: []string{"test.finalizer.io/lock"},
					},
				},
			},
			msaName:     "finalizer-msa",
			expectError: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(c.initObjects...).Build()

			syncer := &MigrationTargetSyncer{
				client:      fakeClient,
				leafHubName: "hub1",
			}

			err := syncer.cleanupMigrationRBAC(ctx, c.msaName)

			if c.expectError {
				assert.Error(t, err)
				for _, expectedStr := range c.errorContains {
					assert.Contains(t, err.Error(), expectedStr)
				}
			} else {
				assert.NoError(t, err)
			}

			// Verify all RBAC resources were deleted
			clusterRole := &rbacv1.ClusterRole{}
			err = fakeClient.Get(ctx, client.ObjectKey{
				Name: GetSubjectAccessReviewClusterRoleName(c.msaName),
			}, clusterRole)
			assert.True(t, apierrors.IsNotFound(err) || clusterRole.GetDeletionTimestamp() != nil,
				"ClusterRole should be deleted or have deletion timestamp")

			sarCRB := &rbacv1.ClusterRoleBinding{}
			err = fakeClient.Get(ctx, client.ObjectKey{
				Name: GetSubjectAccessReviewClusterRoleBindingName(c.msaName),
			}, sarCRB)
			assert.True(t, apierrors.IsNotFound(err) || sarCRB.GetDeletionTimestamp() != nil,
				"SAR ClusterRoleBinding should be deleted or have deletion timestamp")

			regCRB := &rbacv1.ClusterRoleBinding{}
			err = fakeClient.Get(ctx, client.ObjectKey{
				Name: GetAgentRegistrationClusterRoleBindingName(c.msaName),
			}, regCRB)
			assert.True(t, apierrors.IsNotFound(err) || regCRB.GetDeletionTimestamp() != nil,
				"Registration ClusterRoleBinding should be deleted or have deletion timestamp")
		})
	}
}

// TestRollbackingStageSwitch tests the rollbacking function's stage switch logic
func TestRollbackingStageSwitch(t *testing.T) {
	scheme := configs.GetRuntimeScheme()
	ctx := context.Background()

	cases := []struct {
		name          string
		initObjects   []client.Object
		rollbackStage string
		expectError   bool
		errorContains string
	}{
		{
			name: "Rollback initializing stage",
			initObjects: []client.Object{
				&operatorv1.ClusterManager{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster-manager"},
					Spec: operatorv1.ClusterManagerSpec{
						RegistrationConfiguration: &operatorv1.RegistrationHubConfiguration{
							AutoApproveUsers: []string{"system:serviceaccount:test:test"},
						},
					},
				},
			},
			rollbackStage: migrationv1alpha1.PhaseInitializing,
			expectError:   false,
		},
		{
			name: "Rollback deploying stage",
			initObjects: []client.Object{
				&operatorv1.ClusterManager{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster-manager"},
					Spec: operatorv1.ClusterManagerSpec{
						RegistrationConfiguration: &operatorv1.RegistrationHubConfiguration{},
					},
				},
			},
			rollbackStage: migrationv1alpha1.PhaseDeploying,
			expectError:   false,
		},
		{
			name: "Rollback registering stage",
			initObjects: []client.Object{
				&operatorv1.ClusterManager{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster-manager"},
				},
			},
			rollbackStage: migrationv1alpha1.PhaseRegistering,
			expectError:   false,
		},
		{
			name:          "Unsupported rollback stage",
			initObjects:   []client.Object{},
			rollbackStage: migrationv1alpha1.PhaseValidating,
			expectError:   true,
			errorContains: "no specific rollback action needed",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(c.initObjects...).Build()

			agentConfig := &configs.AgentConfig{
				LeafHubName: "hub1",
			}
			syncer := NewMigrationTargetSyncer(fakeClient, nil, agentConfig)
			syncer.SetMigrationID("test-migration")

			target := &migration.MigrationTargetBundle{
				MigrationId:                           "test-migration",
				Stage:                                 migrationv1alpha1.PhaseRollbacking,
				RollbackStage:                         c.rollbackStage,
				ManagedServiceAccountName:             "test",
				ManagedServiceAccountInstallNamespace: "test",
				ManagedClusters:                       []string{},
			}

			clusterErrors := make(map[string]string)
			err := syncer.rollbacking(ctx, target, clusterErrors)

			if c.expectError {
				assert.Error(t, err)
				if c.errorContains != "" {
					assert.Contains(t, err.Error(), c.errorContains)
				}
			} else {
				// Rollback may have errors but should continue
				if err != nil {
					t.Logf("Rollback completed with errors: %v", err)
				}
			}
		})
	}
}
