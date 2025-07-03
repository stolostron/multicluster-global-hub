package generic

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
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

func TestSyncController_Reconcile_ObjectUpdate(t *testing.T) {
	configs.SetAgentConfig(&configs.AgentConfig{LeafHubName: "test-hub"})

	fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()

	// Create a pod in the fake client
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-ns",
			UID:       "test-uid",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "test-container",
					Image: "nginx",
				},
			},
		},
	}

	err := fakeClient.Create(context.Background(), pod)
	require.NoError(t, err)

	emitter := emitters.NewObjectEmitter(enum.EventType("test"), nil)

	controller := &syncController{
		client:        fakeClient,
		emitter:       emitter,
		instance:      func() client.Object { return &corev1.Pod{} },
		leafHubName:   "test-hub",
		finalizerName: constants.GlobalHubCleanupFinalizer,
	}

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "test-ns",
			Name:      "test-pod",
		},
	}

	result, err := controller.Reconcile(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, ctrl.Result{}, result)
}

func TestSyncController_Reconcile_ObjectDeletion(t *testing.T) {
	configs.SetAgentConfig(&configs.AgentConfig{LeafHubName: "test-hub"})

	fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()

	// Create a pod with deletion timestamp
	now := metav1.Now()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-pod",
			Namespace:         "test-ns",
			UID:               "test-uid",
			DeletionTimestamp: &now,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "test-container",
					Image: "nginx",
				},
			},
		},
	}

	err := fakeClient.Create(context.Background(), pod)
	require.NoError(t, err)

	emitter := emitters.NewObjectEmitter(enum.EventType("test"), nil)

	controller := &syncController{
		client:        fakeClient,
		emitter:       emitter,
		instance:      func() client.Object { return &corev1.Pod{} },
		leafHubName:   "test-hub",
		finalizerName: constants.GlobalHubCleanupFinalizer,
	}

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "test-ns",
			Name:      "test-pod",
		},
	}

	result, err := controller.Reconcile(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, ctrl.Result{}, result)
}

func TestSyncController_Reconcile_ObjectNotFound(t *testing.T) {
	configs.SetAgentConfig(&configs.AgentConfig{LeafHubName: "test-hub"})

	fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()

	emitter := emitters.NewObjectEmitter(enum.EventType("test"), nil)

	controller := &syncController{
		client:        fakeClient,
		emitter:       emitter,
		instance:      func() client.Object { return &corev1.Pod{} },
		leafHubName:   "test-hub",
		finalizerName: constants.GlobalHubCleanupFinalizer,
	}

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "test-ns",
			Name:      "test-pod",
		},
	}

	result, err := controller.Reconcile(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, ctrl.Result{}, result)
}
