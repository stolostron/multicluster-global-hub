package syncers

import (
	"context"
	"testing"

	klusterletv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestMigrationFromSyncer(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = clientgoscheme.AddToScheme(scheme)
	_ = clusterv1.AddToScheme(scheme)
	_ = operatorv1.AddToScheme(scheme)
	_ = klusterletv1alpha1.AddToScheme(scheme)
	testPayload := []byte(`
{
	"managedclusters": [
		"test"
	],
	"bootstrapsecret": {
		"apiVersion": "v1",
		"kind": "Secret",
		"metadata": {
			"name": "test",
			"namespace": "test"
		},
		"data": {
			"test": "dGVzdA=="
		}
	},
	"klusterletconfig": {
		"apiVersion": "klusterletconfig.open-cluster-management.io/v1alpha1",
		"kind": "KlusterletConfig",
		"metadata": {
			"name": "test"
		},
		"spec": {
			"bootstrapKubeConfigs": {
				"type": "LocalSecrets",
				"localSecrets": {
					"kubeConfigSecrets": [
						{
							"name": "test"
						}
					]
				}
			}
		}
	}
}`)

	cases := []struct {
		name                          string
		initObjects                   []client.Object
		expectedBootstrapSecret       *corev1.Secret
		expectedBootstrapBackupSecret *corev1.Secret
		expectedKlusterletConfig      *klusterletv1alpha1.KlusterletConfig
		expectedManagedCluster        *clusterv1.ManagedCluster
	}{
		{
			name: "migration without existing bootstrap secret",
			initObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient:     true,
						LeaseDurationSeconds: 60,
					},
				},
			},
			expectedBootstrapSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
				},
				Data: map[string][]byte{
					"test": []byte("test"),
				},
			},
			expectedBootstrapBackupSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "test",
				},
				Data: map[string][]byte{
					"test": []byte("test"),
				},
			},
			expectedKlusterletConfig: &klusterletv1alpha1.KlusterletConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: klusterletv1alpha1.KlusterletConfigSpec{
					BootstrapKubeConfigs: operatorv1.BootstrapKubeConfigs{
						Type: operatorv1.LocalSecrets,
						LocalSecrets: operatorv1.LocalSecretsConfig{
							KubeConfigSecrets: []operatorv1.KubeConfigSecret{
								{
									Name: "test",
								},
								{
									Name: "test-backup",
								},
							},
						},
					},
				},
			},
			expectedManagedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Annotations: map[string]string{
						"agent.open-cluster-management.io/klusterlet-config": "test",
					},
				},
				Spec: clusterv1.ManagedClusterSpec{
					HubAcceptsClient:     true,
					LeaseDurationSeconds: 60,
				},
			},
		},
		{
			name: "migration with existing bootstrap secret",
			initObjects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Data: map[string][]byte{
						"test": []byte("foo"),
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient:     true,
						LeaseDurationSeconds: 60,
					},
				},
			},
			expectedBootstrapSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
				},
				Data: map[string][]byte{
					"test": []byte("test"),
				},
			},
			expectedBootstrapBackupSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "test",
				},
				Data: map[string][]byte{
					"test": []byte("test"),
				},
			},
			expectedKlusterletConfig: &klusterletv1alpha1.KlusterletConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: klusterletv1alpha1.KlusterletConfigSpec{
					BootstrapKubeConfigs: operatorv1.BootstrapKubeConfigs{
						Type: operatorv1.LocalSecrets,
						LocalSecrets: operatorv1.LocalSecretsConfig{
							KubeConfigSecrets: []operatorv1.KubeConfigSecret{
								{
									Name: "test",
								},
								{
									Name: "test-backup",
								},
							},
						},
					},
				},
			},
			expectedManagedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Annotations: map[string]string{
						"agent.open-cluster-management.io/klusterlet-config": "test",
					},
				},
				Spec: clusterv1.ManagedClusterSpec{
					HubAcceptsClient:     true,
					LeaseDurationSeconds: 60,
				},
			},
		},
		{
			name: "migration with existing klusterlet config",
			initObjects: []client.Object{
				&klusterletv1alpha1.KlusterletConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
					Spec: klusterletv1alpha1.KlusterletConfigSpec{
						BootstrapKubeConfigs: operatorv1.BootstrapKubeConfigs{
							Type: operatorv1.LocalSecrets,
							LocalSecrets: operatorv1.LocalSecretsConfig{
								KubeConfigSecrets: []operatorv1.KubeConfigSecret{
									{
										Name: "test",
									},
									{
										Name: "test-backup",
									},
								},
							},
						},
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient:     true,
						LeaseDurationSeconds: 60,
					},
				},
			},
			expectedBootstrapSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
				},
				Data: map[string][]byte{
					"test": []byte("test"),
				},
			},
			expectedBootstrapBackupSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "test",
				},
				Data: map[string][]byte{
					"test": []byte("test"),
				},
			},
			expectedKlusterletConfig: &klusterletv1alpha1.KlusterletConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: klusterletv1alpha1.KlusterletConfigSpec{
					BootstrapKubeConfigs: operatorv1.BootstrapKubeConfigs{
						Type: operatorv1.LocalSecrets,
						LocalSecrets: operatorv1.LocalSecretsConfig{
							KubeConfigSecrets: []operatorv1.KubeConfigSecret{
								{
									Name: "test",
								},
								{
									Name: "test-backup",
								},
							},
						},
					},
				},
			},
			expectedManagedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Annotations: map[string]string{
						"agent.open-cluster-management.io/klusterlet-config": "test",
					},
				},
				Spec: clusterv1.ManagedClusterSpec{
					HubAcceptsClient:     true,
					LeaseDurationSeconds: 60,
				},
			},
		},
		{
			name: "migration with manager cluster having annotation",
			initObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
						Annotations: map[string]string{
							"open-cluster-management.io/klusterlet": "test",
						},
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient:     true,
						LeaseDurationSeconds: 60,
					},
				},
			},
			expectedBootstrapSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
				},
				Data: map[string][]byte{
					"test": []byte("test"),
				},
			},
			expectedBootstrapBackupSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "test",
				},
				Data: map[string][]byte{
					"test": []byte("test"),
				},
			},
			expectedKlusterletConfig: &klusterletv1alpha1.KlusterletConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: klusterletv1alpha1.KlusterletConfigSpec{
					BootstrapKubeConfigs: operatorv1.BootstrapKubeConfigs{
						Type: operatorv1.LocalSecrets,
						LocalSecrets: operatorv1.LocalSecretsConfig{
							KubeConfigSecrets: []operatorv1.KubeConfigSecret{
								{
									Name: "test",
								},
								{
									Name: "test-backup",
								},
							},
						},
					},
				},
			},
			expectedManagedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Annotations: map[string]string{
						"agent.open-cluster-management.io/klusterlet-config": "test",
					},
				},
				Spec: clusterv1.ManagedClusterSpec{
					HubAcceptsClient:     true,
					LeaseDurationSeconds: 60,
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(c.initObjects...).Build()
			managedClusterMigrationSyncer := NewManagedClusterMigrationFromSyncer(client)

			err := managedClusterMigrationSyncer.Sync(ctx, testPayload)
			if err != nil {
				t.Errorf("Failed to sync managed cluster migration: %v", err)
			}

			if c.expectedBootstrapSecret != nil {
				foundBootstrapSecret := &corev1.Secret{}
				if err := client.Get(ctx, types.NamespacedName{Name: c.expectedBootstrapSecret.Name, Namespace: c.expectedBootstrapSecret.Namespace}, foundBootstrapSecret); err != nil {
					t.Errorf("Failed to get bootstrap secret: %v", err)
				}
				if !apiequality.Semantic.DeepDerivative(c.expectedBootstrapSecret, foundBootstrapSecret) {
					t.Errorf("Expected bootstrap secret %v, but got %v", c.expectedBootstrapSecret, foundBootstrapSecret)
				}
			}

			if c.expectedBootstrapBackupSecret != nil {
				foundBootstrapBackupSecret := &corev1.Secret{}
				if err := client.Get(ctx, types.NamespacedName{Name: c.expectedBootstrapBackupSecret.Name, Namespace: c.expectedBootstrapBackupSecret.Namespace}, foundBootstrapBackupSecret); err != nil {
					t.Errorf("Failed to get bootstrap backup secret: %v", err)
				}
				if !apiequality.Semantic.DeepDerivative(c.expectedBootstrapBackupSecret, foundBootstrapBackupSecret) {
					t.Errorf("Expected bootstrap backup secret %v, but got %v", c.expectedBootstrapBackupSecret, foundBootstrapBackupSecret)
				}
			}

			if c.expectedKlusterletConfig != nil {
				foundKlusterletConfig := &klusterletv1alpha1.KlusterletConfig{}
				if err := client.Get(ctx, types.NamespacedName{Name: c.expectedKlusterletConfig.Name}, foundKlusterletConfig); err != nil {
					t.Errorf("Failed to get klusterlet config: %v", err)
				}
				if !apiequality.Semantic.DeepDerivative(c.expectedKlusterletConfig, foundKlusterletConfig) {
					t.Errorf("Expected klusterlet config %v, but got %v", c.expectedKlusterletConfig, foundKlusterletConfig)
				}
			}

			if c.expectedManagedCluster != nil {
				foundManagedCluster := &clusterv1.ManagedCluster{}
				if err := client.Get(ctx, types.NamespacedName{Name: c.expectedManagedCluster.Name}, foundManagedCluster); err != nil {
					t.Errorf("Failed to get managed cluster: %v", err)
				}
				if !apiequality.Semantic.DeepDerivative(c.expectedManagedCluster, foundManagedCluster) {
					t.Errorf("Expected managed cluster %v, but got %v", c.expectedManagedCluster, foundManagedCluster)
				}
			}
		})
	}
}
