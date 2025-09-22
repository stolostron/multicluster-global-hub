package handlers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

func TestPostSendFunc(t *testing.T) {
	configs.SetAgentConfig(&configs.AgentConfig{LeafHubName: "leaf-hub-name"})
	localStatusEventHandler := NewPolicyStatusEventHandler(context.TODO(), enum.LocalReplicatedPolicyEventType,
		func(obj client.Object) bool {
			return true
		},
		nil,
	)
	localStatusEventEmitter := NewPolicyStatusEventEmitter(enum.LocalReplicatedPolicyEventType)

	obj := localStatusEventHandler.Get()
	assertBundle(t, obj, 0)

	// mock the handler update
	localStatusEventHandler.Append(&event.ReplicatedPolicyEvent{
		BaseEvent: event.BaseEvent{
			EventName: "name",
			Reason:    "PolicyStatusSync",
			Count:     1,
			Source: corev1.EventSource{
				Component: "policy-status-history-sync",
			},
		},
		ClusterName: "hello",
	})

	// mock cloudevents to send
	obj = localStatusEventHandler.Get()
	assertBundle(t, obj, 1)

	_, err := localStatusEventEmitter.ToCloudEvent(obj)
	assert.Nil(t, err)

	// post send -> the payload is cleanup
	localStatusEventEmitter.PostSend(localStatusEventHandler.Get())
	obj = localStatusEventHandler.Get()
	assertBundle(t, obj, 0)
}

func assertBundle(t *testing.T, obj interface{}, size int) {
	bundle, ok := obj.(*event.ReplicatedPolicyEventBundle)
	require.Equal(t, ok, true)
	require.Equal(t, size, len(*bundle))
}
