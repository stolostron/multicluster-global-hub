package generic

import (
	"context"
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

	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const REQUEUE_PERIOD = 5 * time.Second

type genericEventController struct {
	log             logr.Logger
	client          client.Client
	emitter         Emitter
	eventController EventController
	lock            *sync.Mutex
}

func AddEventController(mgr ctrl.Manager, eventController EventController, emitter Emitter,
	lock *sync.Mutex,
) error {
	obj := eventController.Instance()
	statusSyncCtrl := &genericEventController{
		log:             ctrl.Log.WithName(fmt.Sprintf("status.%s", obj.GetObjectKind())),
		client:          mgr.GetClient(),
		emitter:         emitter,
		eventController: eventController,
		lock:            lock,
	}

	controllerBuilder := ctrl.NewControllerManagedBy(mgr).For(obj)
	if eventController.Predicate() != nil {
		controllerBuilder = controllerBuilder.WithEventFilter(eventController.Predicate())
	}
	return controllerBuilder.Complete(statusSyncCtrl)
}

func (c *genericEventController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := c.log.WithValues("Namespace", request.Namespace, "Name", request.Name)
	object := c.eventController.Instance()

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

func (c *genericEventController) updateObjectAndFinalizer(ctx context.Context, object client.Object) error {
	// only add finalizer for the global resources
	if enableCleanUpFinalizer(object) {
		err := addFinalizer(ctx, c.client, object, FinalizerName)
		if err != nil {
			return err
		}
	}

	c.lock.Lock() // make sure bundles are not updated if we're during bundles sync
	defer c.lock.Unlock()
	if c.emitter.ShouldUpdate(object) && c.eventController.Update(object) {
		c.emitter.PostUpdate()
	}
	return nil
}

func (c *genericEventController) deleteObjectAndFinalizer(ctx context.Context, object bundle.Object) error {
	c.lock.Lock() // make sure bundles are not updated if we're during bundles sync
	if c.emitter.ShouldUpdate(object) && c.eventController.Delete(object) {
		c.emitter.PostUpdate()
	}
	c.lock.Unlock()

	if enableCleanUpFinalizer(object) {
		err := removeFinalizer(ctx, c.client, object, FinalizerName)
		if err != nil {
			return err
		}
	}
	return nil
}

func enableCleanUpFinalizer(obj client.Object) bool {
	return utils.HasLabel(obj, constants.GlobalHubGlobalResourceLabel) ||
		utils.HasAnnotation(obj, constants.OriginOwnerReferenceAnnotation)
}

func cleanObject(object bundle.Object) {
	object.SetManagedFields(nil)
	object.SetFinalizers(nil)
	object.SetGeneration(0)
	object.SetOwnerReferences(nil)
	object.SetSelfLink("")
	// object.SetClusterName("")
}

func addFinalizer(ctx context.Context, c client.Client, obj bundle.Object, finalizer string) error {
	// if the removing finalizer label hasn't expired, then skip the adding finalizer action
	if val, found := obj.GetLabels()[constants.GlobalHubFinalizerRemovingDeadline]; found {
		deadline, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return err
		}
		if time.Now().Unix() < deadline {
			return nil
		} else {
			delete(obj.GetLabels(), constants.GlobalHubFinalizerRemovingDeadline)
		}
	}

	if controllerutil.ContainsFinalizer(obj, finalizer) {
		return nil
	}

	controllerutil.AddFinalizer(obj, finalizer)

	if err := c.Update(ctx, obj); err != nil && !strings.Contains(err.Error(), "the object has been modified") {
		return err
	}
	return nil
}

func removeFinalizer(ctx context.Context, c client.Client, obj bundle.Object, finalizer string) error {
	if !controllerutil.ContainsFinalizer(obj, finalizer) {
		return nil // if finalizer is not there, do nothing.
	}
	controllerutil.RemoveFinalizer(obj, finalizer)

	return c.Update(ctx, obj)
}
