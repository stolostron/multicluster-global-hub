package jobs

import (
	"context"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/client-go/util/retry"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	clusterv1beta2 "open-cluster-management.io/api/cluster/v1beta2"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	chnv1 "open-cluster-management.io/multicloud-operators-channel/pkg/apis/apps/v1"
	placementrulesv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	appsubv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1"
	appv1beta1 "sigs.k8s.io/application/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

type PruneFinalizer struct {
	ctx       context.Context
	log       logr.Logger
	client    client.Client
	finalizer string
}

// only delete the finalizer from the global resources.
var globalResourceListOptions = []client.ListOption{
	client.MatchingLabels(map[string]string{
		constants.GlobalHubGlobalResourceLabel: "",
	}),
}

func NewPruneFinalizer(ctx context.Context, runtimeClient client.Client) Runnable {
	return &PruneFinalizer{
		ctx:       ctx,
		log:       ctrl.Log.WithName("prune-finalizer-job"),
		client:    runtimeClient,
		finalizer: constants.GlobalHubCleanupFinalizer,
	}
}

func (p *PruneFinalizer) Run() error {
	if err := p.prunePlacementResources(); err != nil {
		return err
	}
	if err := p.pruneApplication(); err != nil {
		return err
	}
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

func (p *PruneFinalizer) prunePlacementResources() error {
	p.log.Info("clean up the placement finalizer")
	placements := &clusterv1beta1.PlacementList{}
	if err := p.client.List(p.ctx, placements, globalResourceListOptions...); err != nil {
		p.log.Error(err, "failed to list placements")
	}
	for idx := range placements.Items {
		if err := p.pruneFinalizer(&placements.Items[idx]); err != nil {
			return err
		}
	}

	p.log.Info("clean up the managedclusterset finalizer")
	managedclustersets := &clusterv1beta2.ManagedClusterSetList{}
	if err := p.client.List(p.ctx, managedclustersets, globalResourceListOptions...); err != nil {
		p.log.Error(err, "failed to list managedclustersets")
	}
	for idx := range managedclustersets.Items {
		if err := p.pruneFinalizer(&managedclustersets.Items[idx]); err != nil {
			return err
		}
	}

	p.log.Info("clean up the managedclustersetbinding finalizer")
	managedclustersetbindings := &clusterv1beta2.ManagedClusterSetBindingList{}
	if err := p.client.List(p.ctx, managedclustersetbindings, globalResourceListOptions...); err != nil {
		p.log.Error(err, "failed to list managedclustersetbindings")
	}
	for idx := range managedclustersetbindings.Items {
		if err := p.pruneFinalizer(&managedclustersetbindings.Items[idx]); err != nil {
			return err
		}
	}

	p.log.Info("clean up the managedclusterset finalizer")
	managedclustersetv1beta1 := &clusterv1beta1.ManagedClusterSetList{}
	if err := p.client.List(p.ctx, managedclustersetv1beta1, globalResourceListOptions...); err != nil {
		p.log.Error(err, "failed to list managedclustersets")
	}
	for idx := range managedclustersetv1beta1.Items {
		if err := p.pruneFinalizer(&managedclustersetv1beta1.Items[idx]); err != nil {
			return err
		}
	}

	p.log.Info("clean up the managedclustersetbinding finalizer")
	managedclustersetbindingv1beta1 := &clusterv1beta1.ManagedClusterSetBindingList{}
	if err := p.client.List(p.ctx, managedclustersetbindingv1beta1, globalResourceListOptions...); err != nil {
		p.log.Error(err, "failed to list managedclustersetbindings")
	}
	for idx := range managedclustersetbindingv1beta1.Items {
		if err := p.pruneFinalizer(&managedclustersetbindingv1beta1.Items[idx]); err != nil {
			return err
		}
	}

	p.log.Info("clean up the application placementrule finalizer")
	palcementrules := &placementrulesv1.PlacementRuleList{}
	if err := p.client.List(p.ctx, palcementrules, globalResourceListOptions...); err != nil {
		p.log.Error(err, "failed to list placementrules")
	}
	for idx := range palcementrules.Items {
		if err := p.pruneFinalizer(&palcementrules.Items[idx]); err != nil {
			return err
		}
	}

	p.log.Info("clean up the placementbindings finalizer")
	placementbindings := &policyv1.PlacementBindingList{}
	if err := p.client.List(p.ctx, placementbindings, globalResourceListOptions...); err != nil {
		p.log.Error(err, "failed to list placementbindings")
	}
	for idx := range placementbindings.Items {
		if err := p.pruneFinalizer(&placementbindings.Items[idx]); err != nil {
			return err
		}
	}

	p.log.Info("the global hub finalizer of placement resources are cleaned up")
	return nil
}

func (p *PruneFinalizer) pruneApplication() error {
	p.log.Info("clean up the application finalizer")
	applications := &appv1beta1.ApplicationList{}
	if err := p.client.List(p.ctx, applications, globalResourceListOptions...); err != nil {
		p.log.Error(err, "failed to list applications")
	}
	for idx := range applications.Items {
		if err := p.pruneFinalizer(&applications.Items[idx]); err != nil {
			return err
		}
	}

	p.log.Info("clean up the application subscription finalizer")
	appsubs := &appsubv1.SubscriptionList{}
	if err := p.client.List(p.ctx, appsubs, globalResourceListOptions...); err != nil {
		p.log.Error(err, "failed to list application subscriptions")
	}
	for idx := range appsubs.Items {
		if err := p.pruneFinalizer(&appsubs.Items[idx]); err != nil {
			return err
		}
	}

	p.log.Info("clean up the application channel finalizer")
	channels := &chnv1.ChannelList{}
	if err := p.client.List(p.ctx, channels, globalResourceListOptions...); err != nil {
		p.log.Error(err, "failed to list application channels")
	}
	for idx := range channels.Items {
		if err := p.pruneFinalizer(&channels.Items[idx]); err != nil {
			return err
		}
	}
	p.log.Info("the global hub finalizer of application resources are cleaned up")
	return nil
}

func (p *PruneFinalizer) prunePolicy() error {
	p.log.Info("clean up the policies finalizer")
	policies := &policyv1.PolicyList{}
	if err := p.client.List(p.ctx, policies, globalResourceListOptions...); err != nil {
		p.log.Error(err, "failed to list policies")
	}
	for idx := range policies.Items {
		if err := p.pruneFinalizer(&policies.Items[idx]); err != nil {
			return err
		}
	}
	p.log.Info("the policy global hub finalizer are cleaned up")
	return nil
}
