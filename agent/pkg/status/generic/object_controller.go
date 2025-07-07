package generic

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/emitters"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

type syncController struct {
	client        client.Client
	emitter       emitters.Emitter
	instance      func() client.Object
	leafHubName   string
	finalizerName string
}

// AddSyncCtrl registers a controller that watch the specific client.Object,
// and load the object into the bundle in the provided emitter.
func AddSyncCtrl(mgr ctrl.Manager, instanceFunc func() client.Object, objectEmitter emitters.Emitter) error {
	controllerName := enum.ShortenEventType(objectEmitter.EventType())
	syncer := &syncController{
		client:        mgr.GetClient(),
		emitter:       objectEmitter,
		instance:      instanceFunc,
		leafHubName:   configs.GetLeafHubName(),
		finalizerName: constants.GlobalHubCleanupFinalizer,
	}

	return ctrl.NewControllerManagedBy(mgr).For(instanceFunc()).WithEventFilter(objectEmitter.EventFilter()).
		Named(controllerName).Complete(syncer)
}

func (c *syncController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	object := c.instance()
	if err := c.client.Get(ctx, request.NamespacedName, object); errors.IsNotFound(err) {
		// the instance was deleted and it had no finalizer on it.
		// for the local resources, there is no finalizer so we need to delete the object from the entry handler
		object.SetNamespace(request.Namespace)
		object.SetName(request.Name)
		if err = c.emitter.Delete(object); err != nil {
			log.Errorw("failed to add deleted object into bundle", "error", err, "name", request.Name)
			return ctrl.Result{Requeue: true, RequeueAfter: 5 * time.Second}, err
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

		if err := c.emitter.Delete(object); err != nil {
			log.Errorw("failed to add deleted object into bundle", "error", err, "name", request.Name)
			return ctrl.Result{Requeue: true, RequeueAfter: 5 * time.Second}, err
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
	if err := c.emitter.Update(object); err != nil {
		log.Errorw("failed to add updated object into bundle", "error", err, "name", request.Name)
		return ctrl.Result{Requeue: true, RequeueAfter: 5 * time.Second}, err
	}

	return ctrl.Result{}, nil
}

func IsGlobalResource(obj client.Object) bool {
	return utils.HasLabel(obj, constants.GlobalHubGlobalResourceLabel) ||
		utils.HasAnnotation(obj, constants.OriginOwnerReferenceAnnotation)
}
