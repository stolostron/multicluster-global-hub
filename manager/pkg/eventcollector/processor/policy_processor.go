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
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
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
		fmt.Sprintf("%d", event.Count))

	clusterCompliance, ok := event.InvolvedObject.Labels[constants.PolicyEventComplianceLabelKey]
	if !ok {
		return
	}
	compliance := p.getPolicyCompliance(clusterCompliance)

	clusterId, hasClusterId := event.InvolvedObject.Labels[constants.PolicyEventClusterIdLabelKey]
	rootPolicyId, hasRootPolicyId := event.InvolvedObject.Labels[constants.PolicyEventRootPolicyIdLabelKey]

	source, err := json.Marshal(event.Source)
	if err != nil {
		p.log.Error(err, "failed to marshal event")
		return
	}
	baseLocalPolicyEvent := &models.BaseLocalPolicyEvent{
		Message:     event.Message,
		Reason:      event.Reason,
		LeafHubName: event.ClusterName,
		Source:      source,
		Count:       int(event.Count),
		Compliance:  string(compliance),
		CreatedAt:   event.LastTimestamp.Time,
	}

	var insertEvent interface{}
	conflictColumns := []clause.Column{{Name: "policy_id"}, {Name: "count"}}
	if hasClusterId || hasRootPolicyId {
		baseLocalPolicyEvent.PolicyID = rootPolicyId
		insertEvent = &models.LocalClusterPolicyEvent{
			BaseLocalPolicyEvent: *baseLocalPolicyEvent,
			ClusterID:            clusterId,
		}
		conflictColumns = append(conflictColumns, clause.Column{Name: "cluster_id"})
	} else {
		baseLocalPolicyEvent.PolicyID = string(event.InvolvedObject.UID)
		insertEvent = &models.LocalRootPolicyEvent{
			BaseLocalPolicyEvent: *baseLocalPolicyEvent,
		}
		conflictColumns = append(conflictColumns, clause.Column{Name: "leaf_hub_name"})
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

func (p *policyProcessor) getPolicyCompliance(compliance string) database.ComplianceStatus {
	// algin with the database enum values
	status := database.Unknown
	switch compliance {
	case string(policyv1.Compliant):
		status = database.Compliant
	case string(policyv1.NonCompliant):
		status = database.NonCompliant
	default:
		p.log.Info("unknown compliance status", "compliance", compliance)
	}
	return status
}
