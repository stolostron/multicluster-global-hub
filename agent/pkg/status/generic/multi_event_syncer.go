package generic

import (
	"context"
	"fmt"
	"sync"
	"time"

	cecontext "github.com/cloudevents/sdk-go/v2/context"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/interfaces"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

const (
	ExtVersion = "extversion"
)

type multiEventSyncer struct {
	log              *zap.SugaredLogger
	client           client.Client
	producer         transport.Producer
	controller       interfaces.Controller
	eventEmitters    []*EmitterHandler
	leafHubName      string
	syncIntervalFunc func() time.Duration
	finalizerName    string
	startOnce        sync.Once
	lock             sync.Mutex
}

type EmitterHandler struct {
	interfaces.Emitter
	interfaces.Handler
}

// LaunchMultiEventSyncer is used to send multi event(by the eventEmitter) by a specific client.Object
func LaunchMultiEventSyncer(name string, mgr ctrl.Manager, controller interfaces.Controller,
	producer transport.Producer, intervalFunc func() time.Duration, eventEmitters []*EmitterHandler,
) error {
	syncer := &multiEventSyncer{
		log:              logger.ZapLogger(name),
		client:           mgr.GetClient(),
		producer:         producer,
		syncIntervalFunc: intervalFunc,
		controller:       controller,
		eventEmitters:    eventEmitters,
		leafHubName:      configs.GetLeafHubName(),
		finalizerName:    constants.GlobalHubCleanupFinalizer,
	}

	// start the periodic syncer
	syncer.startOnce.Do(func() {
		go syncer.periodicSync()
	})

	return ctrl.NewControllerManagedBy(mgr).For(controller.Instance()).
		WithEventFilter(controller.Predicate()).Complete(syncer)
}

func (c *multiEventSyncer) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
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
		return ctrl.Result{}, nil
	}

	// update/insert
	cleanObject(object)
	c.updateObject(object)
	return ctrl.Result{}, nil
}

func (c *multiEventSyncer) updateObject(object client.Object) {
	c.lock.Lock() // make sure handler are not updated if we're during bundles sync
	defer c.lock.Unlock()
	for _, eventEmitter := range c.eventEmitters {
		// update in each handler from the collection according to their order.
		if eventEmitter.Update(object) {
			eventEmitter.PostUpdate()
		}
	}
}

func (c *multiEventSyncer) deleteObject(object client.Object) {
	c.lock.Lock() // make sure bundles are not updated if we're during bundles sync
	defer c.lock.Unlock()

	for _, eventEmitter := range c.eventEmitters {
		if eventEmitter.Delete(object) {
			eventEmitter.PostUpdate()
		}
	}
}

func (c *multiEventSyncer) periodicSync() {
	currentSyncInterval := c.syncIntervalFunc()
	ticker := time.NewTicker(currentSyncInterval)
	defer ticker.Stop()

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

func (c *multiEventSyncer) syncEvents() {
	c.lock.Lock() // make sure bundles are not updated if we're during bundles sync
	defer c.lock.Unlock()

	for i := range c.eventEmitters {
		emitter := c.eventEmitters[i]

		if emitter.ShouldSend() {
			evt, err := emitter.ToCloudEvent(emitter.Get())
			if err != nil {
				c.log.Error(err, "failed to get CloudEvent instance", "evt", evt)
			}
			evt.SetSource(c.leafHubName)

			ctx := context.TODO()
			if emitter.Topic() != "" {
				ctx = cecontext.WithTopic(ctx, emitter.Topic())
			}
			if err := c.producer.SendEvent(ctx, *evt); err != nil {
				c.log.Error(err, "failed to send event", "evt", evt)
				continue
			}
			emitter.PostSend(emitter.Get())
		}
	}
}
