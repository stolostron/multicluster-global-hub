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

	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

const (
	HubActive      = "active"
	HubInactive    = "inactive"
	SessionTimeout = 5 * time.Minute // 5 * heartbeat: if heartbeat < now - SessionTimeout, then status = inactive
	ProbeDuration  = 1 * time.Minute // 1 * heartbeat: the frequency to detect run the updating
)

// manage the leaf hub lifecycle based on the heartbeat
type hubManagement struct {
	log            logr.Logger
	probeDuration  time.Duration
	sessionTimeout time.Duration
}

func AddHubManagement(mgr ctrl.Manager) error {
	return mgr.Add(&hubManagement{log: ctrl.Log.WithName("hub-management"),
		probeDuration: ProbeDuration, sessionTimeout: SessionTimeout})
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
	lastTime := time.Now().Add(-h.sessionTimeout)
	db := database.GetGorm()

	// filter the inactive hubs
	var expiredHubs []models.LeafHubHeartbeat
	err := db.Where("last_timestamp < ? AND status = ?", lastTime, HubActive).Find(&expiredHubs).Error
	if err != nil || len(expiredHubs) == 0 {
		return err
	}
	if err := h.inactive(ctx, expiredHubs); err != nil {
		return fmt.Errorf("inactive the hubs with error: %v", err)
	}

	// filter the reactive hubs
	var reactiveHubs []models.LeafHubHeartbeat
	err = db.Where("last_timestamp > ? AND status = ?", lastTime, HubInactive).Find(&reactiveHubs).Error
	if err != nil || len(reactiveHubs) == 0 {
		return err
	}
	if err := h.active(ctx, reactiveHubs); err != nil {
		return fmt.Errorf("reactive the hubs with error: %v", err)
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

func (h *hubManagement) active(ctx context.Context, hubs []models.LeafHubHeartbeat) error {
	// TODO: resync the resources has been cleanup

	// mark the status as active
	db := database.GetGorm()
	for i := range hubs {
		hubs[i].Status = HubActive
	}
	err := db.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(hubs, 100).Error
	if err != nil {
		return fmt.Errorf("failed to update the hubs to active: %v", err)
	}
	return nil
}
