// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubstatus

import (
	"context"
	"fmt"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/hubha"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

var log = logger.DefaultZapLogger()

// HubStatusSyncer handles hub status update messages from global hub manager
// When active hub status changes, it updates hubAcceptsClient for affected ManagedClusters
type HubStatusSyncer struct {
	client client.Client
}

func NewHubStatusSyncer(mgr ctrl.Manager) *HubStatusSyncer {
	return &HubStatusSyncer{
		client: mgr.GetClient(),
	}
}

func (s *HubStatusSyncer) Sync(ctx context.Context, evt *cloudevents.Event) error {
	eventType := evt.Type()
	if eventType != constants.HubStatusUpdateMsgKey {
		return nil // Not a hub status update message
	}

	log.Infow("received hub status update message", "source", evt.Source())

	// Parse payload
	var update hubha.HubStatusUpdate
	if err := evt.DataAs(&update); err != nil {
		return fmt.Errorf("failed to parse hub status update payload: %w", err)
	}

	log.Infow("processing hub status update",
		"hub", update.HubName,
		"status", update.Status,
		"managedClusters", len(update.ManagedClusters))

	// Determine hubAcceptsClient value based on status
	// - inactive: standby hub should accept clients (failover)
	// - active: standby hub should NOT accept clients (normal state)
	hubAcceptsClient := update.Status == constants.HubStatusInactive

	// Update each ManagedCluster's hubAcceptsClient field
	var updateErrors []error
	successCount := 0
	for _, clusterName := range update.ManagedClusters {
		if err := s.updateManagedClusterHubAcceptsClient(ctx, clusterName, hubAcceptsClient); err != nil {
			log.Warnw("failed to update ManagedCluster hubAcceptsClient",
				"cluster", clusterName,
				"hubAcceptsClient", hubAcceptsClient,
				"error", err)
			updateErrors = append(updateErrors, fmt.Errorf("cluster %s: %w", clusterName, err))
		} else {
			successCount++
		}
	}

	log.Infow("completed hub status update processing",
		"hub", update.HubName,
		"status", update.Status,
		"totalClusters", len(update.ManagedClusters),
		"successCount", successCount)

	// Return error if any updates failed - this will trigger retry
	if len(updateErrors) > 0 {
		return fmt.Errorf("failed to update %d/%d ManagedClusters: %v",
			len(updateErrors), len(update.ManagedClusters), updateErrors)
	}

	return nil
}

// updateManagedClusterHubAcceptsClient updates the hubAcceptsClient field in ManagedCluster spec
func (s *HubStatusSyncer) updateManagedClusterHubAcceptsClient(ctx context.Context, clusterName string, hubAcceptsClient bool) error {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Get the ManagedCluster
		managedCluster := &clusterv1.ManagedCluster{}
		if err := s.client.Get(ctx, client.ObjectKey{Name: clusterName}, managedCluster); err != nil {
			if errors.IsNotFound(err) {
				log.Debugw("ManagedCluster not found, skipping update", "cluster", clusterName)
				return nil
			}
			return fmt.Errorf("failed to get ManagedCluster: %w", err)
		}

		// Check if update is needed
		if managedCluster.Spec.HubAcceptsClient == hubAcceptsClient {
			log.Debugw("ManagedCluster hubAcceptsClient already set to desired value",
				"cluster", clusterName,
				"hubAcceptsClient", hubAcceptsClient)
			return nil
		}

		// Set hubAcceptsClient
		managedCluster.Spec.HubAcceptsClient = hubAcceptsClient

		// Update in cluster
		if err := s.client.Update(ctx, managedCluster); err != nil {
			return fmt.Errorf("failed to update ManagedCluster: %w", err)
		}

		log.Infow("updated ManagedCluster hubAcceptsClient",
			"cluster", clusterName,
			"hubAcceptsClient", hubAcceptsClient)

		return nil
	})

	return err
}
