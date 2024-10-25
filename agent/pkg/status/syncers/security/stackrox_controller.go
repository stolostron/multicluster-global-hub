package security

import (
	"context"
	"fmt"

	zaplogger "github.com/stolostron/multicluster-global-hub/pkg/logger"
	"go.uber.org/zap"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	cr "sigs.k8s.io/controller-runtime"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	crmanager "sigs.k8s.io/controller-runtime/pkg/manager"
)

type stackRoxController struct {
	manager crmanager.Manager
	logger  *zap.SugaredLogger
	client  crclient.Client
	syncer  *StackRoxSyncer
}

func (s *stackRoxController) Reconcile(ctx context.Context, request cr.Request) (result cr.Result, err error) {
	// If the object doesn't exist or has been deleted then unregister it:
	centralObject := &unstructured.Unstructured{}
	centralObject.SetGroupVersionKind(centralCRGVK)
	err = s.client.Get(ctx, request.NamespacedName, centralObject)
	if apierrors.IsNotFound(err) {
		err = s.syncer.Unregister(ctx, request.NamespacedName)
		if err != nil {
			return
		}
	}
	if !centralObject.GetDeletionTimestamp().IsZero() {
		err = s.syncer.Unregister(ctx, request.NamespacedName)
		if err != nil {
			return
		}
	}

	// If the object exists and has the annotation that contains the location of the details secret then
	// register it:
	_, ok := centralObject.GetAnnotations()[stacRoxDetailsAnnotation]
	if ok {
		err = s.syncer.Register(ctx, request.NamespacedName)
		if err != nil {
			return
		}
	}

	return
}

func AddStacRoxController(manager cr.Manager, syncer *StackRoxSyncer) error {
	centralObject := &unstructured.Unstructured{}
	centralObject.SetGroupVersionKind(centralCRGVK)

	controller := stackRoxController{
		manager: manager,
		logger:  zaplogger.ZapLogger("stack-rock-controller"),
		client:  manager.GetClient(),
		syncer:  syncer,
	}

	if err := cr.NewControllerManagedBy(manager).
		For(centralObject).
		Complete(&controller); err != nil {
		return fmt.Errorf("failed to create Stackrox Central CR controller: %v", err)
	}

	return nil
}
