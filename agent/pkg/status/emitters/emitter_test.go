package emitters

import (
	"context"
	"encoding/json"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	genericbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

// Simple mock producer
type MockProducer struct {
	events []cloudevents.Event
}

func (m *MockProducer) SendEvent(ctx context.Context, evt cloudevents.Event) error {
	m.events = append(m.events, evt)
	return nil
}

func (m *MockProducer) Reconnect(config *transport.TransportInternalConfig, clientID string) error {
	return nil
}

func TestObjectEmitter_BundleOperations(t *testing.T) {
	producer := &MockProducer{}
	eventType := enum.EventType("test-event")
	emitter := NewObjectEmitter(eventType, producer)

	// Test objects
	obj1 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "obj1",
			UID:  "uid1",
		},
		Data: map[string]string{"key1": "value1"},
	}
	obj2 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "obj2",
			UID:  "uid2",
		},
		Data: map[string]string{"key2": "value2"},
	}

	// Test Update operation - should add to bundle.Update
	err := emitter.Update(obj1)
	require.NoError(t, err)
	require.Len(t, emitter.bundle.Update, 1)

	err = emitter.Update(obj2)
	require.NoError(t, err)
	require.Len(t, emitter.bundle.Update, 2)

	// Test Delete operation - should add to bundle.Delete
	err = emitter.Delete(obj1)
	require.NoError(t, err)
	require.Len(t, emitter.bundle.Delete, 1)

	// Verify delete metadata
	deleteMeta := emitter.bundle.Delete[0]
	require.Equal(t, string(obj1.GetUID()), deleteMeta.ID)
	require.Equal(t, obj1.GetName(), deleteMeta.Name)
}

func TestObjectEmitter_ResyncOperations(t *testing.T) {
	configs.SetAgentConfig(&configs.AgentConfig{LeafHubName: "test-leaf-hub"})
	producer := &MockProducer{}
	eventType := enum.EventType("test-event")
	emitter := NewObjectEmitter(eventType, producer)

	// Test objects
	objects := []client.Object{
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "obj1",
				UID:       "uid1",
				Namespace: "ns1",
			},
			Data: map[string]string{"key1": "value1"},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "obj2",
				UID:       "uid2",
				Namespace: "ns2",
			},
			Data: map[string]string{"key2": "value2"},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "obj3",
				UID:       "uid3",
				Namespace: "ns3",
			},
			Data: map[string]string{"key3": "value3"},
		},
	}

	// Test Resync operation - should add to bundle.Resync and bundle.ResyncMetadata
	err := emitter.Resync(objects)
	require.NoError(t, err)

	// verify the bundle is in the producer
	require.Len(t, producer.events, 2)

	// the event1 is the resync event
	require.Equal(t, producer.events[0].Type(), string(eventType))
	require.Equal(t, producer.events[0].Source(), configs.GetLeafHubName())

	receivedBundle := &genericbundle.GenericBundle[corev1.ConfigMap]{}
	err = json.Unmarshal(producer.events[0].Data(), receivedBundle)
	require.NoError(t, err)

	require.Len(t, receivedBundle.Resync, 3)
	require.Len(t, receivedBundle.ResyncMetadata, 0)

	receivedBundle = &genericbundle.GenericBundle[corev1.ConfigMap]{}
	err = json.Unmarshal(producer.events[1].Data(), receivedBundle)
	require.NoError(t, err)

	require.Len(t, receivedBundle.ResyncMetadata, 3)
}

func TestObjectEmitter_MixedBundleOperations(t *testing.T) {
	configs.SetAgentConfig(&configs.AgentConfig{LeafHubName: "test-leaf-hub"})

	producer := &MockProducer{}
	eventType := enum.EventType("test-event")
	emitter := NewObjectEmitter(eventType, producer)

	// Test mixed operations
	obj1 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "obj1",
			UID:  "uid1",
		},
		Data: map[string]string{"key1": "value1"},
	}
	obj2 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "obj2",
			UID:  "uid2",
		},
		Data: map[string]string{"key2": "value2"},
	}

	// Add different operations to the same bundle
	err := emitter.Update(obj1)
	require.NoError(t, err)
	err = emitter.Update(obj2)
	require.NoError(t, err)
	err = emitter.Delete(obj1)
	require.NoError(t, err)

	// Verify bundle state
	require.Len(t, emitter.bundle.Update, 1)
	require.Len(t, emitter.bundle.Delete, 1)

	// Test that bundle is not empty
	require.False(t, emitter.bundle.IsEmpty())

	// Test Send operation - should send and clean bundle
	err = emitter.Send()
	require.NoError(t, err)

	// Bundle should be cleaned after send
	require.True(t, emitter.bundle.IsEmpty())

	// Should have sent an event
	require.Len(t, producer.events, 1)
}

func TestObjectEmitter_WithTweakFunction(t *testing.T) {
	producer := &MockProducer{}
	eventType := enum.EventType("test-event")

	// Create emitter with tweak function
	tweakFunc := func(obj client.Object) {
		obj.SetName("tweaked-" + obj.GetName())
	}
	emitter := NewObjectEmitter(eventType, producer, WithTweakFunc(tweakFunc))

	obj := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "original",
			UID:  "uid1",
		},
		Data: map[string]string{"key": "value"},
	}

	// Test that update applies tweak function
	err := emitter.Update(obj)
	require.NoError(t, err)

	// Original object should be unchanged
	require.Equal(t, "original", obj.GetName())

	// Bundle should contain tweaked object
	require.Len(t, emitter.bundle.Update, 1)

	// assert the object in the bundle is tweaked
	require.Equal(t, "tweaked-original", emitter.bundle.Update[0].GetName())
}
