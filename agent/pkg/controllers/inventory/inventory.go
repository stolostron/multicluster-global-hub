// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package inventory

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/controllers/inventory/policy"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var inventoryCtrlStarted = false

// AddToManager adds all the syncers to the Manager.
func AddToManager(ctx context.Context, mgr ctrl.Manager, transportClient transport.TransportClient,
	agentConfig *configs.AgentConfig,
) error {
	if inventoryCtrlStarted {
		return nil
	}

	if err := policy.AddPolicyInventorySyncer(mgr, transportClient.GetRequester()); err != nil {
		return err
	}

	inventoryCtrlStarted = true
	return nil
}
