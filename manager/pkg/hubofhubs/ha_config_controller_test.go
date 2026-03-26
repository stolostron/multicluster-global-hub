package hubofhubs

import (
	"context"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"open-cluster-management.io/managed-serviceaccount/apis/authentication/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

type mockProducer struct {
	sentEvents []cloudevents.Event
}

func (m *mockProducer) SendEvent(ctx context.Context, event cloudevents.Event) error {
	m.sentEvents = append(m.sentEvents, event)
	return nil
}

func (m *mockProducer) Reconnect(config *transport.TransportInternalConfig, topic string) error {
	return nil
}

func newTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = clusterv1.Install(scheme)
	_ = v1beta1.AddToScheme(scheme)
	return scheme
}

func TestReconcile_CreatesMSAInLocalCluster(t *testing.T) {
	scheme := newTestScheme()
	activeHub := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "hub1",
			Labels: map[string]string{
				constants.GHHubRoleLabelKey: constants.GHHubRoleActive,
			},
		},
		Spec: clusterv1.ManagedClusterSpec{
			ManagedClusterClientConfigs: []clusterv1.ClientConfig{
				{URL: "https://hub1.example.com:6443"},
			},
		},
	}
	localCluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "local-cluster",
		},
		Spec: clusterv1.ManagedClusterSpec{
			ManagedClusterClientConfigs: []clusterv1.ClientConfig{
				{URL: "https://global-hub.example.com:6443"},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(activeHub, localCluster).
		Build()
	producer := &mockProducer{}
	controller := &HAConfigController{
		Client:   fakeClient,
		Producer: producer,
		Scheme:   scheme,
	}

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "hub1"},
	})
	require.NoError(t, err)

	// MSA token not available yet, should requeue
	assert.Equal(t, haConfigRequeueInterval, result.RequeueAfter)

	// Verify MSA was created in local-cluster namespace (not hub1 namespace)
	msa := &v1beta1.ManagedServiceAccount{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name:      "ha-config-hub1",
		Namespace: "local-cluster",
	}, msa)
	require.NoError(t, err)
	assert.Equal(t, "ha-config", msa.Labels["owner"])
}

func TestReconcile_BootstrapSecretUsesLocalClusterURL(t *testing.T) {
	scheme := newTestScheme()
	activeHub := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "hub1",
			Labels: map[string]string{
				constants.GHHubRoleLabelKey: constants.GHHubRoleActive,
			},
		},
		Spec: clusterv1.ManagedClusterSpec{
			ManagedClusterClientConfigs: []clusterv1.ClientConfig{
				{URL: "https://hub1.example.com:6443"},
			},
		},
	}
	localCluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "local-cluster",
		},
		Spec: clusterv1.ManagedClusterSpec{
			ManagedClusterClientConfigs: []clusterv1.ClientConfig{
				{URL: "https://global-hub.example.com:6443"},
			},
		},
	}
	msaSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ha-config-hub1",
			Namespace: "local-cluster",
			Labels: map[string]string{
				constants.LabelKeyIsManagedServiceAccount: "true",
			},
		},
		Data: map[string][]byte{
			"ca.crt": []byte("test-ca-data"),
			"token":  []byte("test-token"),
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(activeHub, localCluster, msaSecret).
		Build()
	producer := &mockProducer{}
	controller := &HAConfigController{
		Client:   fakeClient,
		Producer: producer,
		Scheme:   scheme,
	}

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "hub1"},
	})
	require.NoError(t, err)
	assert.Zero(t, result.RequeueAfter)

	// Verify event was sent
	require.Len(t, producer.sentEvents, 1)
	evt := producer.sentEvents[0]
	assert.Equal(t, constants.HAConfigMsgKey, evt.Type())
	assert.Equal(t, constants.CloudEventGlobalHubClusterName, evt.Source())
	assert.Equal(t, "hub1", evt.Subject())
	assert.NotNil(t, evt.Extensions()[constants.CloudEventExtensionKeyExpireTime])
}

func TestReconcile_CleanupWhenLabelRemoved(t *testing.T) {
	scheme := newTestScheme()
	activeHub := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "hub1",
			Labels: map[string]string{},
		},
	}
	msa := &v1beta1.ManagedServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ha-config-hub1",
			Namespace: "local-cluster",
			Labels: map[string]string{
				"owner": "ha-config",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(activeHub, msa).
		Build()
	producer := &mockProducer{}
	controller := &HAConfigController{
		Client:   fakeClient,
		Producer: producer,
		Scheme:   scheme,
	}

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "hub1"},
	})
	require.NoError(t, err)

	// Verify MSA was deleted
	deletedMSA := &v1beta1.ManagedServiceAccount{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name:      "ha-config-hub1",
		Namespace: "local-cluster",
	}, deletedMSA)
	assert.True(t, apierrors.IsNotFound(err), "MSA should be deleted")
}

func TestReconcile_Idempotent(t *testing.T) {
	scheme := newTestScheme()
	activeHub := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "hub1",
			Labels: map[string]string{
				constants.GHHubRoleLabelKey: constants.GHHubRoleActive,
			},
		},
	}
	localCluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "local-cluster",
		},
		Spec: clusterv1.ManagedClusterSpec{
			ManagedClusterClientConfigs: []clusterv1.ClientConfig{
				{URL: "https://global-hub.example.com:6443"},
			},
		},
	}
	existingMSA := &v1beta1.ManagedServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ha-config-hub1",
			Namespace: "local-cluster",
			Labels: map[string]string{
				"owner": "ha-config",
			},
		},
	}
	msaSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ha-config-hub1",
			Namespace: "local-cluster",
			Labels: map[string]string{
				constants.LabelKeyIsManagedServiceAccount: "true",
			},
		},
		Data: map[string][]byte{
			"ca.crt": []byte("test-ca"),
			"token":  []byte("test-token"),
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(activeHub, localCluster, existingMSA, msaSecret).
		Build()
	producer := &mockProducer{}
	controller := &HAConfigController{
		Client:   fakeClient,
		Producer: producer,
		Scheme:   scheme,
	}

	// First reconcile
	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "hub1"},
	})
	require.NoError(t, err)

	// Second reconcile (idempotent)
	_, err = controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "hub1"},
	})
	require.NoError(t, err)

	assert.Len(t, producer.sentEvents, 2)
}

func TestReconcile_ClusterNotFound(t *testing.T) {
	scheme := newTestScheme()
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()
	producer := &mockProducer{}
	controller := &HAConfigController{
		Client:   fakeClient,
		Producer: producer,
		Scheme:   scheme,
	}

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "nonexistent"},
	})
	require.NoError(t, err)
	assert.Empty(t, producer.sentEvents)
}
