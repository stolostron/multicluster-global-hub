package processor

import (
	"github.com/go-logr/logr"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/resmoio/kubernetes-event-exporter/pkg/kube"
	ctrl "sigs.k8s.io/controller-runtime"
)

type policyProcessor struct {
	log  logr.Logger
	pool *pgxpool.Pool
}

func NewPolicyProcessor(pool *pgxpool.Pool) *policyProcessor {
	return &policyProcessor{
		log:  ctrl.Log.WithName("policy-event-processor"),
		pool: pool,
	}
}

func (p *policyProcessor) Process(event *kube.EnhancedEvent) {
	// parse the message to policy event
	// check if the policy event is valid
	// if valid, update the policy status
	// if invalid, log the error and return
	p.log.Info("process policy event", "event", event)
}
