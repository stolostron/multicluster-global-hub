package generic

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/emitters"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

type syncController struct {
	client        client.Client
	emitters      []emitters.Emitter
	instance      func() client.Object
	leafHubName   string
	finalizerName string
}

// AddSyncCtrl registers a controller that watch the specific client.Object,
// and load the object into the bundle in the provided emitters.
func AddSyncCtrl(mgr ctrl.Manager, ctrlName string, instanceFunc func() client.Object,
	objectEmitters ...emitters.Emitter,
) error {
	if len(objectEmitters) == 0 {
		return fmt.Errorf("at least one emitter is required")
	}

	syncer := &syncController{
		client:        mgr.GetClient(),
		emitters:      objectEmitters,
		instance:      instanceFunc,
		leafHubName:   configs.GetLeafHubName(),
		finalizerName: constants.GlobalHubCleanupFinalizer,
	}

	// Create combined event filter using OR relationship for all methods
	combinedFilter := createCombinedFilter(objectEmitters...)

	return ctrl.NewControllerManagedBy(mgr).For(instanceFunc()).WithEventFilter(combinedFilter).
		Named(ctrlName).Complete(syncer)
}

// createCombinedFilter creates a combined filter using OR relationship for all event types
func createCombinedFilter(emitters ...emitters.Emitter) predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			for _, emitter := range emitters {
				if emitter.Predicate().Create(e) {
					return true
				}
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			for _, emitter := range emitters {
				if emitter.Predicate().Update(e) {
					return true
				}
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			for _, emitter := range emitters {
				if emitter.Predicate().Delete(e) {
					return true
				}
			}
			return false
		},
		GenericFunc: func(e event.GenericEvent) bool {
			for _, emitter := range emitters {
				if emitter.Predicate().Generic(e) {
					return true
				}
			}
			return false
		},
	}
}

func (c *syncController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	object := c.instance()
	if err := c.client.Get(ctx, request.NamespacedName, object); errors.IsNotFound(err) {
		// the instance was deleted and it had no finalizer on it.
		// for the local resources, there is no finalizer so we need to add the delete event in the bundle
		object.SetNamespace(request.Namespace)
		object.SetName(request.Name)
		log.Infof("Object %s/%s not found, deleting from bundle\n", request.Namespace, request.Name)
		for _, emitter := range c.emitters {
			if err = emitter.Delete(object); err != nil {
				log.Errorw("failed to add deleted object into bundle", "error", err, "name", request.Name)
				return ctrl.Result{Requeue: true, RequeueAfter: 5 * time.Second}, err
			}
		}
		return ctrl.Result{}, nil
	} else if err != nil {
		log.Errorw("failed to get the object", "error", err, "namespace", request.Namespace, "name", request.Name)
		return ctrl.Result{Requeue: true, RequeueAfter: 5 * time.Second}, err
	}

	// delete
	if !object.GetDeletionTimestamp().IsZero() {
		if IsGlobalResource(object) && controllerutil.RemoveFinalizer(object, c.finalizerName) {
			if err := c.client.Update(ctx, object); err != nil {
				log.Errorw("failed to remove finalizer", "error", err, "name", request.Name)
				return ctrl.Result{Requeue: true, RequeueAfter: 5 * time.Second}, err
			}
		}

		for _, emitter := range c.emitters {
			if err := emitter.Delete(object); err != nil {
				log.Errorw("failed to add deleted object into bundle", "error", err, "name", request.Name)
				return ctrl.Result{Requeue: true, RequeueAfter: 5 * time.Second}, err
			}
		}

		return ctrl.Result{}, nil
	}

	if IsGlobalResource(object) && controllerutil.AddFinalizer(object, c.finalizerName) {
		if err := c.client.Update(ctx, object); err != nil {
			log.Error(err, "failed to add fianlizer", "namespace", request.Namespace, "name", request.Name)
			return ctrl.Result{Requeue: true, RequeueAfter: 5 * time.Second}, err
		}
	}

	// update/insert
	cleanObject(object)
	for _, emitter := range c.emitters {
		if err := emitter.Update(object); err != nil {
			log.Errorw("failed to add updated object into bundle", "error", err, "name", request.Name)
			return ctrl.Result{Requeue: true, RequeueAfter: 5 * time.Second}, err
		}
	}

	return ctrl.Result{}, nil
}

func IsGlobalResource(obj client.Object) bool {
	return utils.HasLabel(obj, constants.GlobalHubGlobalResourceLabel) ||
		utils.HasAnnotation(obj, constants.OriginOwnerReferenceAnnotation)
}
