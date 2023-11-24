package generic

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const REQUEUE_PERIOD = 5 * time.Second

// CreateObjectFunction is a function for how to create an object that is stored inside the bundle.
type CreateObjectFunction func() bundle.Object

type genericStatusSyncer struct {
	log                     logr.Logger
	client                  client.Client
	transport               transport.Producer
	orderedBundleCollection []*BundleEntry
	finalizerName           string
	createBundleObjFunc     func() bundle.Object
	resolveSyncIntervalFunc config.ResolveSyncIntervalFunc
	startOnce               sync.Once
	lock                    sync.Mutex
}

// NewGenericStatusSyncer creates a new instance of genericStatusSyncController and adds it to the manager.
func NewGenericStatusSyncer(mgr ctrl.Manager, logName string, producer transport.Producer,
	orderedBundleCollection []*BundleEntry, createObjFunc CreateObjectFunction, predicate predicate.Predicate,
	resolveSyncIntervalFunc config.ResolveSyncIntervalFunc,
) error {
	statusSyncCtrl := &genericStatusSyncer{
		client:                  mgr.GetClient(),
		log:                     ctrl.Log.WithName(logName),
		transport:               producer,
		orderedBundleCollection: orderedBundleCollection,
		finalizerName:           constants.GlobalHubCleanupFinalizer,
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

func (c *genericStatusSyncer) init() {
	c.startOnce.Do(func() {
		go c.periodicSync()
	})
}

func (c *genericStatusSyncer) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := c.log.WithValues("Namespace", request.Namespace, "Name", request.Name)

	object := c.createBundleObjFunc()
	if err := c.client.Get(ctx, request.NamespacedName, object); apierrors.IsNotFound(err) {
		// the instance was deleted and it had no finalizer on it.
		// for the local resources, there is no finalizer so we need to delete the object from the bundle
		object.SetNamespace(request.Namespace)
		object.SetName(request.Name)
		if e := c.deleteObjectAndFinalizer(ctx, object, reqLogger); e != nil {
			return ctrl.Result{Requeue: true, RequeueAfter: REQUEUE_PERIOD}, e
		}
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

	reqLogger.V(2).Info("Reconciliation complete.")

	return ctrl.Result{}, nil
}

func (c *genericStatusSyncer) isObjectBeingDeleted(object bundle.Object) bool {
	return !object.GetDeletionTimestamp().IsZero()
}

func (c *genericStatusSyncer) updateObjectAndFinalizer(ctx context.Context, object bundle.Object,
	log logr.Logger,
) error {
	// only add finalizer for the global resources
	_, globalLabelResource := object.GetLabels()[constants.GlobalHubGlobalResourceLabel]
	if globalLabelResource || utils.HasAnnotation(object,
		constants.OriginOwnerReferenceAnnotation) {
		if err := c.addFinalizer(ctx, object, log); err != nil {
			return fmt.Errorf("failed to add finalizer - %w", err)
		}
	}

	cleanObject(object)

	c.lock.Lock() // make sure bundles are not updated if we're during bundles sync
	defer c.lock.Unlock()

	for _, entry := range c.orderedBundleCollection {
		// update in each bundle from the collection according to their order.
		entry.Bundle.UpdateObject(object)
	}
	return nil
}

func (c *genericStatusSyncer) deleteObjectAndFinalizer(ctx context.Context, object bundle.Object,
	log logr.Logger,
) error {
	c.lock.Lock() // make sure bundles are not updated if we're during bundles sync

	for _, entry := range c.orderedBundleCollection {
		entry.Bundle.DeleteObject(object) // delete from all bundles.
	}

	c.lock.Unlock() // not using defer since remove finalizer may get delayed. release lock as soon as possible.

	return c.removeFinalizer(ctx, object, log)
}

func (c *genericStatusSyncer) periodicSync() {
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

func (c *genericStatusSyncer) syncBundles() {
	c.lock.Lock() // make sure bundles are not updated if we're during bundles sync
	defer c.lock.Unlock()

	for i := range c.orderedBundleCollection {
		entry := c.orderedBundleCollection[i]

		if !entry.bundlePredicate() { // evaluate if bundle has to be sent only if predicate is true.
			continue
		}

		bundleVersion := entry.Bundle.GetVersion()

		// send to transport only if bundle has changed.
		if bundleVersion.NewerThan(&entry.lastSentBundleVersion) {

			payloadBytes, err := json.Marshal(entry.Bundle)
			if err != nil {
				c.log.Error(err, "marshal entry.Bundle error", "entry.BundleKey", entry.transportBundleKey)
				continue
			}

			messageId := entry.transportBundleKey
			transportMessageKey := entry.transportBundleKey
			if deltaStateBundle, ok := entry.Bundle.(bundle.AgentDeltaBundle); ok {
				transportMessageKey = fmt.Sprintf("%s@%d", entry.transportBundleKey, deltaStateBundle.GetTransportationID())
			}

			if err := c.transport.Send(context.TODO(), &transport.Message{
				Key:     transportMessageKey,
				ID:      messageId,
				MsgType: constants.StatusBundle,
				Version: entry.Bundle.GetVersion().String(),
				Payload: payloadBytes,
			}); err != nil {
				c.log.Error(err, "send transport message error", "id", messageId)
				continue
			}

			// 1. get into the next generation
			// 2. set the lastSentBundleVersion to first version of next generation
			entry.Bundle.GetVersion().Next()
			entry.lastSentBundleVersion = *entry.Bundle.GetVersion()
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

func (c *genericStatusSyncer) addFinalizer(ctx context.Context, object bundle.Object, log logr.Logger) error {
	// if the removing finalizer label hasn't expired, then skip the adding finalizer action
	if val, found := object.GetLabels()[constants.GlobalHubFinalizerRemovingDeadline]; found {
		deadline, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return err
		}
		if time.Now().Unix() < deadline {
			return nil
		} else {
			delete(object.GetLabels(), constants.GlobalHubFinalizerRemovingDeadline)
		}
	}

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

func (c *genericStatusSyncer) removeFinalizer(ctx context.Context, object bundle.Object,
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
