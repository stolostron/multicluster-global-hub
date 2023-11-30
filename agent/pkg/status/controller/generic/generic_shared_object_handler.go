package generic

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// the object handler is for the shared bundle syncer
type objectHandler struct {
	log           logr.Logger
	client        client.Client
	bundleEntry   *SharedBundleEntry
	handler       bundle.SharedBundleObject
	producer      transport.Producer
	finalizerName string
	lock          *sync.Mutex
}

func newObjectHandler(mgr ctrl.Manager, producer transport.Producer, bundleEntry *SharedBundleEntry,
	handler bundle.SharedBundleObject, lock *sync.Mutex,
) error {
	obj := handler.CreateObject()
	logName := fmt.Sprintf("%s.%s", bundleEntry.transportBundleKey, obj.GetObjectKind())
	statusSyncCtrl := &objectHandler{
		log:           ctrl.Log.WithName(logName),
		bundleEntry:   bundleEntry,
		client:        mgr.GetClient(),
		producer:      producer,
		handler:       handler,
		finalizerName: constants.GlobalHubCleanupFinalizer,
		lock:          lock,
	}

	controllerBuilder := ctrl.NewControllerManagedBy(mgr).For(obj)
	if handler.Predicate() != nil {
		controllerBuilder = controllerBuilder.WithEventFilter(handler.Predicate())
	}
	return controllerBuilder.Complete(statusSyncCtrl)
}

func (c *objectHandler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
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

func (c *objectHandler) updateObjectAndFinalizer(ctx context.Context, object bundle.Object) error {
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

func (c *objectHandler) deleteObjectAndFinalizer(ctx context.Context, object bundle.Object) error {
	c.lock.Lock() // make sure bundles are not updated if we're during bundles sync
	c.handler.BundleDelete(object, c.bundleEntry.bundle)
	c.lock.Unlock()

	return removeFinalizer(ctx, c.client, object, c.finalizerName)
}
