package policy

import (
	"context"
	"fmt"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	set "github.com/deckarep/golang-set"
	"go.uber.org/zap"
	"gorm.io/gorm/clause"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/grc"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

type policyMiniComplianceHandler struct {
	log           *zap.SugaredLogger
	eventType     string
	eventSyncMode enum.EventSyncMode
	eventPriority conflator.ConflationPriority
}

func RegisterPolicyMiniComplianceHandler(conflationManager *conflator.ConflationManager) {
	eventType := string(enum.MiniComplianceType)
	logName := strings.ReplaceAll(eventType, enum.EventTypePrefix, "")
	h := &policyMiniComplianceHandler{
		log:           logger.ZapLogger(logName),
		eventType:     eventType,
		eventSyncMode: enum.CompleteStateMode,
		eventPriority: conflator.MinimalCompliancePriority,
	}
	conflationManager.Register(conflator.NewConflationRegistration(
		h.eventPriority,
		h.eventSyncMode,
		h.eventType,
		h.handleEvent,
	))
}

// if we got to the handler function, then the bundle pre-conditions are satisfied.
func (h *policyMiniComplianceHandler) handleEvent(ctx context.Context, evt *cloudevents.Event) error {
	version := evt.Extensions()[eventversion.ExtVersion]
	leafHub := evt.Source()
	table := database.StatusSchema + "." + database.MinimalComplianceTable

	h.log.Debugw(startMessage, "type", evt.Type(), "LH", evt.Source(), "version", version)

	data := make([]grc.MinimalCompliance, 0)
	if err := evt.DataAs(&data); err != nil {
		return err
	}

	// exist policy
	policyIDSetFromDB := set.NewSet()

	db := database.GetGorm()
	sql := fmt.Sprintf(`SELECT DISTINCT(policy_id) FROM %s WHERE leaf_hub_name = ?`, table)
	rows, err := db.Raw(sql, leafHub).Rows()
	if err != nil {
		return fmt.Errorf("error reading from table %s - %w", table, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var policyID string
		if err := rows.Scan(&policyID); err != nil {
			return fmt.Errorf("error reading from table %s - %w", table, err)
		}
		policyIDSetFromDB.Add(policyID)
	}

	for _, minPolicyCompliance := range data { // every object in bundle is minimal policy compliance status.
		err := db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "policy_id"}, {Name: "leaf_hub_name"}},
			UpdateAll: true,
		}).Create(&models.AggregatedCompliance{
			PolicyID:             minPolicyCompliance.PolicyID,
			LeafHubName:          leafHub,
			AppliedClusters:      minPolicyCompliance.AppliedClusters,
			NonCompliantClusters: minPolicyCompliance.NonCompliantClusters,
		}).Error
		if err != nil {
			return fmt.Errorf("failed to InsertUpdate minimal compliance of policy '%s', leaf hub '%s' in db - %w",
				minPolicyCompliance.PolicyID, leafHub, err)
		}
		// eventually we will be left with policies not in the bundle inside policyIDsFromDB and will use it to remove
		// policies that has to be deleted from the table.
		policyIDSetFromDB.Remove(minPolicyCompliance.PolicyID)
	}

	// remove policies that in the db but were not sent in the bundle (leaf hub sends only living resources).
	for _, object := range policyIDSetFromDB.ToSlice() {
		policyID, ok := object.(string)
		if !ok {
			continue
		}
		ret := db.Where(&models.AggregatedCompliance{
			PolicyID:    policyID,
			LeafHubName: leafHub,
		}).Delete(&models.AggregatedCompliance{})
		if ret.Error != nil {
			return fmt.Errorf("failed to delete minimal compliance of policy '%s', leaf hub '%s' from db - %w",
				policyID, leafHub, ret.Error)
		}
	}

	h.log.Debugw(finishMessage, "type", evt.Type(), "LH", evt.Source(), "version", version)
	return nil
}
