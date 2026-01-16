// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package migration

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	addonv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
	"github.com/stretchr/testify/assert"
	rbacv1 "k8s.io/api/rbac/v1"
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
				// Bootstrap ClusterRole needed for dynamic ClusterRole detection
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: "open-cluster-management:managedcluster:bootstrap:agent-registration",
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
				// Bootstrap ClusterRole needed for dynamic ClusterRole detection
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: "open-cluster-management:managedcluster:bootstrap:agent-registration",
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
				// Bootstrap ClusterRole needed for dynamic ClusterRole detection
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: "open-cluster-management:managedcluster:bootstrap:agent-registration",
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
				// Bootstrap ClusterRole needed for dynamic ClusterRole detection
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: "open-cluster-management:managedcluster:bootstrap:agent-registration",
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
				// Bootstrap ClusterRole needed for dynamic ClusterRole detection
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: "open-cluster-management:managedcluster:bootstrap:agent-registration",
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
				// Bootstrap ClusterRole needed for dynamic ClusterRole detection
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: "open-cluster-management:managedcluster:bootstrap:agent-registration",
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
				// Bootstrap ClusterRole needed for dynamic ClusterRole detection
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: "open-cluster-management:managedcluster:bootstrap:agent-registration",
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

// TestGetBootstrapClusterRoleName tests the dynamic ClusterRole detection logic
func TestGetBootstrapClusterRoleName(t *testing.T) {
	ctx := context.Background()
	scheme := configs.GetRuntimeScheme()

	cases := []struct {
		name                    string
		initObjects             []client.Object
		expectedClusterRoleName string
		expectedError           string
	}{
		{
			name: "ACM ClusterRole exists - should return ACM ClusterRole name",
			initObjects: []client.Object{
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: DefaultACMBootstrapClusterRole,
					},
				},
			},
			expectedClusterRoleName: DefaultACMBootstrapClusterRole,
			expectedError:           "",
		},
		{
			name: "Only OCM ClusterRole exists - should return OCM ClusterRole name",
			initObjects: []client.Object{
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: DefaultOCMBootstrapClusterRole,
					},
				},
			},
			expectedClusterRoleName: DefaultOCMBootstrapClusterRole,
			expectedError:           "",
		},
		{
			name: "Both ACM and OCM ClusterRoles exist - should return ACM ClusterRole name (priority)",
			initObjects: []client.Object{
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: DefaultACMBootstrapClusterRole,
					},
				},
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: DefaultOCMBootstrapClusterRole,
					},
				},
			},
			expectedClusterRoleName: DefaultACMBootstrapClusterRole,
			expectedError:           "",
		},
		{
			name:                    "Neither ClusterRole exists - should return error",
			initObjects:             []client.Object{},
			expectedClusterRoleName: "",
			expectedError:           "no bootstrap ClusterRole found",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(c.initObjects...).Build()

			syncer := &MigrationTargetSyncer{
				client: fakeClient,
			}

			clusterRoleName, err := syncer.getBootstrapClusterRoleName(ctx)

			if c.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), c.expectedError)
				assert.Equal(t, "", clusterRoleName)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, c.expectedClusterRoleName, clusterRoleName)
			}
		})
	}
}

// TestInitializingWithOCMClusterRole tests initialization when only OCM ClusterRole exists
func TestInitializingWithOCMClusterRole(t *testing.T) {
	ctx := context.Background()
	scheme := configs.GetRuntimeScheme()

	initObjects := []client.Object{
		&operatorv1.ClusterManager{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cluster-manager",
			},
			Spec: operatorv1.ClusterManagerSpec{
				RegistrationImagePullSpec: "test",
				WorkImagePullSpec:         "test",
			},
		},
		// Only OCM ClusterRole exists (no ACM ClusterRole)
		&rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name: DefaultOCMBootstrapClusterRole,
			},
		},
	}

	migrationEvent := &migration.MigrationTargetBundle{
		MigrationId:                           "020340324302432049234023040320",
		Stage:                                 migrationv1alpha1.PhaseInitializing,
		ManagedServiceAccountName:             "test",
		ManagedServiceAccountInstallNamespace: "test",
	}

	producer := ProducerMock{}
	transportClient := &controller.TransportClient{}
	transportClient.SetProducer(&producer)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

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
	syncer := NewMigrationTargetSyncer(fakeClient, transportClient, agentConfig)
	configs.SetAgentConfig(&configs.AgentConfig{LeafHubName: "hub2"})

	syncer.SetMigrationID(migrationEvent.MigrationId)

	payload, err := json.Marshal(migrationEvent)
	assert.Nil(t, err)
	evt := utils.ToCloudEvent(constants.MigrationTargetMsgKey, constants.CloudEventGlobalHubClusterName,
		"hub2", payload)
	evt.SetTime(time.Now())
	err = syncer.Sync(ctx, &evt)
	assert.Nil(t, err)

	// Verify ClusterRoleBinding was created with OCM ClusterRole
	foundClusterRoleBinding := &rbacv1.ClusterRoleBinding{}
	err = fakeClient.Get(ctx, types.NamespacedName{Name: GetAgentRegistrationClusterRoleBindingName("test")}, foundClusterRoleBinding)
	assert.Nil(t, err)
	assert.Equal(t, DefaultOCMBootstrapClusterRole, foundClusterRoleBinding.RoleRef.Name)
}

// TestInitializingWithNoClusterRole tests initialization fails when no bootstrap ClusterRole exists
func TestInitializingWithNoClusterRole(t *testing.T) {
	ctx := context.Background()
	scheme := configs.GetRuntimeScheme()

	initObjects := []client.Object{
		&operatorv1.ClusterManager{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cluster-manager",
			},
			Spec: operatorv1.ClusterManagerSpec{
				RegistrationImagePullSpec: "test",
				WorkImagePullSpec:         "test",
			},
		},
		// No bootstrap ClusterRole exists
	}

	migrationEvent := &migration.MigrationTargetBundle{
		MigrationId:                           "020340324302432049234023040320",
		Stage:                                 migrationv1alpha1.PhaseInitializing,
		ManagedServiceAccountName:             "test",
		ManagedServiceAccountInstallNamespace: "test",
	}

	producer := ProducerMock{}
	transportClient := &controller.TransportClient{}
	transportClient.SetProducer(&producer)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

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
	syncer := NewMigrationTargetSyncer(fakeClient, transportClient, agentConfig)
	configs.SetAgentConfig(&configs.AgentConfig{LeafHubName: "hub2"})

	syncer.SetMigrationID(migrationEvent.MigrationId)

	payload, err := json.Marshal(migrationEvent)
	assert.Nil(t, err)
	evt := utils.ToCloudEvent(constants.MigrationTargetMsgKey, constants.CloudEventGlobalHubClusterName,
		"hub2", payload)
	evt.SetTime(time.Now())
	err = syncer.Sync(ctx, &evt)

	// Should fail because no bootstrap ClusterRole exists
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "no bootstrap ClusterRole found")
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
