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
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
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

// Sync applies a hub status update by setting hubAcceptsClient on the listed
// managed clusters. The hub's own local cluster is left unchanged so one hub
// cannot disable another hub's self-managed cluster.
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

	// Every hub has its own ManagedCluster named local-cluster. A status update from
	// another hub must not change this hub's self-managed cluster.
	managedClusters := excludeLocalClusterNames(update.ManagedClusters)

	// Update each ManagedCluster's hubAcceptsClient field
	var updateErrors []error
	successCount := 0
	for _, clusterName := range managedClusters {
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
			len(updateErrors), len(managedClusters), updateErrors)
	}

	return nil
}

// updateManagedClusterHubAcceptsClient updates the hubAcceptsClient field in ManagedCluster spec
func (s *HubStatusSyncer) updateManagedClusterHubAcceptsClient(ctx context.Context, clusterName string,
	hubAcceptsClient bool,
) error {
	// First, try to get the cluster without retry to check if it exists
	managedCluster := &clusterv1.ManagedCluster{}
	if err := s.client.Get(ctx, client.ObjectKey{Name: clusterName}, managedCluster); err != nil {
		if errors.IsNotFound(err) {
			// Return NotFound error - this will cause Sync() to return error
			// triggering message-level retry with proper backoff, giving cache time to sync
			log.Debugw("ManagedCluster not found, will retry message later",
				"cluster", clusterName)
			return fmt.Errorf("ManagedCluster not found (may be cache sync delay): %s", clusterName)
		}
		return fmt.Errorf("failed to get ManagedCluster: %w", err)
	}

	// Use retry for update conflicts only. Identity is checked on every attempt
	// because a conflicting write can add the local-cluster label before retry.
	updated := false
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		updated = false
		if err := s.client.Get(ctx, client.ObjectKey{Name: clusterName}, managedCluster); err != nil {
			return fmt.Errorf("failed to get ManagedCluster: %w", err)
		}

		// A custom-named local cluster (local-cluster=true) is this hub, not a failover target.
		if utils.IsLocalManagedCluster(managedCluster) {
			log.Infow("skipping hubAcceptsClient update for the hub local cluster",
				"cluster", clusterName)
			return nil
		}

		if managedCluster.Spec.HubAcceptsClient == hubAcceptsClient {
			log.Debugw("ManagedCluster hubAcceptsClient already set to desired value",
				"cluster", clusterName,
				"hubAcceptsClient", hubAcceptsClient)
			return nil
		}

		managedCluster.Spec.HubAcceptsClient = hubAcceptsClient
		if err := s.client.Update(ctx, managedCluster); err != nil {
			return fmt.Errorf("failed to update ManagedCluster: %w", err)
		}
		updated = true
		return nil
	})
	if err != nil {
		return err
	}
	if !updated {
		return nil
	}

	log.Infow("updated ManagedCluster hubAcceptsClient",
		"cluster", clusterName,
		"hubAcceptsClient", hubAcceptsClient)

	return nil
}

// excludeLocalClusterNames drops the reserved local-cluster name. That name is not
// unique across hubs, so applying it on the standby updates the wrong cluster.
func excludeLocalClusterNames(names []string) []string {
	if len(names) == 0 {
		return names
	}
	filtered := make([]string, 0, len(names))
	for _, name := range names {
		if name == constants.LocalClusterName {
			log.Infow("skipping hubAcceptsClient update for local cluster", "cluster", name)
			continue
		}
		filtered = append(filtered, name)
	}
	return filtered
}
