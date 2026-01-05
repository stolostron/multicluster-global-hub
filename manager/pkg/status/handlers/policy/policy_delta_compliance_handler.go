package policy

import (
	"context"
	"fmt"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator/dependency"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/grc"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

type policyDeltaComplianceHandler struct {
	log            *zap.SugaredLogger
	eventType      string
	dependencyType string
	eventSyncMode  enum.EventSyncMode
	eventPriority  conflator.ConflationPriority
}

func RegisterPolicyDeltaComplianceHandler(conflationManager *conflator.ConflationManager) {
	eventType := string(enum.DeltaComplianceType)
	logName := strings.ReplaceAll(eventType, enum.EventTypePrefix, "")
	h := &policyDeltaComplianceHandler{
		log:            logger.ZapLogger(logName),
		eventType:      eventType,
		dependencyType: string(enum.CompleteComplianceType),
		eventSyncMode:  enum.DeltaStateMode,
		eventPriority:  conflator.DeltaCompliancePriority,
	}
	registration := conflator.NewConflationRegistration(
		h.eventPriority,
		h.eventSyncMode,
		h.eventType,
		h.handleEvent,
	)
	registration.WithDependency(dependency.NewDependency(h.dependencyType, dependency.ExactMatch))
	conflationManager.Register(registration)
}

// if we got to the handler function, then the bundle pre-conditions were satisfied.
func (h *policyDeltaComplianceHandler) handleEvent(ctx context.Context, evt *cloudevents.Event) error {
	version := evt.Extensions()[eventversion.ExtVersion]
	leafHub := evt.Source()
	h.log.Debugw(startMessage, "type", enum.ShortenEventType(evt.Type()), "LH", evt.Source(), "version", version)

	data := grc.ComplianceBundle{}
	if err := evt.DataAs(&data); err != nil {
		return err
	}

	db := database.GetGorm()
	err := db.Transaction(func(tx *gorm.DB) error {
		for _, eventCompliance := range data { // every object in bundle is policy generic compliance status

			for _, cluster := range eventCompliance.CompliantClusters {
				err := updateCompliance(tx, eventCompliance.PolicyID, leafHub, cluster, database.Compliant)
				if err != nil {
					return err
				}
			}

			for _, cluster := range eventCompliance.NonCompliantClusters {
				err := updateCompliance(tx, eventCompliance.PolicyID, leafHub, cluster, database.NonCompliant)
				if err != nil {
					return err
				}
			}

			for _, cluster := range eventCompliance.UnknownComplianceClusters {
				err := updateCompliance(tx, eventCompliance.PolicyID, leafHub, cluster, database.Unknown)
				if err != nil {
					return err
				}
			}

			for _, cluster := range eventCompliance.PendingComplianceClusters {
				err := updateCompliance(tx, eventCompliance.PolicyID, leafHub, cluster, database.Pending)
				if err != nil {
					return err
				}
			}
		}

		// return nil will commit the whole transaction
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to handle delta compliance bundle - %w", err)
	}

	h.log.Debugw(finishMessage, "type", enum.ShortenEventType(evt.Type()), "LH", evt.Source(), "version", version)
	return nil
}

func updateCompliance(tx *gorm.DB, policyID, leafHub, cluster string, compliance database.ComplianceStatus) error {
	return tx.Model(&models.StatusCompliance{}).Where(&models.StatusCompliance{
		PolicyID:    policyID,
		ClusterName: cluster,
		LeafHubName: leafHub,
	}).Updates(&models.StatusCompliance{
		Compliance: compliance,
	}).Error
}
