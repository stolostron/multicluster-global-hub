// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubmanagement

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const (
	HubActive   = "active"
	HubInactive = "inactive"

	// heartbeatInterval = 1 * time.Minute
	ActiveTimeout = 5 * time.Minute // if heartbeat < (now - ActiveTimeout), then status = inactive, vice versa
	ProbeDuration = 2 * time.Minute // the duration to detect run the updating
)

var hubStatusManager HubStatusManager

type HubStatusManager interface {
	inactive(ctx context.Context, hubs []models.LeafHubHeartbeat) error
	reactive(ctx context.Context, hubs []models.LeafHubHeartbeat) error
}

// manage the leaf hub lifecycle based on the heartbeat
type HubManagement struct {
	log           *zap.SugaredLogger
	producer      transport.Producer
	probeDuration time.Duration
	activeTimeout time.Duration
}

func NewHubManagement(producer transport.Producer, probeDuration, activeTimeout time.Duration) *HubManagement {
	return &HubManagement{
		log:           logger.DefaultZapLogger(),
		producer:      producer,
		probeDuration: probeDuration,
		activeTimeout: activeTimeout,
	}
}

func AddHubManagement(mgr ctrl.Manager, producer transport.Producer) error {
	if hubStatusManager != nil {
		return nil
	}
	instance := NewHubManagement(producer, ProbeDuration, ActiveTimeout)
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
	// when start the hub management, resync all the necessary resources
	err := h.resync(ctx, transport.Broadcast)
	if err != nil {
		return err
	}

	go func() {
		h.log.Infow("hub management status switch frequency", "interval", h.probeDuration)
		ticker := time.NewTicker(h.probeDuration)
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				if err := h.update(ctx); err != nil {
					h.log.Error(err, "failed to update the hub status")
				}
			}
		}
	}()
	return nil
}

func (h *HubManagement) update(ctx context.Context) error {
	thresholdTime := time.Now().Add(-h.activeTimeout)
	db := database.GetGorm()
	var expiredHubs []models.LeafHubHeartbeat
	if err := db.Where("last_timestamp < ? AND status = ?", thresholdTime, HubActive).
		Find(&expiredHubs).Error; err != nil {
		return err
	}
	if err := h.inactive(ctx, expiredHubs); err != nil {
		return fmt.Errorf("failed to inactive hubs %v", err)
	}

	var reactiveHubs []models.LeafHubHeartbeat
	if err := db.Where("last_timestamp > ? AND status = ?", thresholdTime, HubInactive).
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
		err := wait.PollUntilContextTimeout(ctx, 2*time.Second, 5*time.Minute, true,
			func(ctx context.Context) (bool, error) {
				if e := h.cleanup(hub.Name); e != nil {
					h.log.Infow("cleanup the hub resource failed, retrying...", "name", hub.Name, "err", e.Error())
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
	db := database.GetGorm()
	return db.Transaction(func(tx *gorm.DB) error {
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
		return tx.Model(&models.LeafHubHeartbeat{}).Where("leaf_hub_name = ?", hubName).Update("status", HubInactive).Error
	})
}

func (h *HubManagement) reactive(ctx context.Context, hubs []models.LeafHubHeartbeat) error {
	// resync hub resources
	db := database.GetGorm()
	for _, hub := range hubs {
		err := wait.PollUntilContextTimeout(ctx, 2*time.Second, 5*time.Minute, true,
			func(ctx context.Context) (bool, error) {
				if e := h.resync(ctx, hub.Name); e != nil {
					h.log.Info("resync the hub resources failed, retrying...", "name", hub.Name, "err", e.Error())
					return false, nil
				}
				// reactive the batch hub status
				e := db.Model(&models.LeafHubHeartbeat{}).Where("leaf_hub_name = ?", hub.Name).Update("status", HubActive).Error
				if e != nil {
					h.log.Info("fail to reactive the hub, retrying...", "name", hub.Name, "err", e.Error())
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
