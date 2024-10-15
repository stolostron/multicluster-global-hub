package controllers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
)

func TestAddCRDController(t *testing.T) {
	cfg := &rest.Config{
		Host:            "https://mock-cluster",
		APIPath:         "/api",
		BearerToken:     "mock-token",
		TLSClientConfig: rest.TLSClientConfig{Insecure: true},
	}
	mgr, err := manager.New(cfg, manager.Options{})
	require.NoError(t, err)
	crdCtrl := &initController{
		mgr:         mgr,
		agentConfig: &configs.AgentConfig{LeafHubName: "hub1"},
	}
	_, err = crdCtrl.Reconcile(context.TODO(), reconcile.Request{NamespacedName: types.NamespacedName{
		Namespace: "hello",
		Name:      "world",
	}})
	require.NoError(t, err)
}
