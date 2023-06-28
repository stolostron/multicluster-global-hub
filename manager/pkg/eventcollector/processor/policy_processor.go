package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/resmoio/kubernetes-event-exporter/pkg/kube"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/common"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

type policyProcessor struct {
	log           logr.Logger
	ctx           context.Context
	db            *gorm.DB
	offsetManager OffsetManager
}

func NewPolicyProcessor(ctx context.Context, offsetManager OffsetManager) *policyProcessor {
	return &policyProcessor{
		log:           ctrl.Log.WithName("policy-event-processor"),
		ctx:           ctx,
		db:            database.GetGorm(),
		offsetManager: offsetManager,
	}
}

func (p *policyProcessor) Process(event *kube.EnhancedEvent, eventOffset *EventOffset) {
	p.log.Info(event.ClusterName, "namespace", event.Namespace, "name", event.Name, "count",
		fmt.Sprintf("%d", event.Count), "offset", fmt.Sprintf("%d", eventOffset.Offset))

	clusterCompliance, ok := event.InvolvedObject.Labels[constants.PolicyEventComplianceLabelKey]
	if !ok {
		return
	}
	compliance := common.GetDatabaseCompliance(clusterCompliance)

	clusterId, hasClusterId := event.InvolvedObject.Labels[constants.PolicyEventClusterIdLabelKey]
	rootPolicyId, hasRootPolicyId := event.InvolvedObject.Labels[constants.PolicyEventRootPolicyIdLabelKey]

	source, err := json.Marshal(event.Source)
	if err != nil {
		p.log.Error(err, "failed to marshal event")
		return
	}
	baseLocalPolicyEvent := &models.BaseLocalPolicyEvent{
		EventName:   event.Name,
		Message:     event.Message,
		Reason:      event.Reason,
		LeafHubName: event.ClusterName,
		Source:      source,
		Count:       int(event.Count),
		Compliance:  string(compliance),
		CreatedAt:   event.LastTimestamp.Time,
	}

	var insertEvent interface{}
	conflictColumns := []clause.Column{{Name: "event_name"}, {Name: "count"}}
	if hasClusterId || hasRootPolicyId {
		baseLocalPolicyEvent.PolicyID = rootPolicyId
		insertEvent = &models.LocalClusterPolicyEvent{
			BaseLocalPolicyEvent: *baseLocalPolicyEvent,
			ClusterID:            clusterId,
		}
	} else {
		baseLocalPolicyEvent.PolicyID = string(event.InvolvedObject.UID)
		insertEvent = &models.LocalRootPolicyEvent{
			BaseLocalPolicyEvent: *baseLocalPolicyEvent,
		}
	}

	ctx, cancel := context.WithTimeout(p.ctx, 1*time.Minute)
	defer cancel()
	err = wait.PollUntilWithContext(ctx, 10*time.Second, func(ctx context.Context) (bool, error) {
		result := p.db.Clauses(clause.OnConflict{
			Columns:   conflictColumns,
			UpdateAll: true,
		}).Create(insertEvent)
		if result.Error != nil {
			p.log.Error(result.Error, "insert or update local (root) policy event failed, retrying...")
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		p.log.Error(err, "insert or update local (root) policy event failed")
		return
	}
	p.offsetManager.MarkOffset(eventOffset.Topic, eventOffset.Partition, eventOffset.Offset)
}
