// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/apps"
	agentstatusconfig "github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/event"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/filter"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/hubcluster"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/managedclusters"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/placement"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/policies"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/security"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var statusCtrlStarted = false

// AddControllers adds all the controllers to the Manager.
func AddControllers(ctx context.Context, mgr ctrl.Manager, producer transport.Producer,
	agentConfig *config.AgentConfig,
) error {
	if statusCtrlStarted {
		return nil
	}

	if err := agentstatusconfig.AddConfigController(mgr, agentConfig); err != nil {
		return fmt.Errorf("failed to add ConfigMap controller: %w", err)
	}

	// managed cluster
	if err := managedclusters.LaunchManagedClusterSyncer(ctx, mgr, agentConfig, producer); err != nil {
		return fmt.Errorf("failed to launch managedcluster syncer: %w", err)
	}

	// event syncer
	if err := event.LaunchEventSyncer(ctx, mgr, agentConfig, producer); err != nil {
		return fmt.Errorf("failed to launch event syncer: %w", err)
	}

	// policy syncer(local and global)
	if err := policies.LaunchPolicySyncer(ctx, mgr, agentConfig, producer); err != nil {
		return fmt.Errorf("failed to launch policy syncer: %w", err)
	}

	// hub cluster info
	if err := hubcluster.LaunchHubClusterInfoSyncer(mgr, producer); err != nil {
		return fmt.Errorf("failed to launch hub cluster info syncer: %w", err)
	}

	// hub cluster heartbeat
	if err := hubcluster.LaunchHubClusterHeartbeatSyncer(mgr, producer); err != nil {
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
	if err := filter.LaunchTimeFilter(ctx, mgr.GetClient(), agentConfig.PodNameSpace,
		agentConfig.TransportConfig.KafkaCredential.StatusTopic); err != nil {
		return fmt.Errorf("failed to launch time filter: %w", err)
	}
	if agentConfig.EnableStackroxIntegration {
		if err := security.LaunchStackroxCentralCRDController(ctx, mgr); err != nil {
			return fmt.Errorf("failed to launch Stackrox Central CRD controller: %w", err)
		}
	}

	statusCtrlStarted = true

	return nil
}
