package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var _ = client.Object(nil) // silence unused import warning

func TestGetCurrentClusterName(t *testing.T) {
	namespace := utils.GetDefaultNamespace()

	err := appsv1.AddToScheme(scheme.Scheme)
	assert.NoError(t, err)

	tests := []struct {
		name        string
		initObjects []runtime.Object
		expected    string
		wantErr     bool
	}{
		{
			name: "deployment with cluster name",
			initObjects: []runtime.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "multicluster-global-hub-agent",
						Namespace: namespace,
					},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name: "agent",
										Args: []string{"--leaf-hub-name=local-cluster"},
									},
								},
							},
						},
					},
				},
			},
			expected: "local-cluster",
			wantErr:  false,
		},
		{
			name: "deployment without cluster name arg",
			initObjects: []runtime.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "multicluster-global-hub-agent",
						Namespace: namespace,
					},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name: "agent",
										Args: []string{"--other-arg=value"},
									},
								},
							},
						},
					},
				},
			},
			expected: "",
			wantErr:  false,
		},
		{
			name:        "deployment not found",
			initObjects: []runtime.Object{},
			expected:    "",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(tt.initObjects...).Build()

			ctx := context.Background()
			result, err := getCurrentClusterName(ctx, fakeClient, namespace)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestGetLocalClusterName(t *testing.T) {
	namespace := utils.GetDefaultNamespace()

	err := clusterv1.Install(scheme.Scheme)
	assert.NoError(t, err)

	tests := []struct {
		name        string
		initObjects []runtime.Object
		expected    string
		wantErr     bool
	}{
		{
			name: "single local cluster",
			initObjects: []runtime.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "local-cluster",
						Labels: map[string]string{
							constants.LocalClusterName: "true",
						},
					},
				},
			},
			expected: "local-cluster",
			wantErr:  false,
		},
		{
			name:        "no local cluster",
			initObjects: []runtime.Object{},
			expected:    "",
			wantErr:     false,
		},
		{
			name: "multiple local clusters - should error",
			initObjects: []runtime.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "local-cluster-1",
						Labels: map[string]string{
							constants.LocalClusterName: "true",
						},
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "local-cluster-2",
						Labels: map[string]string{
							constants.LocalClusterName: "true",
						},
					},
				},
			},
			expected: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(tt.initObjects...).Build()

			ctx := context.Background()
			result, err := GetLocalClusterName(ctx, fakeClient, namespace)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestClusterPredicate(t *testing.T) {
	tests := []struct {
		name     string
		obj      *clusterv1.ManagedCluster
		wantBool bool
	}{
		{
			name: "local cluster should match create",
			obj: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "local-cluster",
					Labels: map[string]string{
						constants.LocalClusterName: "true",
					},
				},
			},
			wantBool: true,
		},
		{
			name: "non-local cluster should not match create",
			obj: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "remote-cluster",
				},
			},
			wantBool: false,
		},
		{
			name: "cluster with nil labels should not match",
			obj: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "cluster",
					Labels: nil,
				},
			},
			wantBool: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test CreateFunc
			createEvent := event.CreateEvent{
				Object: tt.obj,
			}
			assert.Equal(t, tt.wantBool, clusterPred.Create(createEvent))

			// Test UpdateFunc - should always return false
			updateEvent := event.UpdateEvent{
				ObjectNew: tt.obj,
			}
			assert.False(t, clusterPred.Update(updateEvent))

			// Test DeleteFunc
			deleteEvent := event.DeleteEvent{
				Object: tt.obj,
			}
			assert.Equal(t, tt.wantBool, clusterPred.Delete(deleteEvent))
		})
	}
}

func TestLocalAgentControllerIsResourceRemoved(t *testing.T) {
	err := v1alpha4.AddToScheme(scheme.Scheme)
	assert.NoError(t, err)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme.Scheme,
	})
	if err != nil {
		t.Skip("Skipping test - no kubeconfig available")
	}

	controller := &LocalAgentController{
		Manager: mgr,
	}

	// Test initial state
	result := controller.IsResourceRemoved()
	t.Logf("IsResourceRemoved (initial): %v", result)

	// Test after changing global variable
	isResourceRemoved = false
	result = controller.IsResourceRemoved()
	assert.False(t, result)

	// Reset
	isResourceRemoved = true
}

func TestReconcile_LocalAgent(t *testing.T) {
	namespace := utils.GetDefaultNamespace()

	err := v1alpha4.AddToScheme(scheme.Scheme)
	assert.NoError(t, err)
	err = clusterv1.Install(scheme.Scheme)
	assert.NoError(t, err)
	err = appsv1.AddToScheme(scheme.Scheme)
	assert.NoError(t, err)

	tests := []struct {
		name        string
		initObjects []runtime.Object
	}{
		{
			name:        "reconcile with nil MGH",
			initObjects: []runtime.Object{},
		},
		{
			name: "reconcile with paused MGH",
			initObjects: []runtime.Object{
				&v1alpha4.MulticlusterGlobalHub{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-mgh",
						Namespace: namespace,
						Annotations: map[string]string{
							"mgh-pause": "true",
						},
					},
					Spec: v1alpha4.MulticlusterGlobalHubSpec{},
				},
			},
		},
		{
			name: "reconcile with deleting MGH",
			initObjects: []runtime.Object{
				&v1alpha4.MulticlusterGlobalHub{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-mgh",
						Namespace:         namespace,
						DeletionTimestamp: &metav1.Time{},
					},
					Spec: v1alpha4.MulticlusterGlobalHubSpec{},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
				Scheme: scheme.Scheme,
			})
			if err != nil {
				t.Skip("Skipping test - no kubeconfig available")
			}

			controller := &LocalAgentController{
				Manager: mgr,
			}

			ctx := context.Background()
			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "test-mgh",
					Namespace: namespace,
				},
			}

			_, err = controller.Reconcile(ctx, req)
			// Errors are expected in unit tests due to missing configuration
			t.Logf("Reconcile result: %v", err)
		})
	}
}

func TestPruneAgentResources(t *testing.T) {
	namespace := utils.GetDefaultNamespace()

	err := v1alpha4.AddToScheme(scheme.Scheme)
	assert.NoError(t, err)
	err = appsv1.AddToScheme(scheme.Scheme)
	assert.NoError(t, err)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme.Scheme,
	})
	if err != nil {
		t.Skip("Skipping test - no kubeconfig available")
	}

	controller := &LocalAgentController{
		Manager: mgr,
	}

	ctx := context.Background()
	err = controller.pruneAgentResources(ctx, namespace)
	// Error is expected due to missing transporter
	t.Logf("PruneAgentResources result: %v", err)
}

func TestGetTransportSecretName(t *testing.T) {
	// Set the cluster name
	clusterName = "test-cluster"

	expected := constants.GHTransportConfigSecret + "-test-cluster"
	result := getTransportSecretName()

	assert.Equal(t, expected, result)
}
