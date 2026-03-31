package hubha

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	haconfigbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/haconfig"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

func newHAConfigTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = clientgoscheme.AddToScheme(scheme)
	_ = clusterv1.Install(scheme)
	_ = mchv1.SchemeBuilder.AddToScheme(scheme)
	return scheme
}

func newHAConfigTestEvent(t *testing.T, bundle *haconfigbundle.HAConfigBundle) *cloudevents.Event {
	t.Helper()
	data, err := json.Marshal(bundle)
	require.NoError(t, err, "failed to marshal test bundle")
	evt := cloudevents.NewEvent()
	evt.SetType("HAConfig")
	evt.SetSource("local-cluster")
	evt.SetSubject("hub1")
	evt.SetExtension(constants.CloudEventExtensionKeyExpireTime,
		time.Now().Add(10*time.Minute).Format(time.RFC3339))
	_ = evt.SetData(cloudevents.ApplicationJSON, data)
	return &evt
}

func newHAConfigTestBundle() *haconfigbundle.HAConfigBundle {
	return &haconfigbundle.HAConfigBundle{
		BootstrapSecret: &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "bootstrap-ha-local-cluster",
				Namespace: "multicluster-engine",
			},
			Data: map[string][]byte{
				"kubeconfig": []byte("test-kubeconfig"),
			},
		},
	}
}

func newHAConfigSyncer(fakeClient client.Client) *HAConfigSyncer {
	return &HAConfigSyncer{
		client:      fakeClient,
		leafHubName: "hub1",
	}
}

func TestSync_BootstrapSecretCreatedInMCE(t *testing.T) {
	scheme := newHAConfigTestScheme()
	mch := &mchv1.MultiClusterHub{
		ObjectMeta: metav1.ObjectMeta{Name: "multiclusterhub"},
		Status:     mchv1.MultiClusterHubStatus{CurrentVersion: "2.14.0"},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mch).
		WithStatusSubresource(mch).
		Build()

	err := newHAConfigSyncer(fakeClient).Sync(context.Background(), newHAConfigTestEvent(t, newHAConfigTestBundle()))
	require.NoError(t, err)

	secret := &corev1.Secret{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name:      "bootstrap-ha-local-cluster",
		Namespace: "multicluster-engine",
	}, secret)
	require.NoError(t, err)
	assert.Equal(t, []byte("test-kubeconfig"), secret.Data["kubeconfig"])
}

func TestSync_KlusterletConfigCreated_214(t *testing.T) {
	scheme := newHAConfigTestScheme()
	mch := &mchv1.MultiClusterHub{
		ObjectMeta: metav1.ObjectMeta{Name: "multiclusterhub"},
		Status:     mchv1.MultiClusterHubStatus{CurrentVersion: "2.14.0"},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mch).
		WithStatusSubresource(mch).
		Build()

	err := newHAConfigSyncer(fakeClient).Sync(context.Background(), newHAConfigTestEvent(t, newHAConfigTestBundle()))
	require.NoError(t, err)

	klusterletConfig := &unstructured.Unstructured{}
	klusterletConfig.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "config.open-cluster-management.io",
		Version: "v1alpha1",
		Kind:    "KlusterletConfig",
	})
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: "ha-standby-local-cluster",
	}, klusterletConfig)
	require.NoError(t, err)

	spec, ok := klusterletConfig.Object["spec"].(map[string]interface{})
	require.True(t, ok)
	multipleHubsConfig, ok := spec["multipleHubsConfig"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "IncludeCurrentHub", multipleHubsConfig["genBootstrapKubeConfigStrategy"])
}

func TestSync_KlusterletConfigCreated_213(t *testing.T) {
	scheme := newHAConfigTestScheme()
	mch := &mchv1.MultiClusterHub{
		ObjectMeta: metav1.ObjectMeta{Name: "multiclusterhub"},
		Status:     mchv1.MultiClusterHubStatus{CurrentVersion: "2.13.5"},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mch).
		WithStatusSubresource(mch).
		Build()

	err := newHAConfigSyncer(fakeClient).Sync(context.Background(), newHAConfigTestEvent(t, newHAConfigTestBundle()))
	require.NoError(t, err)

	klusterletConfig := &unstructured.Unstructured{}
	klusterletConfig.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "config.open-cluster-management.io",
		Version: "v1alpha1",
		Kind:    "KlusterletConfig",
	})
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: "ha-standby-local-cluster",
	}, klusterletConfig)
	require.NoError(t, err)

	spec, ok := klusterletConfig.Object["spec"].(map[string]interface{})
	require.True(t, ok)
	_, hasMultipleHubs := spec["multipleHubsConfig"]
	assert.False(t, hasMultipleHubs, "2.13.x should not use multipleHubsConfig")
	_, hasBootstrap := spec["bootstrapKubeConfigs"]
	assert.True(t, hasBootstrap, "2.13.x should use bootstrapKubeConfigs")
}

func TestSync_AllManagedClustersAnnotated(t *testing.T) {
	scheme := newHAConfigTestScheme()
	mch := &mchv1.MultiClusterHub{
		ObjectMeta: metav1.ObjectMeta{Name: "multiclusterhub"},
		Status:     mchv1.MultiClusterHubStatus{CurrentVersion: "2.14.0"},
	}
	cluster1 := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster1"},
	}
	cluster2 := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster2"},
	}
	localCluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "local-cluster"},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mch, cluster1, cluster2, localCluster).
		WithStatusSubresource(mch).
		Build()

	err := newHAConfigSyncer(fakeClient).Sync(context.Background(), newHAConfigTestEvent(t, newHAConfigTestBundle()))
	require.NoError(t, err)

	checkAnnotated := func(name string, shouldHave bool) {
		mc := &clusterv1.ManagedCluster{}
		err := fakeClient.Get(context.Background(), types.NamespacedName{Name: name}, mc)
		require.NoError(t, err)
		annotations := mc.GetAnnotations()
		if shouldHave {
			assert.Equal(t, "ha-standby-local-cluster", annotations[klusterletConfigAnnotation],
				"cluster %s should have klusterlet-config annotation", name)
		} else {
			if annotations != nil {
				_, exists := annotations[klusterletConfigAnnotation]
				assert.False(t, exists,
					"cluster %s should NOT have klusterlet-config annotation", name)
			}
		}
	}

	checkAnnotated("cluster1", true)
	checkAnnotated("cluster2", true)
	checkAnnotated("local-cluster", false)
}

func TestSync_NilBootstrapSecret(t *testing.T) {
	scheme := newHAConfigTestScheme()
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	bundle := &haconfigbundle.HAConfigBundle{
		BootstrapSecret: nil,
	}

	err := newHAConfigSyncer(fakeClient).Sync(context.Background(), newHAConfigTestEvent(t, bundle))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "bootstrap secret is nil")
}

func TestNewHAConfigSyncer(t *testing.T) {
	agentConfig := &configs.AgentConfig{
		LeafHubName:     "hub1",
		TransportConfig: &transport.TransportInternalConfig{},
	}
	syncer := NewHAConfigSyncer(nil, nil, agentConfig)
	assert.Equal(t, "hub1", syncer.leafHubName)
}

func TestAnnotateSkipsAlreadyAnnotated(t *testing.T) {
	scheme := newHAConfigTestScheme()
	cluster1 := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster1",
			Annotations: map[string]string{
				klusterletConfigAnnotation: "ha-standby-local-cluster",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster1).
		Build()

	err := newHAConfigSyncer(fakeClient).annotateAllManagedClusters(context.Background(), "ha-standby-local-cluster")
	require.NoError(t, err)

	mc := &clusterv1.ManagedCluster{}
	err = fakeClient.Get(context.Background(), client.ObjectKeyFromObject(cluster1), mc)
	require.NoError(t, err)
	assert.Equal(t, "ha-standby-local-cluster", mc.GetAnnotations()[klusterletConfigAnnotation])
}
