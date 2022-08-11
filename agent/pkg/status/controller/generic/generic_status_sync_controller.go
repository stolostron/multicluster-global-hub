package generic

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-globalhub/agent/pkg/status/bundle"
	"github.com/stolostron/multicluster-globalhub/agent/pkg/status/controller/syncintervals"
	producer "github.com/stolostron/multicluster-globalhub/agent/pkg/transport/producer"
	"github.com/stolostron/multicluster-globalhub/pkg/constants"
)

const REQUEUE_PERIOD = 5 * time.Second

// CreateObjectFunction is a function for how to create an object that is stored inside the bundle.
type CreateObjectFunction func() bundle.Object

const (
	hubOfHubsCleanupFinalizer = "hub-of-hubs.open-cluster-management.io/resource-cleanup"
)

type genericStatusSyncController struct {
	client                  client.Client
	log                     logr.Logger
	transport               producer.Producer
	orderedBundleCollection []*BundleCollectionEntry
	finalizerName           string
	createBundleObjFunc     func() bundle.Object
	resolveSyncIntervalFunc syncintervals.ResolveSyncIntervalFunc
	startOnce               sync.Once
	lock                    sync.Mutex
}

// NewGenericStatusSyncController creates a new instance of genericStatusSyncController and adds it to the manager.
func NewGenericStatusSyncController(mgr ctrl.Manager, logName string, producer producer.Producer,
	orderedBundleCollection []*BundleCollectionEntry, createObjFunc CreateObjectFunction, predicate predicate.Predicate,
	resolveSyncIntervalFunc syncintervals.ResolveSyncIntervalFunc,
) error {
	statusSyncCtrl := &genericStatusSyncController{
		client:                  mgr.GetClient(),
		log:                     ctrl.Log.WithName(logName),
		transport:               producer,
		orderedBundleCollection: orderedBundleCollection,
		finalizerName:           hubOfHubsCleanupFinalizer,
		createBundleObjFunc:     createObjFunc,
		resolveSyncIntervalFunc: resolveSyncIntervalFunc,
		lock:                    sync.Mutex{},
	}
	statusSyncCtrl.init()

	controllerBuilder := ctrl.NewControllerManagedBy(mgr).For(createObjFunc())
	if predicate != nil {
		controllerBuilder = controllerBuilder.WithEventFilter(predicate)
	}

	if err := controllerBuilder.Complete(statusSyncCtrl); err != nil {
		return fmt.Errorf("failed to add controller to the manager - %w", err)
	}

	return nil
}

func (c *genericStatusSyncController) init() {
	c.startOnce.Do(func() {
		go c.periodicSync()
	})
}

func (c *genericStatusSyncController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := c.log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)

	object := c.createBundleObjFunc()

	if err := c.client.Get(ctx, request.NamespacedName, object); apierrors.IsNotFound(err) {
		// the instance was deleted and it had no finalizer on it.
		// this means either LH removed the finalizer so it was already deleted from bundle, or
		// LH didn't update HoH about this object ever.
		// either way, no need to do anything in this state.
		return ctrl.Result{}, nil
	} else if err != nil {
		return ctrl.Result{Requeue: true, RequeueAfter: REQUEUE_PERIOD},
			fmt.Errorf("reconciliation failed: %w", err)
	}

	if c.isObjectBeingDeleted(object) {
		if err := c.deleteObjectAndFinalizer(ctx, object, reqLogger); err != nil {
			return ctrl.Result{Requeue: true, RequeueAfter: REQUEUE_PERIOD}, err
		}
	} else { // otherwise, the object was not deleted and no error occurred
		if err := c.updateObjectAndFinalizer(ctx, object, reqLogger); err != nil {
			return ctrl.Result{Requeue: true, RequeueAfter: REQUEUE_PERIOD}, err
		}
	}

	reqLogger.Info("Reconciliation complete.")

	return ctrl.Result{}, nil
}

func (c *genericStatusSyncController) isObjectBeingDeleted(object bundle.Object) bool {
	return !object.GetDeletionTimestamp().IsZero()
}

func (c *genericStatusSyncController) updateObjectAndFinalizer(ctx context.Context, object bundle.Object,
	log logr.Logger,
) error {
	if err := c.addFinalizer(ctx, object, log); err != nil {
		return fmt.Errorf("failed to add finalizer - %w", err)
	}

	cleanObject(object)

	c.lock.Lock() // make sure bundles are not updated if we're during bundles sync
	defer c.lock.Unlock()

	for _, entry := range c.orderedBundleCollection {
		entry.bundle.UpdateObject(object) // update in each bundle from the collection according to their order.
	}

	return nil
}

func (c *genericStatusSyncController) addFinalizer(ctx context.Context, object bundle.Object, log logr.Logger) error {
	if controllerutil.ContainsFinalizer(object, c.finalizerName) {
		return nil
	}

	log.Info("adding finalizer")
	controllerutil.AddFinalizer(object, c.finalizerName)

	if err := c.client.Update(ctx, object); err != nil &&
		!strings.Contains(err.Error(), "the object has been modified") {
		return fmt.Errorf("failed to add finalizer %s - %w", c.finalizerName, err)
	}

	return nil
}

func (c *genericStatusSyncController) deleteObjectAndFinalizer(ctx context.Context, object bundle.Object,
	log logr.Logger,
) error {
	c.lock.Lock() // make sure bundles are not updated if we're during bundles sync

	for _, entry := range c.orderedBundleCollection {
		entry.bundle.DeleteObject(object) // delete from all bundles.
	}

	c.lock.Unlock() // not using defer since remove finalizer may get delayed. release lock as soon as possible.

	return c.removeFinalizer(ctx, object, log)
}

func (c *genericStatusSyncController) removeFinalizer(ctx context.Context, object bundle.Object,
	log logr.Logger,
) error {
	if !controllerutil.ContainsFinalizer(object, c.finalizerName) {
		return nil // if finalizer is not there, do nothing.
	}

	log.Info("removing finalizer")
	controllerutil.RemoveFinalizer(object, c.finalizerName)

	if err := c.client.Update(ctx, object); err != nil {
		return fmt.Errorf("failed to remove finalizer %s - %w", c.finalizerName, err)
	}

	return nil
}

func (c *genericStatusSyncController) periodicSync() {
	currentSyncInterval := c.resolveSyncIntervalFunc()
	ticker := time.NewTicker(currentSyncInterval)

	for {
		<-ticker.C // wait for next time interval
		c.syncBundles()

		resolvedInterval := c.resolveSyncIntervalFunc()

		// reset ticker if sync interval has changed
		if resolvedInterval != currentSyncInterval {
			currentSyncInterval = resolvedInterval
			ticker.Reset(currentSyncInterval)
			c.log.Info(fmt.Sprintf("sync interval has been reset to %s", currentSyncInterval.String()))
		}
	}
}

func (c *genericStatusSyncController) syncBundles() {
	c.lock.Lock() // make sure bundles are not updated if we're during bundles sync
	defer c.lock.Unlock()

	for _, entry := range c.orderedBundleCollection {
		if !entry.predicate() { // evaluate if bundle has to be sent only if predicate is true.
			continue
		}

		bundleVersion := entry.bundle.GetBundleVersion()

		// send to transport only if bundle has changed.
		if bundleVersion.NewerThan(&entry.lastSentBundleVersion) {

			payloadBytes, err := json.Marshal(entry.bundle)
			if err != nil {
				c.log.Error(
					fmt.Errorf("sync object from type %s with id %s - %w",
						constants.StatusBundle, entry.transportBundleKey, err),
					"failed to sync bundle")
			}

			transportMessageKey := entry.transportBundleKey
			if deltaStateBundle, ok := entry.bundle.(bundle.DeltaStateBundle); ok {
				transportMessageKey = fmt.Sprintf("%s@%d", entry.transportBundleKey, deltaStateBundle.GetTransportationID())
			}

			c.transport.SendAsync(&producer.Message{
				Key:     transportMessageKey,
				ID:      entry.transportBundleKey,
				MsgType: constants.StatusBundle,
				Version: entry.bundle.GetBundleVersion().String(),
				Payload: payloadBytes,
			})

			entry.lastSentBundleVersion = *bundleVersion
		}
	}
}

func cleanObject(object bundle.Object) {
	object.SetManagedFields(nil)
	object.SetFinalizers(nil)
	object.SetGeneration(0)
	object.SetOwnerReferences(nil)
	object.SetSelfLink("")
	// object.SetClusterName("")
}
