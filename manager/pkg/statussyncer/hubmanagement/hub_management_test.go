// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubmanagement

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gorm.io/gorm/clause"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/test/pkg/testpostgres"
)

func TestHubManagement(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	testPostgres, err := testpostgres.NewTestPostgres()
	assert.Nil(t, err)
	err = testpostgres.InitDatabase(testPostgres.URI)
	assert.Nil(t, err)

	// insert data
	hubs := []models.LeafHubHeartbeat{
		{
			Name:         "heartbeat-hub01",
			LastUpdateAt: time.Now(),
		},
		{
			Name:         "heartbeat-hub02",
			LastUpdateAt: time.Now().Add(-100 * time.Second),
		},
		{
			Name:         "heartbeat-hub03",
			LastUpdateAt: time.Now().Add(-20 * time.Second),
		},
	}
	db := database.GetGorm()
	err = db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&hubs).Error
	assert.Nil(t, err)

	var heartbeatHubs []models.LeafHubHeartbeat
	err = db.Find(&heartbeatHubs).Error
	assert.Nil(t, err)
	for _, heartbeatHub := range heartbeatHubs {
		fmt.Println(heartbeatHub.Name, heartbeatHub.LastUpdateAt, heartbeatHub.Status)
	}

	// update
	hubManagement := &hubManagement{
		log:           ctrl.Log.WithName("hub-management"),
		probeInterval: 90 * time.Second,
	}
	assert.Nil(t, hubManagement.Start(ctx))
	err = hubManagement.update(ctx)
	assert.Nil(t, err)

	fmt.Println("hub management updated")
	var updatedHubs []models.LeafHubHeartbeat
	err = db.Find(&updatedHubs).Error
	assert.Nil(t, err)
	inactiveCount := 0
	for _, updatedHub := range updatedHubs {
		fmt.Println(updatedHub.Name, updatedHub.LastUpdateAt, updatedHub.Status)
		if updatedHub.Status == HubInactive {
			inactiveCount++
		}
	}
	assert.Equal(t, 1, inactiveCount)

	// close
	cancel()
	testPostgres.Stop()
}
