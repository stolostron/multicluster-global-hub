package generic

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// NewMultiHandlerStatusSyncer creates a new instance of genericStatusSyncController and adds it to the manager.
func NewMultiHandlerStatusSyncer(mgr ctrl.Manager, producer transport.Producer, bundleEntry *HandlerBundleEntry,
	objectHandlers []bundle.ObjectHandler,
) error {
	var lock sync.Mutex
	for _, handler := range objectHandlers {
		err := newHandlerStatusSyncer(mgr, producer, bundleEntry, handler, &lock)
		if err != nil {
			return err
		}
	}
	return nil
}

type handlerStatusSyncer struct {
	log         logr.Logger
	client      client.Client
	producer    transport.Producer
	bundleEntry *HandlerBundleEntry
	handler     bundle.ObjectHandler

	finalizerName string
	startOnce     sync.Once
	lock          *sync.Mutex
}

func newHandlerStatusSyncer(mgr ctrl.Manager, producer transport.Producer, bundleEntry *HandlerBundleEntry,
	handler bundle.ObjectHandler, lock *sync.Mutex,
) error {
	obj := handler.CreateObject()
	logName := fmt.Sprintf("%s.%s", bundleEntry.transportBundleKey, obj.GetObjectKind())
	statusSyncCtrl := &handlerStatusSyncer{
		log:           ctrl.Log.WithName(logName),
		client:        mgr.GetClient(),
		producer:      producer,
		bundleEntry:   bundleEntry,
		handler:       handler,
		finalizerName: constants.GlobalHubCleanupFinalizer,
		lock:          lock,
	}
	statusSyncCtrl.init()

	controllerBuilder := ctrl.NewControllerManagedBy(mgr).For(obj)
	if handler.Predicate() != nil {
		controllerBuilder = controllerBuilder.WithEventFilter(handler.Predicate())
	}
	return controllerBuilder.Complete(statusSyncCtrl)
}

func (c *handlerStatusSyncer) init() {
	c.startOnce.Do(func() {
		go c.periodicSync()
	})
}

func (c *handlerStatusSyncer) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := c.log.WithValues("Namespace", request.Namespace, "Name", request.Name)
	object := c.handler.CreateObject()
	if err := c.client.Get(ctx, request.NamespacedName, object); apierrors.IsNotFound(err) {
		// the instance was deleted and it had no finalizer on it.
		// for the local resources, there is no finalizer so we need to delete the object from the bundle
		object.SetNamespace(request.Namespace)
		object.SetName(request.Name)
		if e := c.deleteObjectAndFinalizer(ctx, object); e != nil {
			return ctrl.Result{Requeue: true, RequeueAfter: REQUEUE_PERIOD}, e
		}
		return ctrl.Result{}, nil
	} else if err != nil {
		return ctrl.Result{Requeue: true, RequeueAfter: REQUEUE_PERIOD},
			fmt.Errorf("reconciliation failed: %w", err)
	}

	if !object.GetDeletionTimestamp().IsZero() {
		if err := c.deleteObjectAndFinalizer(ctx, object); err != nil {
			return ctrl.Result{Requeue: true, RequeueAfter: REQUEUE_PERIOD}, err
		}
	} else { // otherwise, the object was not deleted and no error occurred
		if err := c.updateObjectAndFinalizer(ctx, object); err != nil {
			return ctrl.Result{Requeue: true, RequeueAfter: REQUEUE_PERIOD}, err
		}
	}
	reqLogger.V(2).Info("Reconciliation complete.")
	return ctrl.Result{}, nil
}

func (c *handlerStatusSyncer) updateObjectAndFinalizer(ctx context.Context, object bundle.Object) error {
	// only add finalizer for the global resources
	_, globalLabelResource := object.GetLabels()[constants.GlobalHubGlobalResourceLabel]
	if globalLabelResource || utils.HasAnnotation(object,
		constants.OriginOwnerReferenceAnnotation) {
		if err := addFinalizer(ctx, c.client, object, c.finalizerName); err != nil {
			return fmt.Errorf("failed to add finalizer - %w", err)
		}
	}

	cleanObject(object)

	c.lock.Lock() // make sure bundles are not updated if we're during bundles sync
	defer c.lock.Unlock()
	c.handler.BundleUpdate(object, c.bundleEntry.bundle)
	return nil
}

func (c *handlerStatusSyncer) deleteObjectAndFinalizer(ctx context.Context, object bundle.Object) error {
	c.lock.Lock() // make sure bundles are not updated if we're during bundles sync
	c.handler.BundleDelete(object, c.bundleEntry.bundle)
	c.lock.Unlock()

	return removeFinalizer(ctx, c.client, object, c.finalizerName)
}

func (c *handlerStatusSyncer) periodicSync() {
	currentSyncInterval := c.handler.SyncInterval()
	ticker := time.NewTicker(currentSyncInterval)

	for {
		<-ticker.C // wait for next time interval
		c.syncBundles()

		resolvedInterval := c.handler.SyncInterval()

		// reset ticker if sync interval has changed
		if resolvedInterval != currentSyncInterval {
			currentSyncInterval = resolvedInterval
			ticker.Reset(currentSyncInterval)
			c.log.Info(fmt.Sprintf("sync interval has been reset to %s", currentSyncInterval.String()))
		}
	}
}

func (c *handlerStatusSyncer) syncBundles() {
	c.lock.Lock() // make sure bundles are not updated if we're during bundles sync
	defer c.lock.Unlock()

	entry := c.bundleEntry
	// evaluate if bundle has to be sent only if predicate is true.
	if !entry.bundlePredicate() {
		return
	}
	bundleVersion := entry.bundle.GetVersion()

	// send to transport only if bundle has changed.
	if bundleVersion.NewerThan(&entry.lastSentBundleVersion) {
		payloadBytes, err := json.Marshal(entry.bundle)
		if err != nil {
			c.log.Error(err, "marshal entry.bundle error", "entry.bundleKey", entry.transportBundleKey)
			return
		}

		messageId := entry.transportBundleKey
		transportMessageKey := entry.transportBundleKey
		if deltaStateBundle, ok := entry.bundle.(bundle.AgentDeltaBundle); ok {
			transportMessageKey = fmt.Sprintf("%s@%d", entry.transportBundleKey, deltaStateBundle.GetTransportationID())
		}

		if err := c.producer.Send(context.TODO(), &transport.Message{
			Key:     transportMessageKey,
			ID:      messageId,
			MsgType: constants.StatusBundle,
			Version: entry.bundle.GetVersion().String(),
			Payload: payloadBytes,
		}); err != nil {
			c.log.Error(err, "send transport message error", "id", messageId)
			return
		}

		// 1. get into the next generation
		// 2. set the lastSentBundleVersion to first version of next generation
		entry.bundle.GetVersion().Next()
		entry.lastSentBundleVersion = *entry.bundle.GetVersion()
	}
}
