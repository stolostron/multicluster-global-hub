// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package inventory

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/controllers/inventory/managedclusterinfo"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/controllers/inventory/policy"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var inventoryCtrlStarted = false

// AddToManager adds all the syncers to the Manager.
func AddToManager(ctx context.Context, mgr ctrl.Manager, transportClient transport.TransportClient,
	agentConfig *configs.AgentConfig,
) error {
	if inventoryCtrlStarted {
		return nil
	}

	mch, err := utils.ListMCH(ctx, mgr.GetClient())
	if err != nil {
		return err
	}
	configs.SetMCHVersion(mch.Status.CurrentVersion)

	if err := managedclusterinfo.AddManagedClusterInfoInventorySyncer(mgr, transportClient.GetRequester()); err != nil {
		return err
	}
	if err := policy.AddPolicyInventorySyncer(mgr, transportClient.GetRequester()); err != nil {
		return err
	}
	inventoryCtrlStarted = true
	return nil
}
