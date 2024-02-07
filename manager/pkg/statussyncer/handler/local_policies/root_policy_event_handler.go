package localpolicies

import (
	"encoding/json"
	"fmt"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/types"
	"gorm.io/gorm/clause"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/handler"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/common"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

var _ handler.EventHandler = &localRootPolicyEventHandler{}

type localRootPolicyEventHandler struct {
	lastProcessedVersions map[string]metadata.BundleVersion
	eventType             enum.EventType
}

func NewLocalRootPolicyEventHandler() *localRootPolicyEventHandler {
	return &localRootPolicyEventHandler{
		eventType:             enum.LocalRootPolicyEventType,
		lastProcessedVersions: make(map[string]metadata.BundleVersion),
	}
}

func (h *localRootPolicyEventHandler) EventType() enum.EventType {
	return h.eventType
}

func (h *localRootPolicyEventHandler) ToDatabase(evt cloudevents.Event) error {
	versionStr, err := types.ToString(evt.Extensions()[metadata.ExtVersion])
	if err != nil {
		return err
	}
	eventVersion, err := metadata.BundleVersionFrom(versionStr)
	if err != nil {
		return err
	}

	// TODO: reset the lastProcessedVersion when agent restart
	lastProcessedVersion, ok := h.lastProcessedVersions[evt.Source()]
	if ok && !eventVersion.NewerThan(&lastProcessedVersion) {
		return nil
	}

	rootPolicyEvents := []event.RootPolicyEvent{}
	err = json.Unmarshal(evt.Data(), &rootPolicyEvents)
	if err != nil {
		return fmt.Errorf("failed to unmarshal event.Data to rootPolicyEvent")
	}

	if len(rootPolicyEvents) == 0 {
		return fmt.Errorf("the root policy event payload shouldn't be empty")
	}

	upsertEvents := []models.LocalRootPolicyEvent{}
	for _, element := range rootPolicyEvents {
		upsertEvents = append(upsertEvents, models.LocalRootPolicyEvent{
			BaseLocalPolicyEvent: models.BaseLocalPolicyEvent{
				LeafHubName: evt.Source(),
				EventName:   element.EventName,
				PolicyID:    element.PolicyID,
				Message:     element.Message,
				Reason:      element.Reason,
				Count:       int(element.Count),
				Compliance:  string(common.GetDatabaseCompliance(element.Compliance)),
				CreatedAt:   element.CreatedAt.Time,
			},
		})
	}

	db := database.GetGorm()
	err = db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "event_name"}, {Name: "count"}, {Name: "created_at"}},
		UpdateAll: true,
	}).CreateInBatches(upsertEvents, 100).Error
	if err != nil {
		return fmt.Errorf("failed to handle the event to database %v", err)
	}
	h.lastProcessedVersions[evt.Source()] = *eventVersion
	return nil
}
