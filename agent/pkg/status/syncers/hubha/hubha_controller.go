package hubha

import (
	"context"
	"fmt"
	"time"

	klusterletv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

const klusterletConfigPrefix = "ha-standby-"

type hubHAController struct {
	ctx            context.Context
	mgr            ctrl.Manager
	producer       transport.Producer
	periodicSyncer *generic.PeriodicSyncer
	resourceSyncer *hubHAResourceSyncer
}

func AddHubHAController(ctx context.Context, mgr ctrl.Manager, producer transport.Producer,
	periodicSyncer *generic.PeriodicSyncer,
) error {
	c := &hubHAController{
		ctx:            ctx,
		mgr:            mgr,
		producer:       producer,
		periodicSyncer: periodicSyncer,
	}

	pred := predicate.NewPredicateFuncs(func(obj client.Object) bool {
		standbyHub := configs.GetAgentConfig().GetStandbyHub()
		if standbyHub == "" {
			return false
		}
		return obj.GetName() == klusterletConfigPrefix+standbyHub
	})

	if err := ctrl.NewControllerManagedBy(mgr).
		For(&klusterletv1alpha1.KlusterletConfig{}).
		WithEventFilter(pred).
		Complete(c); err != nil {
		return fmt.Errorf("failed to add hub HA controller: %w", err)
	}
	return nil
}

func (c *hubHAController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	klusterletConfig := &klusterletv1alpha1.KlusterletConfig{}
	err := c.mgr.GetClient().Get(ctx, request.NamespacedName, klusterletConfig)
	if apierrors.IsNotFound(err) || (err == nil && !klusterletConfig.DeletionTimestamp.IsZero()) {
		if c.resourceSyncer != nil {
			c.resourceSyncer.enabled.Store(false)
			c.periodicSyncer.Unregister(constants.HubHAResourcesMsgKey)
			log.Info("Hub HA active syncer disabled: KlusterletConfig deleted")
		}
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, err
	}

	agentConfig := configs.GetAgentConfig()
	standbyHub := agentConfig.GetStandbyHub()

	if c.resourceSyncer != nil {
		c.resourceSyncer.emitter.SetStandbyHub(standbyHub)
		if !c.resourceSyncer.enabled.Load() {
			c.resourceSyncer.emitter.SetActiveResources(GetHubHAResourcesToSync())
			c.periodicSyncer.Register(&generic.EmitterRegistration{
				Emitter: c.resourceSyncer.emitter,
			})
			c.resourceSyncer.enabled.Store(true)
			log.Info("Hub HA active syncer re-enabled")
		}
		return ctrl.Result{}, nil
	}

	log.Infof("starting Hub HA active syncer: %s (active) -> %s (standby)",
		agentConfig.LeafHubName, standbyHub)

	emitter := NewHubHAEmitter(
		c.producer,
		agentConfig.TransportConfig,
		agentConfig.LeafHubName,
		standbyHub,
		c.mgr.GetClient(),
	)

	allResources := GetHubHAResourcesToSync()
	syncer, err := StartHubHAResourceSyncer(c.mgr, allResources, emitter)
	if err != nil {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, err
	}

	log.Infof("Hub HA active syncer watching %d/%d resource types",
		len(emitter.activeResources), len(allResources))

	c.periodicSyncer.Register(&generic.EmitterRegistration{Emitter: emitter})

	c.resourceSyncer = syncer
	log.Info("Hub HA active syncer started via KlusterletConfig controller")
	return ctrl.Result{}, nil
}
