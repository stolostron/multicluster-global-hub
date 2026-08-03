package configs

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetSyncStateConfigMap_createsWhenMissing(t *testing.T) {
	ctx := context.Background()
	SetAgentConfig(&AgentConfig{PodNamespace: "agent-ns"})
	t.Cleanup(func() { SetAgentConfig(nil) })

	c := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	cm, err := GetSyncStateConfigMap(ctx, c)
	require.NoError(t, err)
	assert.Equal(t, AGENT_SYNC_STATE_CONFIG_MAP_NAME, cm.Name)
	assert.Equal(t, "agent-ns", cm.Namespace)
}

func TestGetSyncStateConfigMap_returnsExisting(t *testing.T) {
	ctx := context.Background()
	namespace := "agent-ns"
	SetAgentConfig(&AgentConfig{PodNamespace: namespace})
	t.Cleanup(func() { SetAgentConfig(nil) })

	existing := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      AGENT_SYNC_STATE_CONFIG_MAP_NAME,
			Namespace: namespace,
		},
		Data: map[string]string{"existing": "value"},
	}
	c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(existing).Build()

	cm, err := GetSyncStateConfigMap(ctx, c)
	require.NoError(t, err)
	assert.Equal(t, "value", cm.Data["existing"])
}

func TestSetSyncTimeState_andGetSyncTimeState(t *testing.T) {
	ctx := context.Background()
	namespace := "agent-ns"
	SetAgentConfig(&AgentConfig{PodNamespace: namespace})
	t.Cleanup(func() { SetAgentConfig(nil) })

	c := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	key := "topic--hub1"
	evtTime := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)

	require.NoError(t, SetSyncTimeState(ctx, c, key, evtTime))

	found, got, err := GetSyncTimeState(ctx, c, key)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, evtTime.Format(AGENT_SYNC_STATE_TIME_FORMAT_VALUE), got.Format(AGENT_SYNC_STATE_TIME_FORMAT_VALUE))
}

func TestGetSyncTimeState_missingKey(t *testing.T) {
	ctx := context.Background()
	SetAgentConfig(&AgentConfig{PodNamespace: "agent-ns"})
	t.Cleanup(func() { SetAgentConfig(nil) })

	c := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	require.NoError(t, SetSyncTimeState(ctx, c, "other-key", time.Now()))

	found, _, err := GetSyncTimeState(ctx, c, "missing-key")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestGetSyncTimeState_invalidTimestamp(t *testing.T) {
	ctx := context.Background()
	namespace := "agent-ns"
	SetAgentConfig(&AgentConfig{PodNamespace: namespace})
	t.Cleanup(func() { SetAgentConfig(nil) })

	existing := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      AGENT_SYNC_STATE_CONFIG_MAP_NAME,
			Namespace: namespace,
		},
		Data: map[string]string{"bad-key": "not-a-timestamp"},
	}
	c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(existing).Build()

	found, _, err := GetSyncTimeState(ctx, c, "bad-key")
	require.Error(t, err)
	assert.True(t, found)
}
