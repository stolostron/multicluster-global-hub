// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubmanagement

import (
	"context"
	"fmt"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm/clause"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/test/integration/utils/testpostgres"
)

func TestHubManagement(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	testPostgres, err := testpostgres.NewTestPostgres()
	assert.Nil(t, err)
	err = testpostgres.InitDatabase(testPostgres.URI)
	assert.Nil(t, err)

	// prepare data
	now := time.Now()
	hubs := []models.LeafHubHeartbeat{
		{
			Name:         "heartbeat-hub01",
			LastUpdateAt: now,
		},
		{
			Name:         "heartbeat-hub02",
			LastUpdateAt: now.Add(-120 * time.Second),
			Status:       HubActive,
		},
		{
			Name:         "heartbeat-hub03",
			LastUpdateAt: now.Add(-20 * time.Second),
			Status:       HubActive,
		},
		{
			Name:         "heartbeat-hub04",
			LastUpdateAt: now.Add(-180 * time.Second),
			Status:       HubInactive,
		},
	}
	db := database.GetGorm()
	err = db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&hubs).Error
	assert.Nil(t, err)

	var heartbeatHubs []models.LeafHubHeartbeat
	err = db.Find(&heartbeatHubs).Error
	assert.Nil(t, err)

	assert.Greater(t, len(heartbeatHubs), 0)
	for _, heartbeatHub := range heartbeatHubs {
		fmt.Println(heartbeatHub.Name, heartbeatHub.LastUpdateAt, heartbeatHub.Status)
		if heartbeatHub.Name == "heartbeat-hub04" {
			assert.Equal(t, HubInactive, heartbeatHub.Status)
			continue
		}
		assert.Equal(t, heartbeatHub.Status, HubActive)
	}

	// only update the heartbeat Time
	hub4 := models.LeafHubHeartbeat{
		Name:         "heartbeat-hub04",
		LastUpdateAt: now.Add(-60 * time.Second),
	}
	fmt.Println(">> heartbeat: heartbeat-hub04 ")
	err = db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&hub4).Error
	assert.Nil(t, err)

	var updatedHub4 models.LeafHubHeartbeat
	err = db.Where("leaf_hub_name = ?", "heartbeat-hub04").Find(&updatedHub4).Error
	assert.Nil(t, err)
	fmt.Println(hub4.Name, hub4.LastUpdateAt, hub4.Status)
	assert.Equal(t, HubInactive, hub4.Status, "heartbeat not update the status") // status isn't is updated
	timeFormat := "2006-01-02 15:04:05"                                          // timestamp is updated
	assert.Equal(t, now.Add(-60*time.Second).Format(timeFormat), updatedHub4.LastUpdateAt.Format(timeFormat))

	// update
	hubManagement := &hubManagement{
		log:           ctrl.Log.WithName("hub-management"),
		producer:      &tmpProducer{},
		probeDuration: 1 * time.Second,
		activeTimeout: 90 * time.Second,
	}
	assert.Nil(t, hubManagement.Start(ctx))
	time.Sleep(3 * time.Second)

	fmt.Println(">> hub management[90s]: heartbeat-hub02 -> inactive, heartbeat-hub04 -> active")
	var updatedHubs []models.LeafHubHeartbeat
	err = db.Find(&updatedHubs).Error
	assert.Nil(t, err)
	assert.Greater(t, len(updatedHubs), 0)
	for _, updatedHub := range updatedHubs {
		fmt.Println(updatedHub.Name, updatedHub.LastUpdateAt, updatedHub.Status)
		if updatedHub.Name == "heartbeat-hub02" {
			assert.Equal(t, HubInactive, updatedHub.Status)
			continue
		}
		assert.Equal(t, HubActive, updatedHub.Status)
	}

	// close
	cancel()
	err = testPostgres.Stop()
	assert.Nil(t, err)
}

type tmpProducer struct{}

func (p *tmpProducer) SendEvent(ctx context.Context, evt cloudevents.Event) error {
	return nil
}
