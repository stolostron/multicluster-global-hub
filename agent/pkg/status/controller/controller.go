// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/apps"
	globalhubagentconfig "github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/controlinfo"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/hubcluster"
	localpolicies "github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/local_policies"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/localplacement"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/managedclusters"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/placement"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/policies"
	"github.com/stolostron/multicluster-global-hub/pkg/compressor"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	transportproducer "github.com/stolostron/multicluster-global-hub/pkg/transport/producer"
)

// AddControllers adds all the controllers to the Manager.
func AddControllers(ctx context.Context, mgr ctrl.Manager, agentConfig *config.AgentConfig, incarnation uint64) error {
	config := &corev1.ConfigMap{}
	syncIntervals := globalhubagentconfig.NewSyncIntervals()
	if err := globalhubagentconfig.AddConfigController(mgr, config, syncIntervals); err != nil {
		return fmt.Errorf("failed to add ConfigMap controller: %w", err)
	}

	producer, isAsync, err := getProducer(mgr, agentConfig)
	if err != nil {
		return fmt.Errorf("failed to get producer: %w", err)
	}

	hybirdSyncManger, err := policies.AddPoliciesStatusController(mgr, producer, agentConfig.LeafHubName,
		incarnation, config, syncIntervals)
	if err != nil {
		return fmt.Errorf("failed to add PoliciesStatusController controller: %w", err)
	}

	err = hubcluster.AddHubClusterController(mgr, producer, agentConfig.LeafHubName)
	if err != nil {
		return fmt.Errorf("failed to add HubClusterController controller: %w", err)
	}

	// support delta bundle sync mode
	if isAsync {
		kafkaProducer, ok := producer.(*transportproducer.KafkaProducer)
		if !ok {
			return fmt.Errorf("failed to set the kafka message producer callback() which is to switch the sync mode")
		}
		hybirdSyncManger.SetHybridModeCallBack(agentConfig.StatusDeltaCountSwitchFactor, kafkaProducer)
	}

	addControllerFunctions := []func(ctrl.Manager, transport.Producer, string, uint64,
		*corev1.ConfigMap, *globalhubagentconfig.SyncIntervals) error{
		managedclusters.AddClustersStatusController,
		placement.AddPlacementRulesController,
		placement.AddPlacementsController,
		placement.AddPlacementDecisionsController,
		// apps.AddSubscriptionStatusesController,
		apps.AddSubscriptionReportsController,
		localpolicies.AddLocalPoliciesController,
		localplacement.AddLocalPlacementRulesController,
		controlinfo.AddControlInfoController,
	}

	for _, addControllerFunction := range addControllerFunctions {
		if err := addControllerFunction(mgr, producer, agentConfig.LeafHubName, incarnation, config,
			syncIntervals); err != nil {
			return fmt.Errorf("failed to add controller: %w", err)
		}
	}
	// controller for local cluster policies history event bundle
	localpolicies.AddLocalClusterPoliciesController(ctx, mgr, producer, agentConfig.LeafHubName, incarnation,
		config, syncIntervals)
	return nil
}

func getProducer(mgr ctrl.Manager, agentConfig *config.AgentConfig) (transport.Producer, bool, error) {
	if agentConfig.TransportConfig.TransportFormat == string(transport.KafkaMessageFormat) {
		// support kafka
		isAsync := true
		messageCompressor, err := compressor.NewCompressor(
			compressor.CompressionType(agentConfig.TransportConfig.MessageCompressionType))
		if err != nil {
			return nil, isAsync, fmt.Errorf("failed to create kafka message-compressor: %w", err)
		}
		kafkaProducer, err := transportproducer.NewKafkaProducer(messageCompressor,
			agentConfig.TransportConfig.KafkaConfig,
			ctrl.Log.WithName("kafka-message-producer"))
		if err != nil {
			return nil, isAsync, fmt.Errorf("failed to create kafka-message-producer: %w", err)
		}
		if err := mgr.Add(kafkaProducer); err != nil {
			return nil, isAsync, fmt.Errorf("failed to initialize kafka message producer: %w", err)
		}
		return kafkaProducer, isAsync, nil
	} else {
		isAsync := false
		genericProducer, err := transportproducer.NewGenericProducer(agentConfig.TransportConfig)
		if err != nil {
			return nil, isAsync, fmt.Errorf("failed to init status transport producer: %w", err)
		}
		return genericProducer, isAsync, nil
	}
}
