package generic

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/emitters"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

func TestSyncController_Reconcile_BasicOperations(t *testing.T) {
	configs.SetAgentConfig(&configs.AgentConfig{LeafHubName: "test-hub"})

	tests := []struct {
		name    string
		setup   func(client.Client) (*appsv1.Deployment, types.NamespacedName)
		wantErr bool
	}{
		{
			name: "create/update deployment",
			setup: func(c client.Client) (*appsv1.Deployment, types.NamespacedName) {
				deploy := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-deploy",
						Namespace: "test-ns",
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: int32Ptr(1),
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "test"},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"app": "test"},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{{
									Name:  "test",
									Image: "nginx",
								}},
							},
						},
					},
				}
				require.NoError(t, c.Create(context.Background(), deploy))
				return deploy, types.NamespacedName{Name: deploy.Name, Namespace: deploy.Namespace}
			},
		},
		{
			name: "delete deployment",
			setup: func(c client.Client) (*appsv1.Deployment, types.NamespacedName) {
				now := metav1.Now()
				deploy := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-deploy-del",
						Namespace:         "test-ns",
						DeletionTimestamp: &now,
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: int32Ptr(1),
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "test"},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"app": "test"},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{{
									Name:  "test",
									Image: "nginx",
								}},
							},
						},
					},
				}
				require.NoError(t, c.Create(context.Background(), deploy))
				return deploy, types.NamespacedName{Name: deploy.Name, Namespace: deploy.Namespace}
			},
		},
		{
			name: "deployment not found",
			setup: func(c client.Client) (*appsv1.Deployment, types.NamespacedName) {
				// Don't create the deployment, just return the name
				return nil, types.NamespacedName{Name: "non-existent", Namespace: "test-ns"}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
			emitter := emitters.NewObjectEmitter(enum.EventType("deployments"), nil)

			controller := &syncController{
				client:        fakeClient,
				emitter:       emitter,
				instance:      func() client.Object { return &appsv1.Deployment{} },
				leafHubName:   "test-hub",
				finalizerName: "test-finalizer",
			}

			_, namespacedName := tt.setup(fakeClient)
			request := ctrl.Request{NamespacedName: namespacedName}

			result, err := controller.Reconcile(context.Background(), request)
			if tt.wantErr {
				require.Error(t, err)
				require.Equal(t, ctrl.Result{Requeue: true, RequeueAfter: 5 * time.Second}, result)
			} else {
				require.NoError(t, err)
				require.Equal(t, ctrl.Result{}, result)
			}
		})
	}
}


func TestSyncController_Reconcile_EdgeCases(t *testing.T) {
	configs.SetAgentConfig(&configs.AgentConfig{LeafHubName: "test-hub"})

	t.Run("non-global resource with existing finalizer", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
		emitter := emitters.NewObjectEmitter(enum.EventType("deployments"), nil)

		controller := &syncController{
			client:        fakeClient,
			emitter:       emitter,
			instance:      func() client.Object { return &appsv1.Deployment{} },
			leafHubName:   "test-hub",
			finalizerName: "test-finalizer",
		}

		// Create a non-global resource that already has a finalizer
		deploy := createTestDeployment("test-deploy", "test-ns", nil, nil, nil)
		deploy.Finalizers = []string{"test-finalizer"}
		require.NoError(t, fakeClient.Create(context.Background(), deploy))

		request := ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: "test-ns",
				Name:      "test-deploy",
			},
		}

		// Mark it for deletion
		require.NoError(t, fakeClient.Delete(context.Background(), deploy))

		result, err := controller.Reconcile(context.Background(), request)
		require.NoError(t, err)
		require.Equal(t, ctrl.Result{}, result)

		// For non-global resources, the controller doesn't manage finalizers
		// So the existing finalizer should remain unchanged
		updatedDeploy := &appsv1.Deployment{}
		err = fakeClient.Get(context.Background(), request.NamespacedName, updatedDeploy)
		require.NoError(t, err)
		// The finalizer should still be there since it's not a global resource
		require.Contains(t, updatedDeploy.Finalizers, "test-finalizer")
	})

	t.Run("resource delete without finalizer", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
		emitter := emitters.NewObjectEmitter(enum.EventType("deployments"), nil)

		controller := &syncController{
			client:        fakeClient,
			emitter:       emitter,
			instance:      func() client.Object { return &appsv1.Deployment{} },
			leafHubName:   "test-hub",
			finalizerName: "test-finalizer",
		}

		// Create a test resource without finalizer
		deploy := createTestDeployment("test-deploy", "test-ns", nil, nil, nil)
		// No finalizers on this object
		require.NoError(t, fakeClient.Create(context.Background(), deploy))

		// Mark it for deletion
		require.NoError(t, fakeClient.Delete(context.Background(), deploy))

		request := ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: "test-ns",
				Name:      "test-deploy",
			},
		}

		result, err := controller.Reconcile(context.Background(), request)
		require.NoError(t, err)
		require.Equal(t, ctrl.Result{}, result)

		// Since there was no finalizer to remove, the object should be deleted immediately
		updatedDeploy := &appsv1.Deployment{}
		err = fakeClient.Get(context.Background(), request.NamespacedName, updatedDeploy)
		require.Error(t, err)
		require.True(t, errors.IsNotFound(err), "Object should be deleted when no finalizer to remove")
	})
}


func TestCleanObject(t *testing.T) {
	obj := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deploy",
			Namespace: "test-ns",
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "test-manager"},
			},
			Finalizers: []string{"test-finalizer"},
			OwnerReferences: []metav1.OwnerReference{
				{Name: "test-owner", Kind: "Test"},
			},
			SelfLink: "test-self-link",
		},
	}

	cleanObject(obj)

	require.Nil(t, obj.GetManagedFields())
	require.Nil(t, obj.GetFinalizers())
	require.Nil(t, obj.GetOwnerReferences())
	require.Empty(t, obj.GetSelfLink())
}

// Helper functions
func createTestDeployment(name, namespace string, labels, annotations map[string]string, deletionTimestamp *metav1.Time) *appsv1.Deployment {
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			Labels:            labels,
			Annotations:       annotations,
			DeletionTimestamp: deletionTimestamp,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "test",
						Image: "nginx",
					}},
				},
			},
		},
	}
	return deploy
}

func int32Ptr(i int32) *int32 {
	return &i
}
