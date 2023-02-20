// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/apps"
	configCtrl "github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/controlinfo"
	localpolicies "github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/local_policies"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/localplacement"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/managedclusters"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/placement"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/policies"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/syncintervals"
	"github.com/stolostron/multicluster-global-hub/pkg/compressor"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/producer"
)

// AddControllers adds all the controllers to the Manager.
func AddControllers(mgr ctrl.Manager, agentConfig *config.AgentConfig, incarnation uint64) error {
	config := &corev1.ConfigMap{}
	if err := configCtrl.AddConfigController(mgr, config); err != nil {
		return fmt.Errorf("failed to add ConfigMap controller: %w", err)
	}

	syncIntervals := syncintervals.NewSyncIntervals()
	if err := syncintervals.AddSyncIntervalsController(mgr, syncIntervals); err != nil {
		return fmt.Errorf("failed to add SyncIntervals controller: %w", err)
	}

	var p transport.Producer
	var kafkaProducer *producer.KafkaProducer
	if agentConfig.TransportConfig.TransportType == string(transport.Kafka) {
		// support kafka
		messageCompressor, err := compressor.NewCompressor(
			compressor.CompressionType(agentConfig.TransportConfig.MessageCompressionType))
		if err != nil {
			return fmt.Errorf("failed to create kafka producer message-compressor: %w", err)
		}
		kafkaProducer, err = producer.NewKafkaProducer(messageCompressor,
			agentConfig.TransportConfig.KafkaConfig.BootstrapServer,
			agentConfig.TransportConfig.KafkaConfig.CertPath,
			agentConfig.TransportConfig.KafkaConfig.ProducerConfig, ctrl.Log.WithName("kafka-producer"))
		if err != nil {
			return fmt.Errorf("failed to create kafka-producer: %w", err)
		}
		if err := mgr.Add(kafkaProducer); err != nil {
			return fmt.Errorf("failed to initialize kafka producer: %w", err)
		}
		p = kafkaProducer
	} else {
		genericProducer, err := producer.NewGenericProducer(agentConfig.TransportConfig)
		if err != nil {
			return fmt.Errorf("failed to init status transport producer: %w", err)
		}
		p = genericProducer
	}

	hybirdSyncManger, err := policies.AddPoliciesStatusController(mgr, p, agentConfig.LeafHubName,
		incarnation, config, syncIntervals)
	if err != nil {
		return fmt.Errorf("failed to add PoliciesStatusController controller: %w", err)
	}

	// support delta bundle sync mode
	if agentConfig.TransportConfig.TransportType == string(transport.Kafka) {
		hybirdSyncManger.SetHybridModeCallBack(agentConfig.StatusDeltaCountSwitchFactor, kafkaProducer)
	}

	addControllerFunctions := []func(ctrl.Manager, transport.Producer, string, uint64,
		*corev1.ConfigMap, *syncintervals.SyncIntervals) error{
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
		if err := addControllerFunction(mgr, p, agentConfig.LeafHubName, incarnation, config,
			syncIntervals); err != nil {
			return fmt.Errorf("failed to add controller: %w", err)
		}
	}

	return nil
}
