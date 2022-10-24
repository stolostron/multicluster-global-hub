package jobs

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/open-horizon/edge-utilities/logger/log"
	"k8s.io/client-go/util/retry"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	chnv1 "open-cluster-management.io/multicloud-operators-channel/pkg/apis/apps/v1"
	placementrulesv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	appsubv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1"
	appv1beta1 "sigs.k8s.io/application/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	commonconstants "github.com/stolostron/multicluster-global-hub/pkg/constants"
)

type PruneJob struct {
	ctx       context.Context
	log       logr.Logger
	client    client.Client
	finalizer string
}

type Runnable interface {
	Run() int
}

func NewPruneJob(runtimeClient client.Client) Runnable {
	return &PruneJob{
		ctx:       context.Background(),
		log:       ctrl.Log.WithName("global-hub-agent-prune-job"),
		client:    runtimeClient,
		finalizer: commonconstants.GlobalHubCleanupFinalizer,
	}
}

func (p *PruneJob) Run() int {
	if err := p.prunePlacementResources(); err != nil {
		p.log.Error(err, "prune placements resources error")
		return 1
	}
	if err := p.pruneApplication(); err != nil {
		p.log.Error(err, "prune application resources error")
		return 1
	}
	if err := p.prunePolicy(); err != nil {
		p.log.Error(err, "prune policy resources error")
		return 1
	}
	return 0
}

func (p *PruneJob) pruneFinalizer(object client.Object) error {
	if controllerutil.RemoveFinalizer(object, p.finalizer) {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			return p.client.Update(p.ctx, object, &client.UpdateOptions{})
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *PruneJob) prunePlacementResources() error {
	p.log.Info("clean up the placement finalizer")
	placements := &clusterv1beta1.PlacementList{}
	if err := p.client.List(p.ctx, placements, &client.ListOptions{}); err != nil {
		return err
	}
	for idx := range placements.Items {
		if err := p.pruneFinalizer(&placements.Items[idx]); err != nil {
			return err
		}
	}

	p.log.Info("clean up the managedclusterset finalizer")
	managedclustersets := &clusterv1beta1.ManagedClusterSetList{}
	if err := p.client.List(p.ctx, managedclustersets, &client.ListOptions{}); err != nil {
		return err
	}
	for idx := range managedclustersets.Items {
		if err := p.pruneFinalizer(&managedclustersets.Items[idx]); err != nil {
			return err
		}
	}

	p.log.Info("clean up the managedclustersetbinding finalizer")
	managedclustersetbindings := &clusterv1beta1.ManagedClusterSetBindingList{}
	if err := p.client.List(p.ctx, managedclustersetbindings, &client.ListOptions{}); err != nil {
		return err
	}
	for idx := range managedclustersetbindings.Items {
		if err := p.pruneFinalizer(&managedclustersetbindings.Items[idx]); err != nil {
			return err
		}
	}

	log.Info("clean up the application placementrule finalizer")
	palcementrules := &placementrulesv1.PlacementRuleList{}
	if err := p.client.List(p.ctx, palcementrules, &client.ListOptions{}); err != nil {
		return err
	}
	for idx := range palcementrules.Items {
		if err := p.pruneFinalizer(&palcementrules.Items[idx]); err != nil {
			return err
		}
	}

	p.log.Info("clean up the placementbindings finalizer")
	placementbindings := &policyv1.PlacementBindingList{}
	if err := p.client.List(p.ctx, placementbindings, &client.ListOptions{}); err != nil {
		return err
	}
	for idx := range placementbindings.Items {
		if err := p.pruneFinalizer(&placementbindings.Items[idx]); err != nil {
			return err
		}
	}

	p.log.Info("placement resources are cleaned up")
	return nil
}

func (p *PruneJob) pruneApplication() error {
	p.log.Info("clean up the application finalizer")
	applications := &appv1beta1.ApplicationList{}
	if err := p.client.List(p.ctx, applications, &client.ListOptions{}); err != nil {
		return err
	}
	for idx := range applications.Items {
		if err := p.pruneFinalizer(&applications.Items[idx]); err != nil {
			return err
		}
	}

	p.log.Info("clean up the application subscription finalizer")
	appsubs := &appsubv1.SubscriptionList{}
	if err := p.client.List(p.ctx, appsubs, &client.ListOptions{}); err != nil {
		return err
	}
	for idx := range appsubs.Items {
		if err := p.pruneFinalizer(&appsubs.Items[idx]); err != nil {
			return err
		}
	}

	p.log.Info("clean up the application channel finalizer")
	channels := &chnv1.ChannelList{}
	if err := p.client.List(p.ctx, channels, &client.ListOptions{}); err != nil {
		return err
	}
	for idx := range channels.Items {
		if err := p.pruneFinalizer(&channels.Items[idx]); err != nil {
			return err
		}
	}
	p.log.Info("multicluster-global-hub manager application are cleaned up")
	return nil
}

func (p *PruneJob) prunePolicy() error {
	log.Info("clean up the policies finalizer")
	policies := &policyv1.PolicyList{}
	if err := p.client.List(p.ctx, policies, &client.ListOptions{}); err != nil {
		return err
	}
	for idx := range policies.Items {
		if err := p.pruneFinalizer(&policies.Items[idx]); err != nil {
			return err
		}
	}
	log.Info("multicluster-global-hub manager policy are cleaned up")
	return nil
}
