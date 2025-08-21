package transporter

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
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
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

// +kubebuilder:rbac:groups=kafka.strimzi.io,resources=kafkas;kafkatopics;kafkausers;kafkanodepools,verbs=get;create;list;watch;update;delete
// +kubebuilder:rbac:groups=operators.coreos.com,resources=clusterserviceversions,verbs=get;delete;list;watch
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=podmonitors,verbs=get;create;delete;update;list;watch
// +kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterglobalhubs,verbs=get;list;watch;
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;delete;patch

var WatchedSecret = sets.NewString(
	constants.GHTransportSecretName,
)

var (
	log                 = logger.DefaultZapLogger()
	isResourceRemoved   = true
	transportReconciler *TransportReconciler
	updateConn          bool
)

type TransportReconciler struct {
	ctrl.Manager
	transporter transport.Transporter
}

func (c *TransportReconciler) IsResourceRemoved() bool {
	log.Infof("TransportController resource removed: %v", isResourceRemoved)
	return isResourceRemoved
}

func StartController(controllerOption config.ControllerOption) (config.ControllerInterface, error) {
	if transportReconciler != nil {
		return transportReconciler, nil
	}
	log.Info("start transport controller")

	transportReconciler = NewTransportReconciler(controllerOption.Manager)
	err := transportReconciler.SetupWithManager(controllerOption.Manager)
	if err != nil {
		transportReconciler = nil
		return nil, err
	}
	log.Infof("inited transport controller")
	return transportReconciler, nil
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
	log.Debugf("reconcile transport controller")
	mgh, err := config.GetMulticlusterGlobalHub(ctx, r.GetClient())
	if err != nil {
		return ctrl.Result{}, err
	}
	if mgh == nil || config.IsPaused(mgh) {
		return ctrl.Result{}, nil
	}
	if mgh.DeletionTimestamp != nil {
		if config.IsBYOKafka() {
			isResourceRemoved = true
			config.SetTransporterConn(nil)
			return ctrl.Result{}, nil
		}
		isResourceRemoved = protocol.IsResourceRemoved()
		if !isResourceRemoved {
			log.Info("Wait kafka resource removed")
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
		config.SetTransporterConn(nil)
		return ctrl.Result{}, nil
	}
	var reconcileErr error
	defer func() {
		if !config.IsBYOKafka() {
			return
		}

		err = config.UpdateMGHComponent(ctx, r.GetClient(),
			getTransportComponentStatus(reconcileErr),
			updateConn,
		)
		if err != nil {
			log.Errorf("failed to update mgh status, err:%v", err)
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
		needRequeue, err := r.transporter.EnsureKafka()
		if err != nil {
			return ctrl.Result{}, err
		}
		if needRequeue {
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
		// this controller also will update the transport connection
		err = protocol.StartKafkaController(ctx, r.Manager, r.transporter)
		if err != nil {
			return ctrl.Result{}, err
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
		updateConn = config.SetTransporterConn(conn)
	}
	// update the transporter
	config.SetTransporter(r.transporter)
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
