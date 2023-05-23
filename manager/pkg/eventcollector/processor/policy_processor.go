package processor

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/resmoio/kubernetes-event-exporter/pkg/kube"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

type policyProcessor struct {
	log           logr.Logger
	ctx           context.Context
	pool          *pgxpool.Pool
	offsetManager OffsetManager
}

func NewPolicyProcessor(ctx context.Context, pool *pgxpool.Pool, offsetManager OffsetManager) *policyProcessor {
	return &policyProcessor{
		log:           ctrl.Log.WithName("policy-event-processor"),
		ctx:           ctx,
		pool:          pool,
		offsetManager: offsetManager,
	}
}

func (p *policyProcessor) Process(event *kube.EnhancedEvent, eventOffset *EventOffset) {
	p.log.Info("process policy event", "name", fmt.Sprintf("%s - %s/%s", event.ClusterName,
		event.InvolvedObject.Namespace, event.InvolvedObject.Name), "count",
		fmt.Sprintf("%d", event.Count), "lastTimestamp", event.LastTimestamp)

	// predicate
	clusterId, ok := event.InvolvedObject.Labels[constants.PolicyEventClusterIdLabelKey]
	if !ok {
		return
	}
	rootPolicyId, ok := event.InvolvedObject.Labels[constants.PolicyEventRootPolicyIdLabelKey]
	if !ok {
		return
	}
	clusterCompliance, ok := event.InvolvedObject.Labels[constants.PolicyEventClusterComplianceLabelKey]
	if !ok {
		return
	}
	// algin with the database enum values
	compliance := "unknown"
	switch clusterCompliance {
	case string(policyv1.Compliant):
		compliance = "compliant"
	case string(policyv1.NonCompliant):
		compliance = "non_compliant"
	default:
		p.log.Info("unknown compliance status", "compliance", clusterCompliance)
	}

	localPolicyEvent := &LocalPolicyEvent{
		PolicyID:   rootPolicyId,
		ClusterID:  clusterId,
		Message:    event.Message,
		Reason:     event.Reason,
		Source:     event.Source,
		CreatedAt:  event.LastTimestamp.Time,
		Compliance: compliance,
	}

	ctx, cancel := context.WithTimeout(p.ctx, 1*time.Minute)
	defer cancel()
	err := wait.PollUntilWithContext(ctx, 10*time.Second, func(ctx context.Context) (bool, error) {
		err := insertOrUpdate(ctx, p.pool, localPolicyEvent)
		if err != nil {
			p.log.Error(err, "insert or update local_policies failed, retrying...")
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		p.log.Error(err, "insert or update local_policies failed")
		return
	}
	p.offsetManager.MarkOffset(eventOffset.Topic, eventOffset.Partition, eventOffset.Offset)
}

type LocalPolicyEvent struct {
	PolicyID   string
	ClusterID  string
	Message    string
	Reason     string
	Source     corev1.EventSource
	CreatedAt  time.Time
	Compliance string
}

func insertOrUpdate(ctx context.Context, pool *pgxpool.Pool, policyEvent *LocalPolicyEvent) error {
	insertSql := `
		INSERT INTO event.local_policies (policy_id, cluster_id, message, reason, source, created_at, compliance)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (policy_id, cluster_id, created_at) DO UPDATE
		SET message = excluded.message, reason = excluded.reason, source = excluded.source, compliance = excluded.compliance
	`
	_, err := pool.Exec(ctx, insertSql, policyEvent.PolicyID, policyEvent.ClusterID, policyEvent.Message,
		policyEvent.Reason, policyEvent.Source, policyEvent.CreatedAt, policyEvent.Compliance)
	return err
}
