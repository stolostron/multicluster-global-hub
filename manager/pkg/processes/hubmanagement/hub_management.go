// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubmanagement

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/util/wait"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/hubha"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const (
	// heartbeatInterval = 1 * time.Minute
	ActiveTimeout = 5 * time.Minute // if heartbeat < (now - ActiveTimeout), then status = inactive, vice versa
	ProbeDuration = 2 * time.Minute // the duration to detect run the updating

	// SQL query constants
	whereLeafHubName = "leaf_hub_name = ?"
)

var hubStatusManager HubStatusManager

type HubStatusManager interface {
	inactive(ctx context.Context, hubs []models.LeafHubHeartbeat) error
	reactive(ctx context.Context, hubs []models.LeafHubHeartbeat) error
}

// manage the leaf hub lifecycle based on the heartbeat
type HubManagement struct {
	client        client.Client
	producer      transport.Producer
	probeDuration time.Duration
	activeTimeout time.Duration
	db            *gorm.DB // cached db connection for hub management operations
}

func NewHubManagement(c client.Client, producer transport.Producer, probeDuration,
	activeTimeout time.Duration,
) *HubManagement {
	return &HubManagement{
		client:        c,
		producer:      producer,
		probeDuration: probeDuration,
		activeTimeout: activeTimeout,
	}
}

func AddHubManagement(mgr ctrl.Manager, producer transport.Producer) error {
	if hubStatusManager != nil {
		return nil
	}
	instance := NewHubManagement(mgr.GetClient(), producer, ProbeDuration, ActiveTimeout)
	if err := mgr.Add(instance); err != nil {
		return err
	}
	// add hub addon controller to prune the agent and related resources
	if err := AddManagedClusterAddonController(mgr); err != nil {
		return fmt.Errorf("failed to add the addon controller for hub management: %w", err)
	}
	hubStatusManager = instance
	return nil
}

func (h *HubManagement) Start(ctx context.Context) error {
	h.db = database.GetGorm()
	// when start the hub management, resync all the necessary resources
	err := h.resync(ctx, transport.Broadcast)
	if err != nil {
		return err
	}

	go func() {
		log.Infow("hub management status switch frequency", "interval", h.probeDuration)
		ticker := time.NewTicker(h.probeDuration)
		if err := h.update(ctx); err != nil {
			log.Error(err, "failed to update the hub status on startup")
		}
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				if err := h.update(ctx); err != nil {
					log.Error(err, "failed to update the hub status")
				}
			}
		}
	}()
	return nil
}

func (h *HubManagement) update(ctx context.Context) error {
	thresholdTime := time.Now().Add(-h.activeTimeout)
	var expiredHubs []models.LeafHubHeartbeat
	if err := h.db.Where("last_timestamp < ? AND status = ?", thresholdTime, constants.HubStatusActive).
		Find(&expiredHubs).Error; err != nil {
		return err
	}
	if err := h.inactive(ctx, expiredHubs); err != nil {
		return fmt.Errorf("failed to inactive hubs %v", err)
	}

	var reactiveHubs []models.LeafHubHeartbeat
	if err := h.db.Where("last_timestamp > ? AND status = ?", thresholdTime, constants.HubStatusInactive).
		Find(&reactiveHubs).Error; err != nil {
		return err
	}
	if err := h.reactive(ctx, reactiveHubs); err != nil {
		return fmt.Errorf("failed to reactive hubs %v", err)
	}
	return nil
}

func (h *HubManagement) inactive(ctx context.Context, hubs []models.LeafHubHeartbeat) error {
	for _, hub := range hubs {
		log.Infow("inactive the hub", "name", hub.Name)

		// Notify standby hub BEFORE cleanup, so we can get the managed cluster list
		// cleanup() soft-deletes managed clusters, making them invisible to subsequent queries
		if err := h.sendHubStatusUpdate(ctx, hub.Name, constants.HubStatusInactive); err != nil {
			log.Warnw("failed to send hub status update to standby",
				"hub", hub.Name,
				"status", constants.HubStatusInactive,
				"error", err)
		}

		err := wait.PollUntilContextTimeout(ctx, 2*time.Second, 5*time.Minute, true,
			func(ctx context.Context) (bool, error) {
				if e := h.cleanup(hub.Name); e != nil {
					log.Infow("cleanup the hub resource failed, retrying...", "name", hub.Name, "err", e.Error())
					return false, nil
				}
				return true, nil
			})
		if err != nil {
			return err
		}
	}
	return nil
}

func (h *HubManagement) cleanup(hubName string) error {
	return h.db.Transaction(func(tx *gorm.DB) error {
		// soft delete the cluster
		e := tx.Where(&models.ManagedCluster{
			LeafHubName: hubName,
		}).Delete(&models.ManagedCluster{}).Error
		if e != nil {
			return e
		}

		// soft delete the hub info
		e = tx.Where(&models.LeafHub{
			LeafHubName: hubName,
		}).Delete(&models.LeafHub{}).Error
		if e != nil {
			return e
		}
		// soft delete the policy from the hub
		e = tx.Where(&models.LocalSpecPolicy{
			LeafHubName: hubName,
		}).Delete(&models.LocalSpecPolicy{}).Error
		if e != nil {
			return e
		}
		// delete the compliance
		e = tx.Where(&models.LocalStatusCompliance{
			LeafHubName: hubName,
		}).Delete(&models.LocalStatusCompliance{}).Error
		if e != nil {
			return e
		}

		// inactive the hub status
		return tx.Model(&models.LeafHubHeartbeat{}).Where(whereLeafHubName, hubName).
			Update("status", constants.HubStatusInactive).Error
	})
}

func (h *HubManagement) reactive(ctx context.Context, hubs []models.LeafHubHeartbeat) error {
	// resync hub resources
	for _, hub := range hubs {
		log.Infow("reactive the hub", "name", hub.Name)
		err := wait.PollUntilContextTimeout(ctx, 2*time.Second, 5*time.Minute, true,
			func(ctx context.Context) (bool, error) {
				if e := h.resync(ctx, hub.Name); e != nil {
					log.Info("resync the hub resources failed, retrying...", "name", hub.Name, "err", e.Error())
					return false, nil
				}
				// reactive the batch hub status
				e := h.db.Model(&models.LeafHubHeartbeat{}).Where(whereLeafHubName, hub.Name).
					Update("status", constants.HubStatusActive).Error
				if e != nil {
					log.Info("fail to reactive the hub, retrying...", "name", hub.Name, "err", e.Error())
					return false, nil
				}
				return true, nil
			})
		if err != nil {
			return err
		}

		// Notify standby hub that this hub state update
		if err := h.sendHubStatusUpdate(ctx, hub.Name, constants.HubStatusActive); err != nil {
			log.Warnw("failed to send hub status update to standby",
				"hub", hub.Name,
				"status", constants.HubStatusActive,
				"error", err)
		}
	}
	return nil
}

func (h *HubManagement) resync(ctx context.Context, hubName string) error {
	resyncResources := []string{
		string(enum.HubClusterInfoType),
		string(enum.ManagedClusterType),
		string(enum.LocalPolicySpecType),
		string(enum.LocalComplianceType),
	}
	payloadBytes, err := json.Marshal(resyncResources)
	if err != nil {
		return err
	}

	e := utils.ToCloudEvent(constants.ResyncMsgKey, constants.CloudEventGlobalHubClusterName, hubName, payloadBytes)

	return h.producer.SendEvent(ctx, e)
}

// findStandbyHub finds the standby hub name by querying ManagedCluster resources
// Returns the standby hub name, fallback to local-cluster
func (h *HubManagement) findStandbyHub(ctx context.Context) (string, error) {
	// List ManagedClusters with standby role label
	managedClusterList := &clusterv1.ManagedClusterList{}
	err := h.client.List(ctx, managedClusterList, client.MatchingLabels{
		constants.GHHubRoleLabelKey: constants.GHHubRoleStandby,
	})
	if err != nil {
		return "", fmt.Errorf("failed to list ManagedClusters with standby role: %w", err)
	}

	// If exactly one standby hub found, return it
	if len(managedClusterList.Items) == 1 {
		return managedClusterList.Items[0].Name, nil
	}

	// If no standby hub found, fallback to local-cluster
	if len(managedClusterList.Items) == 0 {
		return constants.LocalClusterName, nil
	}

	// More than one standby hub found - this is a configuration error
	// Choose the first one alphabetically for deterministic behavior and log a warning
	hubNames := make([]string, len(managedClusterList.Items))
	for i, hub := range managedClusterList.Items {
		hubNames[i] = hub.Name
	}

	// Sort to ensure deterministic selection
	chosen := managedClusterList.Items[0].Name
	for _, name := range hubNames {
		if name < chosen {
			chosen = name
		}
	}

	log.Warnw("multiple standby hubs found - using alphabetically first one. Only one standby hub should be configured",
		"count", len(managedClusterList.Items),
		"standbyHubs", hubNames,
		"chosen", chosen)

	return chosen, nil
}

// getManagedClusterNames gets the list of managed cluster names for a given hub
func (h *HubManagement) getManagedClusterNames(hubName string) ([]string, error) {
	var clusterNames []string

	// Use Pluck to get only cluster_name column (generated from payload->metadata->name)
	if err := h.db.Model(&models.ManagedCluster{}).
		Where(whereLeafHubName, hubName).
		Pluck("cluster_name", &clusterNames).Error; err != nil {
		return nil, err
	}

	return clusterNames, nil
}

// sendHubStatusUpdate sends hub status update message to standby hub
func (h *HubManagement) sendHubStatusUpdate(ctx context.Context, hubName, status string) error {
	// Find standby hub
	standbyHub, err := h.findStandbyHub(ctx)
	if err != nil {
		return fmt.Errorf("failed to find standby hub: %w", err)
	}
	if standbyHub == "" {
		log.Debug("no standby hub found, skipping hub status update")
		return nil
	}

	// Get managed clusters for this hub
	managedClusters, err := h.getManagedClusterNames(hubName)
	if err != nil {
		return fmt.Errorf("failed to get managed clusters for hub %s: %w", hubName, err)
	}

	// Create payload
	payload := hubha.HubStatusUpdate{
		HubName:         hubName,
		Status:          status,
		ManagedClusters: managedClusters,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal hub status update payload: %w", err)
	}

	// Send message to standby hub
	e := utils.ToCloudEvent(constants.HubStatusUpdateMsgKey, constants.CloudEventGlobalHubClusterName,
		standbyHub, payloadBytes)

	if err := h.producer.SendEvent(ctx, e); err != nil {
		return fmt.Errorf("failed to send hub status update to standby hub %s: %w", standbyHub, err)
	}

	log.Infow("sent hub status update to standby hub",
		"standbyHub", standbyHub,
		"activeHub", hubName,
		"status", status,
		"managedClusters", len(managedClusters))

	return nil
}
