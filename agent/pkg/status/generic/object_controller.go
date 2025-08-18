package generic

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/emitters"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
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
		finalizerName: "",
	}

	return ctrl.NewControllerManagedBy(mgr).For(instanceFunc()).WithEventFilter(objectEmitter.EventFilter()).
		Named(controllerName).Complete(syncer)
}

func (c *syncController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	object := c.instance()
	if err := c.client.Get(ctx, request.NamespacedName, object); errors.IsNotFound(err) {
		// the instance was deleted and it had no finalizer on it.
		// for the local resources, there is no finalizer so we need to add the delete event in the bundle
		object.SetNamespace(request.Namespace)
		object.SetName(request.Name)
		log.Infof("Object %s/%s not found, deleting from bundle\n", request.Namespace, request.Name)
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
		// Global resource finalizer management removed

		if err := c.emitter.Delete(object); err != nil {
			log.Errorw("failed to add deleted object into bundle", "error", err, "name", request.Name)
			return ctrl.Result{Requeue: true, RequeueAfter: 5 * time.Second}, err
		}

		return ctrl.Result{}, nil
	}

	// Global resource finalizer management removed

	// update/insert
	cleanObject(object)
	if err := c.emitter.Update(object); err != nil {
		log.Errorw("failed to add updated object into bundle", "error", err, "name", request.Name)
		return ctrl.Result{Requeue: true, RequeueAfter: 5 * time.Second}, err
	}

	return ctrl.Result{}, nil
}

// IsGlobalResource function removed - global resource functionality no longer supported
