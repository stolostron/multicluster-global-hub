package syncers

import (
	"context"
	"math/rand/v2"
	"testing"
	"time"

	klusterletv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
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
		// expectedManagedCluster        *clusterv1.ManagedCluster
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
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-backup",
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
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(c.initObjects...).WithObjects(c.initObjects...).Build()
			managedClusterMigrationSyncer := NewManagedClusterMigrationFromSyncer(client)

			// start a new go routine to mimic manager cluster condition update every 100ms
			go func() {
				for {
					mcl := &clusterv1.ManagedCluster{}
					if err := client.Get(ctx, types.NamespacedName{Name: "test"}, mcl); err != nil {
						t.Errorf("Failed to get managed cluster: %v", err)
					}
					randInt := rand.IntN(1000)
					time.Sleep(time.Duration(randInt) * time.Millisecond)
					annotations := mcl.GetAnnotations()
					if annotations != nil && annotations["agent.open-cluster-management.io/klusterlet-config"] == "test" {
						if randInt > 500 {
							_ = client.Delete(ctx, mcl)
							break
						}
						if meta.SetStatusCondition(&mcl.Status.Conditions, metav1.Condition{
							Type:   clusterv1.ManagedClusterConditionAvailable,
							Status: metav1.ConditionUnknown,
						}) {
							if err := client.Status().Update(ctx, mcl); err != nil {
								continue
							} else {
								break
							}
						}
					}
				}
			}()

			// sync managed cluster migration
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
		})
	}
}
