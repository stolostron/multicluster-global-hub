// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package status

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/filter"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/syncers/apps"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/syncers/events"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/syncers/managedcluster"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/syncers/managedhub"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/syncers/placement"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/syncers/policies"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var statusCtrlStarted = false

// AddToManager adds all the syncers to the Manager.
func AddToManager(ctx context.Context, mgr ctrl.Manager, transportClient transport.TransportClient,
	agentConfig *configs.AgentConfig,
) error {
	if statusCtrlStarted {
		return nil
	}

	if err := addKafkaSyncer(ctx, mgr, transportClient.GetProducer(), agentConfig); err != nil {
		return fmt.Errorf("failed to add the syncer: %w", err)
	}

	statusCtrlStarted = true
	return nil
}

func addKafkaSyncer(ctx context.Context, mgr ctrl.Manager, producer transport.Producer,
	agentConfig *configs.AgentConfig,
) error {
	// managed cluster
	if err := managedcluster.LaunchManagedClusterSyncer(ctx, mgr, agentConfig, producer); err != nil {
		return fmt.Errorf("failed to launch managedcluster syncer: %w", err)
	}

	// event syncer
	err := events.LaunchEventSyncer(ctx, mgr, agentConfig, producer)
	if err != nil {
		return fmt.Errorf("failed to launch event syncer: %w", err)
	}

	// policy syncer(local and global)
	err = policies.LaunchPolicySyncer(ctx, mgr, agentConfig, producer)
	if err != nil {
		return fmt.Errorf("failed to launch policy syncer: %w", err)
	}

	// hub cluster info
	err = managedhub.LaunchHubClusterInfoSyncer(mgr, producer)
	if err != nil {
		return fmt.Errorf("failed to launch hub cluster info syncer: %w", err)
	}

	err = managedhub.LaunchHubClusterHeartbeatSyncer(mgr, producer)
	if err != nil {
		return fmt.Errorf("failed to launch hub cluster heartbeat syncer: %w", err)
	}

	// placement
	if err := placement.LaunchPlacementSyncer(ctx, mgr, agentConfig, producer); err != nil {
		return fmt.Errorf("failed to launch placement syncer: %w", err)
	}
	if err := placement.LaunchPlacementDecisionSyncer(ctx, mgr, agentConfig, producer); err != nil {
		return fmt.Errorf("failed to launch placementDecision syncer: %w", err)
	}
	if err := placement.LaunchPlacementRuleSyncer(ctx, mgr, agentConfig, producer); err != nil {
		return fmt.Errorf("failed to launch placementRule syncer: %w", err)
	}

	// app
	if err := apps.LaunchSubscriptionReportSyncer(ctx, mgr, agentConfig, producer); err != nil {
		return fmt.Errorf("failed to launch subscription report syncer: %w", err)
	}

	// lunch a time filter, it must be called after filter.RegisterTimeFilter(key)
	if err := filter.LaunchTimeFilter(ctx, mgr.GetClient(), agentConfig.PodNamespace,
		agentConfig.TransportConfig.KafkaCredential.StatusTopic); err != nil {
		return fmt.Errorf("failed to launch time filter: %w", err)
	}
	return nil
}
