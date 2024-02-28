package generic

import (
	"context"
	"fmt"
	"sync"
	"time"

	cecontext "github.com/cloudevents/sdk-go/v2/context"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/kafka_confluent"
)

const (
	ExtVersion    = "extversion"
	FinalizerName = constants.GlobalHubCleanupFinalizer
)

type genericObjectSyncer struct {
	log              logr.Logger
	client           client.Client
	producer         transport.Producer
	controller       Controller
	eventEmitters    []ObjectEmitter
	leafHubName      string
	syncIntervalFunc func() time.Duration
	startOnce        sync.Once
	lock             sync.Mutex
}

// LaunchGenericObjectSyncer is used to send multi event(by the eventEmitter) by a specific client.Object
func LaunchGenericObjectSyncer(name string, mgr ctrl.Manager, controller Controller,
	producer transport.Producer, intervalFunc func() time.Duration, eventEmitters []ObjectEmitter,
) error {
	syncer := &genericObjectSyncer{
		log:              ctrl.Log.WithName(name),
		client:           mgr.GetClient(),
		producer:         producer,
		syncIntervalFunc: intervalFunc,
		controller:       controller,
		eventEmitters:    eventEmitters,
		leafHubName:      config.GetLeafHubName(),
	}

	// start the periodic syncer
	syncer.startOnce.Do(func() {
		go syncer.periodicSync()
	})

	return ctrl.NewControllerManagedBy(mgr).For(controller.Instance()).
		WithEventFilter(controller.Predicate()).Complete(syncer)
}

func (c *genericObjectSyncer) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	object := c.controller.Instance()
	if err := c.client.Get(ctx, request.NamespacedName, object); errors.IsNotFound(err) {
		// the instance was deleted and it had no finalizer on it.
		// for the local resources, there is no finalizer so we need to delete the object from the entry handler
		object.SetNamespace(request.Namespace)
		object.SetName(request.Name)
		c.deleteObject(object)
		return ctrl.Result{}, nil
	} else if err != nil {
		c.log.Error(err, "failed to get the object", "namespace", request.Namespace, "name", request.Name)
		return ctrl.Result{Requeue: true, RequeueAfter: REQUEUE_PERIOD}, err
	}

	// delete
	if !object.GetDeletionTimestamp().IsZero() {
		c.deleteObject(object)
		if !enableCleanUpFinalizer(object) {
			return ctrl.Result{}, nil
		}
		err := removeFinalizer(ctx, c.client, object, FinalizerName)
		if err != nil {
			c.log.Error(err, "failed to remove cleanup fianlizer", "namespace", request.Namespace, "name", request.Name)
			return ctrl.Result{Requeue: true, RequeueAfter: REQUEUE_PERIOD}, err
		}
		return ctrl.Result{}, nil
	}

	// update/insert
	cleanObject(object)
	c.updateObject(object)
	if !enableCleanUpFinalizer(object) {
		return ctrl.Result{}, nil
	}
	err := addFinalizer(ctx, c.client, object, FinalizerName)
	if err != nil {
		c.log.Error(err, "failed to add finalizer", "namespace", request.Namespace, "name", request.Name)
		return ctrl.Result{Requeue: true, RequeueAfter: REQUEUE_PERIOD}, err
	}
	return ctrl.Result{}, nil
}

func (c *genericObjectSyncer) updateObject(object client.Object) {
	c.lock.Lock() // make sure handler are not updated if we're during bundles sync
	defer c.lock.Unlock()
	for _, eventEmitter := range c.eventEmitters {
		// update in each handler from the collection according to their order.
		if eventEmitter.ShouldUpdate(object) && eventEmitter.Update(object) {
			eventEmitter.PostUpdate()
		}
	}
}

func (c *genericObjectSyncer) deleteObject(object client.Object) {
	c.lock.Lock() // make sure bundles are not updated if we're during bundles sync
	defer c.lock.Lock()
	for _, eventEmitter := range c.eventEmitters {
		if eventEmitter.ShouldUpdate(object) && eventEmitter.Delete(object) {
			eventEmitter.PostUpdate()
		}
	}
}

func (c *genericObjectSyncer) periodicSync() {
	currentSyncInterval := c.syncIntervalFunc()
	ticker := time.NewTicker(currentSyncInterval)

	for {
		<-ticker.C // wait for next time interval
		c.syncEvents()

		resolvedInterval := c.syncIntervalFunc()

		// reset ticker if sync interval has changed
		if resolvedInterval != currentSyncInterval {
			currentSyncInterval = resolvedInterval
			ticker.Reset(currentSyncInterval)
			c.log.Info(fmt.Sprintf("sync interval has been reset to %s", currentSyncInterval.String()))
		}
	}
}

func (c *genericObjectSyncer) syncEvents() {
	c.lock.Lock() // make sure bundles are not updated if we're during bundles sync
	defer c.lock.Unlock()

	for i := range c.eventEmitters {
		emitter := c.eventEmitters[i]

		if emitter.ShouldSend() {
			evt, err := emitter.ToCloudEvent()
			if err != nil {
				c.log.Error(err, "failed to get CloudEvent instance", "evt", evt)
			}
			evt.SetSource(c.leafHubName)

			ctx := context.TODO()
			if emitter.Topic() != "" {
				ctx = cecontext.WithTopic(ctx, emitter.Topic())
			}
			evtCtx := kafka_confluent.WithMessageKey(ctx, c.leafHubName)
			if err := c.producer.SendEvent(evtCtx, *evt); err != nil {
				c.log.Error(err, "failed to send event", "evt", evt)
				continue
			}
			emitter.PostSend()
		}
	}
}
