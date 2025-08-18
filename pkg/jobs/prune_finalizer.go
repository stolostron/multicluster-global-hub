package jobs

import (
	"context"
	"strconv"
	"time"

	"k8s.io/client-go/util/retry"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

var log = logger.DefaultZapLogger()

type PruneFinalizer struct {
	ctx       context.Context
	client    client.Client
	finalizer string
}

func NewPruneFinalizer(ctx context.Context, runtimeClient client.Client) Runnable {
	return &PruneFinalizer{
		ctx:       ctx,
		client:    runtimeClient,
		finalizer: "",
	}
}

func (p *PruneFinalizer) Run() error {
	if err := p.prunePolicy(); err != nil {
		return err
	}
	return nil
}

func (p *PruneFinalizer) pruneFinalizer(object client.Object) error {
	if controllerutil.RemoveFinalizer(object, p.finalizer) {
		labels := object.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		// set the removing finalizer ttl with 60 seconds
		labels[constants.GlobalHubFinalizerRemovingDeadline] = strconv.FormatInt(time.Now().Unix()+60, 10)
		object.SetLabels(labels)
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			return p.client.Update(p.ctx, object, &client.UpdateOptions{})
		})
		if err != nil {
			return err
		}
	}
	return nil
}



func (p *PruneFinalizer) prunePolicy() error {
	log.Info("clean up the policies finalizer")
	policies := &policyv1.PolicyList{}
	if err := p.client.List(p.ctx, policies, &client.ListOptions{}); err != nil {
		log.Error(err, "failed to list policies")
	}
	for idx := range policies.Items {
		if err := p.pruneFinalizer(&policies.Items[idx]); err != nil {
			return err
		}
	}
	log.Info("the policy global hub finalizer are cleaned up")
	return nil
}
