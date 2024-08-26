package security

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type stackroxCentralCRController struct {
	ctx       context.Context
	mgr       manager.Manager
	log       logr.Logger
	client    client.Client
	centralCR *unstructured.Unstructured
}

func (s *stackroxCentralCRController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := s.log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("ACS Central CR controller", "NamespacedName:", request.NamespacedName)

	centralCR, err := getResource(s.ctx, s.client, request.Name, request.Namespace, s.centralCR, reqLogger)
	if err != nil {
		return ctrl.Result{}, err
	} else if centralCR == nil {
		if _, ok := stackroxCentrals[stackroxCentral{name: request.Name, namespace: request.Namespace}]; ok {
			reqLogger.Info("Deleting Stackrox Central instance")
			delete(stackroxCentrals, stackroxCentral{name: request.Name, namespace: request.Namespace})
		}
		return ctrl.Result{}, nil
	}

	if _, ok := stackroxCentrals[stackroxCentral{name: request.Name, namespace: request.Namespace}]; !ok {
		reqLogger.Info("Found a new central instance to sync")
		stackroxCentrals[stackroxCentral{name: request.Name, namespace: request.Namespace}] = &stackroxCentralData{}
	}

	return ctrl.Result{}, nil
}

func LaunchStackroxCentralCRController(ctx context.Context, mgr ctrl.Manager) error {
	central := &unstructured.Unstructured{}
	central.SetGroupVersionKind(centralCRGVK)

	controller := stackroxCentralCRController{
		ctx:       ctx,
		mgr:       mgr,
		log:       ctrl.Log.WithName("stackrox-central-cr-controller"),
		client:    mgr.GetClient(),
		centralCR: central,
	}

	controller.log.Info("Adding Stackrox Central CR controller")
	if err := ctrl.NewControllerManagedBy(mgr).
		For(central).
		Complete(&controller); err != nil {
		return fmt.Errorf("failed to create Stackrox Central CR controller: %v", err)
	}

	return nil
}
