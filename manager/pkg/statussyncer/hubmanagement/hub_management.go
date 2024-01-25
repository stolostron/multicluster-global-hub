// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubmanagement

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

const (
	HubActive   = "active"
	HubInactive = "inactive"

	// heartbeatInterval = 1 * time.Minute
	ActiveTimeout = 5 * time.Minute // if heartbeat < (now - ActiveTimeout), then status = inactive, vice versa
	ProbeDuration = 2 * time.Minute // the duration to detect run the updating
)

// manage the leaf hub lifecycle based on the heartbeat
type hubManagement struct {
	log           logr.Logger
	producer      transport.Producer
	probeDuration time.Duration
	activeTimeout time.Duration
}

func AddHubManagement(mgr ctrl.Manager) error {
	return mgr.Add(&hubManagement{
		log:           ctrl.Log.WithName("hub-management"),
		probeDuration: ProbeDuration, activeTimeout: ActiveTimeout,
	})
}

func (h *hubManagement) Start(ctx context.Context) error {
	go func() {
		h.log.Info("hub management status switch frequency", "interval", h.probeDuration)
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

func (h *hubManagement) update(ctx context.Context) error {
	thresholdTime := time.Now().Add(-h.activeTimeout)
	db := database.GetGorm()

	var expiredHubs []models.LeafHubHeartbeat
	if err := db.Where("last_timestamp < ? AND status = ?", thresholdTime, HubActive).
		Find(&expiredHubs).Error; err != nil {
		return err
	}
	if err := h.inactive(ctx, expiredHubs, thresholdTime); err != nil {
		return fmt.Errorf("failed to inactive hubs %v", err)
	}

	var reactiveHubs []models.LeafHubHeartbeat
	if err := db.Where("last_timestamp > ? AND status = ?", thresholdTime, HubInactive).
		Find(&reactiveHubs).Error; err != nil {
		return err
	}
	if err := h.reactive(ctx, reactiveHubs, thresholdTime); err != nil {
		return fmt.Errorf("failed to reactive hubs %v", err)
	}
	return nil
}

func (h *hubManagement) inactive(ctx context.Context, hubs []models.LeafHubHeartbeat, thresholdTime time.Time) error {
	for _, hub := range hubs {
		err := wait.PollUntilContextTimeout(ctx, 2*time.Second, 5*time.Minute, true,
			func(ctx context.Context) (bool, error) {
				if e := h.cleanup(hub.Name); e != nil {
					h.log.Info("cleanup the hub resource failed, retrying...", "name", hub.Name, "err", e.Error())
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

func (h *hubManagement) cleanup(hubName string) error {
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

func (h *hubManagement) reactive(ctx context.Context, hubs []models.LeafHubHeartbeat, thresholdTime time.Time) error {
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

func (h *hubManagement) resync(ctx context.Context, hubName string) error {
	resyncResources := []string{
		constants.ManagedClustersMsgKey,
		constants.HubClusterInfoMsgKey,
		constants.LocalPolicySpecMsgKey,
		constants.LocalComplianceMsgKey,
	}
	payloadBytes, err := json.Marshal(resyncResources)
	if err != nil {
		return err
	}

	return h.producer.Send(ctx, &transport.Message{
		Key:         constants.ResyncMsgKey,
		Destination: hubName,
		MsgType:     constants.SpecBundle,
		Payload:     payloadBytes,
	})
}
