package jobs

import (
	"context"
	"strconv"
	"time"

	"github.com/go-logr/logr"
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

type PruneFinalizer struct {
	ctx       context.Context
	log       logr.Logger
	client    client.Client
	finalizer string
}

func NewPruneFinalizer(ctx context.Context, runtimeClient client.Client) Runnable {
	return &PruneFinalizer{
		ctx:       ctx,
		log:       ctrl.Log.WithName("prune-finalizer-job"),
		client:    runtimeClient,
		finalizer: commonconstants.GlobalHubCleanupFinalizer,
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
		labels[commonconstants.GlobalHubFinalizerRemovingDeadline] = strconv.FormatInt(time.Now().Unix()+60, 10)
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

	p.log.Info("clean up the application placementrule finalizer")
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

	p.log.Info("the global hub finalizer of placement resources are cleaned up")
	return nil
}

func (p *PruneFinalizer) pruneApplication() error {
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
	p.log.Info("the global hub finalizer of application resources are cleaned up")
	return nil
}

func (p *PruneFinalizer) prunePolicy() error {
	p.log.Info("clean up the policies finalizer")
	policies := &policyv1.PolicyList{}
	if err := p.client.List(p.ctx, policies, &client.ListOptions{}); err != nil {
		return err
	}
	for idx := range policies.Items {
		if err := p.pruneFinalizer(&policies.Items[idx]); err != nil {
			return err
		}
	}
	p.log.Info("the policy global hub finalizer are cleaned up")
	return nil
}
