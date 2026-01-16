package generic

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/emitters"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
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
				emitters:      []emitters.Emitter{emitter},
				instance:      func() client.Object { return &appsv1.Deployment{} },
				leafHubName:   "test-hub",
				finalizerName: constants.GlobalHubCleanupFinalizer,
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

func int32Ptr(i int32) *int32 {
	return &i
}
