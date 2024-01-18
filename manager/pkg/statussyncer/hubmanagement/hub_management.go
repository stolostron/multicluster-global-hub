// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubmanagement

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"gorm.io/gorm/clause"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

const (
	HubActive   = "active"
	HubInactive = "inactive"

	// heartbeatInterval = 1 * time.Minute
	SessionTimeout = 5 * time.Minute // if heartbeat < (now - SessionTimeout), then status = inactive, vice versa
	ProbeDuration  = 2 * time.Minute // the duration to detect run the updating
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

	var expiredHubs []models.LeafHubHeartbeat
	err := db.Model(&expiredHubs).
		Clauses(clause.Returning{Columns: []clause.Column{{Name: "leaf_hub_name"}}}).
		Where("last_timestamp < ? AND status = ?", lastTime, HubActive).
		Update("status", HubInactive).Error
	if err != nil {
		return err
	}
	if err := h.inactive(ctx, expiredHubs); err != nil {
		return err
	}

	var reactiveHubs []models.LeafHubHeartbeat
	err = db.Model(&reactiveHubs).
		Clauses(clause.Returning{Columns: []clause.Column{{Name: "leaf_hub_name"}}}).
		Where("last_timestamp > ? AND status = ?", lastTime, HubInactive).
		Update("status", HubActive).Error
	if err != nil {
		return err
	}
	if err := h.reactive(ctx, reactiveHubs); err != nil {
		return err
	}

	return nil
}

func (h *hubManagement) inactive(ctx context.Context, hubs []models.LeafHubHeartbeat) error {
	// TODO: cleanup the database resources

	return nil
}

func (h *hubManagement) reactive(ctx context.Context, hubs []models.LeafHubHeartbeat) error {
	// TODO: resync the resources has been cleanup

	return nil
}
