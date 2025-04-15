// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package syncers

import (
	"context"
	"encoding/json"
	"testing"

	klusterletv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	addonv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/controller"
)

func TestMigrationToSyncer(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add corev1 to scheme: %v", err)
	}
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add clientgoscheme to scheme: %v", err)
	}
	if err := clusterv1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add clusterv1 to scheme: %v", err)
	}
	if err := operatorv1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add operatorv1 to scheme: %v", err)
	}
	if err := klusterletv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add klusterletv1alpha1 to scheme: %v", err)
	}
	testPayload := []byte(`
{
	"managedServiceAccountName": "test",
	"managedServiceAccountInstallNamespace": "test"
}`)
	cases := []struct {
		name                          string
		initObjects                   []client.Object
		expectedClusterManager        *operatorv1.ClusterManager
		expectedClusterRole           *rbacv1.ClusterRole
		expectedClusterRoleBinding    *rbacv1.ClusterRoleBinding
		expectedSARClusterRoleBinding *rbacv1.ClusterRoleBinding
	}{
		{
			name: "migration with cluster manager having no registration configuration",
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
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(c.initObjects...).Build()
			managedClusterMigrationSyncer := NewManagedClusterMigrationToSyncer(client, nil)

			err := managedClusterMigrationSyncer.Sync(ctx, testPayload)
			if err != nil {
				t.Errorf("Failed to sync managed cluster migration: %v", err)
			}

			if c.expectedClusterManager != nil {
				foundClusterManager := &operatorv1.ClusterManager{}
				if err := client.Get(ctx, types.NamespacedName{Name: c.expectedClusterManager.Name}, foundClusterManager); err != nil {
					t.Errorf("Failed to get cluster manager: %v", err)
				}
				if !apiequality.Semantic.DeepDerivative(c.expectedClusterManager, foundClusterManager) {
					t.Errorf("Expected cluster manager %#v, but got %#v", c.expectedClusterManager, foundClusterManager)
				}
			}

			if c.expectedClusterRole != nil {
				foundClusterRole := &rbacv1.ClusterRole{}
				if err := client.Get(ctx, types.NamespacedName{Name: c.expectedClusterRole.Name}, foundClusterRole); err != nil {
					t.Errorf("Failed to get cluster role: %v", err)
				}
				if !apiequality.Semantic.DeepDerivative(c.expectedClusterRole, foundClusterRole) {
					t.Errorf("Expected cluster role %v, but got %v", c.expectedClusterRole, foundClusterRole)
				}
			}

			if c.expectedClusterRoleBinding != nil {
				foundClusterRoleBinding := &rbacv1.ClusterRoleBinding{}
				if err := client.Get(ctx, types.NamespacedName{Name: c.expectedClusterRoleBinding.Name}, foundClusterRoleBinding); err != nil {
					t.Errorf("Failed to get cluster role binding: %v", err)
				}
				if !apiequality.Semantic.DeepDerivative(c.expectedClusterRoleBinding, foundClusterRoleBinding) {
					t.Errorf("Expected cluster role binding %v, but got %v", c.expectedClusterRoleBinding, foundClusterRoleBinding)
				}
			}

			if c.expectedSARClusterRoleBinding != nil {
				foundSARClusterRoleBinding := &rbacv1.ClusterRoleBinding{}
				if err := client.Get(ctx, types.NamespacedName{Name: c.expectedSARClusterRoleBinding.Name}, foundSARClusterRoleBinding); err != nil {
					t.Errorf("Failed to get cluster role binding: %v", err)
				}
				if !apiequality.Semantic.DeepDerivative(c.expectedSARClusterRoleBinding, foundSARClusterRoleBinding) {
					t.Errorf("Expected cluster role binding %v, but got %v", c.expectedSARClusterRoleBinding, foundSARClusterRoleBinding)
				}
			}
		})
	}
}

func TestMigrationDestinationHubSyncer(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	configs.SetAgentConfig(&configs.AgentConfig{LeafHubName: "hub2"})
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add corev1 to scheme: %v", err)
	}
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add clientgoscheme to scheme: %v", err)
	}
	if err := clusterv1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add clusterv1 to scheme: %v", err)
	}
	if err := operatorv1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add operatorv1 to scheme: %v", err)
	}
	if err := klusterletv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add klusterletv1alpha1 to scheme: %v", err)
	}
	if err := addonv1.SchemeBuilder.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add addonv1 to scheme: %v", err)
	}

	cases := []struct {
		name                         string
		receivedMigrationEventBundle migration.ManagedClusterMigrationToEvent
		initObjects                  []client.Object
		expectedError                error
	}{
		{
			name: "Deploying resources: migrate cluster from hub1 to hub2",
			receivedMigrationEventBundle: migration.ManagedClusterMigrationToEvent{
				Stage:                                 migrationv1alpha1.MigrationResourceDeployed,
				ManagedServiceAccountName:             "test", // the migration cr name
				ManagedServiceAccountInstallNamespace: "test",
				KlusterletAddonConfig: &addonv1.KlusterletAddonConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cluster1",
						Namespace: "cluster1",
					},
				},
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
				Stage:                                 migrationv1alpha1.MigrationResourceCleaned,
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

			managedClusterMigrationSyncer := NewManagedClusterMigrationToSyncer(fakeClient, transportClient)

			payload, err := json.Marshal(c.receivedMigrationEventBundle)
			assert.Nil(t, err)
			if err != nil {
				t.Errorf("Failed to marshal payload of managed cluster migration: %v", err)
			}

			// sync managed cluster migration
			err = managedClusterMigrationSyncer.Sync(ctx, payload)
			if c.expectedError == nil {
				assert.Nil(t, err)
			} else {
				assert.EqualError(t, err, c.expectedError.Error())
			}
		})
	}
}
