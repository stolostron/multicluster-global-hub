// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package syncers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	addonv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		migrationEvent                *migration.ManagedClusterMigrationToEvent
		initObjects                   []client.Object
		expectedClusterManager        *operatorv1.ClusterManager
		expectedClusterRole           *rbacv1.ClusterRole
		expectedClusterRoleBinding    *rbacv1.ClusterRoleBinding
		expectedSARClusterRoleBinding *rbacv1.ClusterRoleBinding
	}{
		{
			name: "Initializing: migration with cluster manager having no registration configuration",
			migrationEvent: &migration.ManagedClusterMigrationToEvent{
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
					Name: "multicluster-global-hub-migration:test",
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
					Name: "agent-registration-clusterrolebinding:test",
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
					Name: "test-subjectaccessreviews-clusterrolebinding",
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
					Name:     "multicluster-global-hub-migration:test",
					APIGroup: "rbac.authorization.k8s.io",
				},
			},
		},
		{
			name: "migration with cluster manager having empty registration configuration",
			migrationEvent: &migration.ManagedClusterMigrationToEvent{
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
			migrationEvent: &migration.ManagedClusterMigrationToEvent{
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
			migrationEvent: &migration.ManagedClusterMigrationToEvent{
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
			migrationEvent: &migration.ManagedClusterMigrationToEvent{
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
			migrationEvent: &migration.ManagedClusterMigrationToEvent{
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
						Name: "multicluster-global-hub-migration:test",
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
						Name: "agent-registration-clusterrolebinding:test",
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
						Name: "test-subjectaccessreviews-clusterrolebinding",
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
						Name:     "multicluster-global-hub-migration:test",
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
					Name: "multicluster-global-hub-migration:test",
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
					Name: "agent-registration-clusterrolebinding:test",
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
					Name: "test-subjectaccessreviews-clusterrolebinding",
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
					Name:     "multicluster-global-hub-migration:test",
					APIGroup: "rbac.authorization.k8s.io",
				},
			},
		},
		{
			name: "migration with changed clusterrole and clusterrolebinding",
			migrationEvent: &migration.ManagedClusterMigrationToEvent{
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
						Name: "multicluster-global-hub-migration:test",
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
						Name: "agent-registration-clusterrolebinding:test",
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
						Name: "test-subjectaccessreviews-clusterrolebinding",
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
						Name:     "multicluster-global-hub-migration:test",
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
					Name: "multicluster-global-hub-migration:test",
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
					Name: "agent-registration-clusterrolebinding:test",
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
					Name: "test-subjectaccessreviews-clusterrolebinding",
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
					Name:     "multicluster-global-hub-migration:test",
					APIGroup: "rbac.authorization.k8s.io",
				},
			},
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
			managedClusterMigrationSyncer := NewMigrationTargetSyncer(client, transportClient, transportConfig)
			configs.SetAgentConfig(&configs.AgentConfig{LeafHubName: "hub2"})

			toEvent := c.migrationEvent
			payload, err := json.Marshal(toEvent)
			assert.Nil(t, err)
			evt := utils.ToCloudEvent(constants.MigrationTargetMsgKey, constants.CloudEventGlobalHubClusterName,
				"hub2", payload)
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
		receivedMigrationEventBundle migration.ManagedClusterMigrationToEvent
		initObjects                  []client.Object
		expectedError                error
	}{
		{
			name: "Deploying resources: migrate cluster from hub1 to hub2",
			receivedMigrationEventBundle: migration.ManagedClusterMigrationToEvent{
				MigrationId:                           "020340324302432049234023040320",
				Stage:                                 migrationv1alpha1.ConditionTypeDeployed,
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
			},
		},
		{
			name: "Cleaning up resources: migrate cluster from hub1 to hub2",
			receivedMigrationEventBundle: migration.ManagedClusterMigrationToEvent{
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
						Name: "multicluster-global-hub-migration:test",
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
						Name: "agent-registration-clusterrolebinding:test",
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
						Name: "test-subjectaccessreviews-clusterrolebinding",
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
						Name:     "multicluster-global-hub-migration:test",
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

			managedClusterMigrationSyncer := NewMigrationTargetSyncer(fakeClient, transportClient, transportConfig)

			payload, err := json.Marshal(c.receivedMigrationEventBundle)
			assert.Nil(t, err)
			if err != nil {
				t.Errorf("Failed to marshal payload of managed cluster migration: %v", err)
			}

			// sync managed cluster migration
			evt := utils.ToCloudEvent(constants.MigrationTargetMsgKey, constants.CloudEventGlobalHubClusterName,
				"hub2", payload)
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

	evt := utils.ToCloudEvent("test", "hub1", "hub2", migration.SourceClusterMigrationResources{
		MigrationId: migrationId,
		ManagedClusters: []clusterv1.ManagedCluster{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster1",
				},
				Spec: clusterv1.ManagedClusterSpec{
					HubAcceptsClient:     true,
					LeaseDurationSeconds: 60,
				},
			},
		},
		KlusterletAddonConfig: []addonv1.KlusterletAddonConfig{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cluster1",
					Namespace: "cluster1",
				},
			},
		},
	})

	scheme := configs.GetRuntimeScheme()
	ctx := context.Background()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	// set agent config
	configs.SetAgentConfig(&configs.AgentConfig{LeafHubName: "hub2"})
	// set tranport config
	transportConfig := &transport.TransportInternalConfig{KafkaCredential: &transport.KafkaConfig{StatusTopic: "status"}}
	syncer := NewMigrationTargetSyncer(fakeClient, nil, transportConfig)
	syncer.currentMigrationId = migrationId
	err := syncer.Sync(ctx, &evt)
	assert.Nil(t, err)

	// verify the resources
	cluster := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{
		Name: "cluster1",
	}}
	err = fakeClient.Get(ctx, client.ObjectKeyFromObject(cluster), cluster)
	assert.Nil(t, err)
	assert.True(t, cluster.Spec.HubAcceptsClient)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "cluster1",
		},
	}
	err = fakeClient.Get(ctx, client.ObjectKeyFromObject(secret), secret)
	assert.Nil(t, err)
	assert.Equal(t, "secret", secret.StringData["test"])
}

// go test -timeout 30s -run ^TestRegistering$ github.com/stolostron/multicluster-global-hub/agent/pkg/spec/syncers -v
func TestRegistering(t *testing.T) {
	ctx := context.Background()
	scheme := configs.GetRuntimeScheme()

	cases := []struct {
		name                 string
		initObjects          []client.Object
		migrationEvent       *migration.ManagedClusterMigrationToEvent
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
			migrationEvent: &migration.ManagedClusterMigrationToEvent{
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
			migrationEvent: &migration.ManagedClusterMigrationToEvent{
				ManagedClusters: []string{"cluster1", "cluster2"},
			},
			expectedError: "manifestwork(*-klusterlet) are not applied in these clusters: [cluster2]",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// change the resitering timeout
			registeringTimeout = 10 * time.Second
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(c.initObjects...).Build()

			managedClusterMigrationSyncer := NewMigrationTargetSyncer(fakeClient, nil, nil)

			err := managedClusterMigrationSyncer.registering(ctx, c.migrationEvent)
			if c.expectedError == "" {
				assert.Nil(t, err)
			} else {
				assert.NotNil(t, err)
				assert.Contains(t, err.Error(), c.expectedError)
			}
		})
	}
}
