package controllers

import (
	"context"
	"testing"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/config"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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
	crdCtrl := &crdController{
		mgr:         mgr,
		agentConfig: &config.AgentConfig{LeafHubName: "hub1"},
	}
	_, err = crdCtrl.Reconcile(context.TODO(), reconcile.Request{NamespacedName: types.NamespacedName{
		Namespace: "hello",
		Name:      "world",
	}})
	require.Contains(t, err.Error(), "failed to add status syncer")
}
