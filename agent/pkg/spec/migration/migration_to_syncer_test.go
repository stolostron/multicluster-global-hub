// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package migration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	addonv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
	"github.com/stretchr/testify/assert"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
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
		{
			name: "Deploying resources: migrate cluster from hub1 to hub2",
			receivedMigrationEventBundle: migration.MigrationTargetBundle{
				MigrationId:                           "020340324302432049234023040320",
				Stage:                                 migrationv1alpha1.PhaseDeploying,
				ManagedServiceAccountName:             "test", // the migration cr name
				ManagedServiceAccountInstallNamespace: "test",
			},
			eventSource:   "source-hub",
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
			},
		},
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
	evt.SetTime(time.Now()) // Set event time to avoid time-based skipping in shouldSkipMigrationEvent

	scheme := configs.GetRuntimeScheme()
	ctx := context.Background()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
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
