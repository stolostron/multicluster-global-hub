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
	objectController ObjectController
	eventEmitters    []MultiEventEmitter
	leafHubName      string
	syncIntervalFunc func() time.Duration
	startOnce        sync.Once
	lock             sync.Mutex
}

// LaunchGenericObjectSyncer is used to send multi event(by the eventEmitter) by a specific client.Object
func LaunchGenericObjectSyncer(name string, mgr ctrl.Manager, objectController ObjectController,
	producer transport.Producer, intervalFunc func() time.Duration, eventEmitters []MultiEventEmitter,
) error {
	syncer := &genericObjectSyncer{
		log:              ctrl.Log.WithName(name),
		client:           mgr.GetClient(),
		producer:         producer,
		objectController: objectController,
		eventEmitters:    eventEmitters,
		leafHubName:      config.GetLeafHubName(),
	}

	// start the periodic syncer
	syncer.startOnce.Do(func() {
		go syncer.periodicSync()
	})

	return ctrl.NewControllerManagedBy(mgr).For(objectController.Instance()).
		WithEventFilter(objectController.Predicate()).Complete(syncer)
}

func (c *genericObjectSyncer) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	object := c.objectController.Instance()
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
		if err := removeFinalizer(ctx, c.client, object, FinalizerName); err != nil {
			c.log.Error(err, "failed to remove finalizer from object", "namespace", request.Namespace, "name", request.Name)
			return ctrl.Result{Requeue: true, RequeueAfter: REQUEUE_PERIOD}, err
		}
		return ctrl.Result{}, nil
	}

	// update/insert
	if c.objectController.EnableFinalizer() {
		if err := addFinalizer(ctx, c.client, object, FinalizerName); err != nil {
			c.log.Error(err, "failed to add finalizer to object", "namespace", request.Namespace, "name", request.Name)
			return ctrl.Result{Requeue: true, RequeueAfter: REQUEUE_PERIOD}, err
		}
	}
	cleanObject(object)
	c.updateObject(object)

	return ctrl.Result{}, nil
}

func (c *genericObjectSyncer) updateObject(object client.Object) {
	c.lock.Lock() // make sure handler are not updated if we're during bundles sync
	defer c.lock.Unlock()
	for _, emitter := range c.eventEmitters {
		// update in each handler from the collection according to their order.
		if emitter.Predicate(object) {
			emitter.Update(object)
		}
	}
}

func (c *genericObjectSyncer) deleteObject(object client.Object) {
	c.lock.Lock() // make sure bundles are not updated if we're during bundles sync
	for _, emitter := range c.eventEmitters {
		if emitter.Predicate(object) {
			emitter.Delete(object)
		}
	}
	c.lock.Unlock() // not using defer since remove finalizer may get delayed. release lock as soon as possible.
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

		if emitter.PreSend() {
			evt := emitter.ToCloudEvent()
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
