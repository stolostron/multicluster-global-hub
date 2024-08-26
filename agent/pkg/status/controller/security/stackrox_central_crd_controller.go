package security

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const StackroxCentralCRDName string = "centrals.platform.stackrox.io"

var stackroxCRControllerEnabled bool = false

type stackroxCentralCRDController struct {
	ctx    context.Context
	mgr    manager.Manager
	log    logr.Logger
	client client.Client
}

func (s *stackroxCentralCRDController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	result, err := s.reconcile(ctx)
	if err != nil {
		s.log.Info("Stackrox Central CR controller is disabled")
	}

	return result, nil
}

func (s *stackroxCentralCRDController) reconcile(ctx context.Context) (ctrl.Result, error) {
	centralCRD, err := getResource(s.ctx, s.client, StackroxCentralCRDName, "", &v1.CustomResourceDefinition{}, s.log)
	if err != nil {
		return ctrl.Result{}, err
	} else if centralCRD == nil {
		return ctrl.Result{}, nil
	}

	mutex.Lock()
	defer mutex.Unlock()

	// Stackrox Cental CR controller should be launched once
	if stackroxCRControllerEnabled {
		return ctrl.Result{}, nil
	}

	s.log.Info("Found Stackrox Central CRD, enabling Stackrox Central CR controller")

	if err := LaunchStackroxCentralCRController(ctx, s.mgr); err != nil {
		return reconcile.Result{}, err
	}

	stackroxCRControllerEnabled = true
	s.log.Info("Stackrox Central CR controller is enabled")

	return ctrl.Result{}, nil
}

func LaunchStackroxCentralCRDController(ctx context.Context, mgr ctrl.Manager) error {
	controller := stackroxCentralCRDController{
		ctx:    ctx,
		mgr:    mgr,
		log:    ctrl.Log.WithName("stackrox-central-crd-controller"),
		client: mgr.GetClient(),
	}

	// initial reconcile
	if _, err := controller.Reconcile(ctx, reconcile.Request{}); err != nil {
		return fmt.Errorf("failed to perform the initial reconcile for stackrox central CRD")
	}

	controller.log.Info("Adding Stackrox Central CRD controller")
	if err := ctrl.NewControllerManagedBy(mgr).
		Named("stackrox_central_crd_controller").
		For(&v1.CustomResourceDefinition{}).
		Complete(&controller); err != nil {
		return fmt.Errorf("failed to create Stacktox Central CRD controller: %v", err)
	}

	return nil
}
