package generic

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-logr/logr"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type genericEventController struct {
	log logr.Logger

	runtimeClient    client.Client
	currentVersion   *metadata.BundleVersion
	payload          bundle.Payload
	objectController ObjectController
	finalizerName    string
	lock             *sync.Mutex
}

func (c *genericEventController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
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

func (g *genericEventController) Instance() client.Object {
	return &clusterv1.ManagedCluster{}
}

func (g *genericEventController) Predicate() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(object client.Object) bool {
		return true
	})
}

func (g *genericEventController) EnableFinalizer() bool {
	return s.finalizer
}
