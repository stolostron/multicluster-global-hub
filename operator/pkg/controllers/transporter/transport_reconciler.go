package transporter

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/transporter/protocol"
	operatorutils "github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var WatchedSecret = sets.NewString(
	constants.GHTransportSecretName,
)
var started bool

type TransportReconciler struct {
	ctrl.Manager
	kafkaController *protocol.KafkaController
	transporter     transport.Transporter
}

func StartController(controllerOption config.ControllerOption) error {
	if started {
		return nil
	}
	err := NewTransportReconciler(controllerOption.Manager).SetupWithManager(controllerOption.Manager)
	if err != nil {
		return err
	}
	started = true
	klog.Infof("inited transport controller")
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TransportReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).Named("transport").
		For(&v1alpha4.MulticlusterGlobalHub{},
			builder.WithPredicates(config.MGHPred)).
		Watches(&corev1.Secret{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(secretPred)).
		Complete(r)
}

var mghPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return true
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return true
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return false
	},
}

var secretPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return secretCond(e.Object)
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return secretCond(e.ObjectNew)
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return secretCond(e.Object)
	},
}

func secretCond(obj client.Object) bool {
	if WatchedSecret.Has(obj.GetName()) {
		return true
	}
	if obj.GetLabels()["strimzi.io/cluster"] == protocol.KafkaClusterName &&
		obj.GetLabels()["strimzi.io/kind"] == "KafkaUser" {
		return true
	}
	return false
}

func NewTransportReconciler(mgr ctrl.Manager) *TransportReconciler {
	return &TransportReconciler{Manager: mgr}
}

// Resources reconcile the transport resources and also update transporter on the configuration
func (r *TransportReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	mgh, err := config.GetMulticlusterGlobalHub(ctx, r.GetClient())
	if err != nil {
		return ctrl.Result{}, err
	}
	if mgh == nil || config.IsPaused(mgh) || mgh.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	var reconcileErr error
	defer func() {
		if !config.IsBYOKafka() {
			return
		}

		err = config.UpdateMGHComponent(ctx, r.GetClient(),
			getTransportComponentStatus(reconcileErr),
		)
		if err != nil {
			klog.Errorf("failed to update mgh status, err:%v", err)
		}
	}()

	reconcileErr = config.SetTransportConfig(ctx, r.GetClient(), mgh)
	if reconcileErr != nil {
		return ctrl.Result{}, reconcileErr
	}

	// set the transporter
	switch config.TransporterProtocol() {
	case transport.StrimziTransporter:
		// initialize strimzi
		// kafkaCluster, it will be blocking until the status is ready
		r.transporter = protocol.NewStrimziTransporter(
			r.Manager,
			mgh,
			protocol.WithContext(ctx),
			protocol.WithCommunity(operatorutils.IsCommunityMode()),
		)
		if _, err := r.transporter.EnsureKafka(); err != nil {
			return ctrl.Result{}, err
		}

		// this controller also will update the transport connection
		if config.GetKafkaResourceReady() && r.kafkaController == nil {
			r.kafkaController, err = protocol.StartKafkaController(ctx, r.Manager, r.transporter)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
	case transport.SecretTransporter:
		r.transporter = protocol.NewBYOTransporter(ctx, types.NamespacedName{
			Namespace: mgh.Namespace,
			Name:      constants.GHTransportSecretName,
		}, r.GetClient())
		// all of hubs will get the same credential
		conn, err := r.transporter.GetConnCredential("")
		if err != nil {
			return ctrl.Result{}, err
		}
		config.SetTransporterConn(conn)

	}
	return ctrl.Result{}, nil
}

func getTransportComponentStatus(reconcileErr error,
) v1alpha4.StatusCondition {
	name := config.COMPONENTS_KAFKA_NAME
	availableType := config.COMPONENTS_AVAILABLE
	if reconcileErr != nil {
		return v1alpha4.StatusCondition{
			Kind:    "TransportConnection",
			Name:    name,
			Type:    availableType,
			Status:  config.CONDITION_STATUS_FALSE,
			Reason:  config.RECONCILE_ERROR,
			Message: reconcileErr.Error(),
		}
	}
	if config.GetTransporterConn() == nil {
		return v1alpha4.StatusCondition{
			Kind:    "TransportConnection",
			Name:    name,
			Type:    availableType,
			Status:  config.CONDITION_STATUS_FALSE,
			Reason:  "TransportConnectionNotSet",
			Message: "Transport connection is null",
		}
	}

	return v1alpha4.StatusCondition{
		Kind:    "TransportConnection",
		Name:    name,
		Type:    availableType,
		Status:  config.CONDITION_STATUS_TRUE,
		Reason:  "TransportConnectionSet",
		Message: "Use customized transport, connection has set using provided secret",
	}
}
