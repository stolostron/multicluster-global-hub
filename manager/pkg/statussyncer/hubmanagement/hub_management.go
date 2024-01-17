// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubmanagement

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"gorm.io/gorm/clause"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

const (
	HubActive   = "active"
	HubInactive = "inactive"
)

// manage the leaf hub lifecycle by the heartbeat
type hubManagement struct {
	log           logr.Logger
	probeInterval time.Duration
}

func AddHubManagement(mgr ctrl.Manager, managerConfig *config.ManagerConfig) error {
	interval, err := time.ParseDuration(managerConfig.AgentSessionTimeout)
	if err != nil {
		return err
	}
	return mgr.Add(&hubManagement{log: ctrl.Log.WithName("hub-management"), probeInterval: interval})
}

func (h *hubManagement) Start(ctx context.Context) error {
	go func() {
		h.log.Info("hub management status switch frequency", "interval", h.probeInterval)
		ticker := time.NewTicker(h.probeInterval)
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
	lastTime := time.Now().Add(-h.probeInterval)
	db := database.GetGorm()
	var expiredHubs []models.LeafHubHeartbeat
	err := db.Where("last_timestamp < ? AND status = ?", lastTime, HubActive).Find(&expiredHubs).Error
	if err != nil || len(expiredHubs) == 0 {
		return err
	}
	if err := h.inactive(ctx, expiredHubs); err != nil {
		return fmt.Errorf("inactive the hubs with error: %v", err)
	}
	return nil
}

func (h *hubManagement) inactive(ctx context.Context, hubs []models.LeafHubHeartbeat) error {
	// TODO: cleanup the database resources

	// mark the status as inactive
	db := database.GetGorm()
	for i := range hubs {
		hubs[i].Status = HubInactive
	}
	err := db.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(hubs, 100).Error
	if err != nil {
		return fmt.Errorf("failed to update the hubs to inactive: %v", err)
	}
	return nil
}
