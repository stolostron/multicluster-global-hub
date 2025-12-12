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
			return ctrl.Result{}, nil
		}
		isResourceRemoved = protocol.IsResourceRemoved()
		if !isResourceRemoved {
			log.Info("Wait kafka resource removed")
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
		return ctrl.Result{}, nil
	}

	if err = config.SetTransportConfig(ctx, r.GetClient(), mgh); err != nil {
		log.Errorf("failed to set transport config: %v", err)
		return ctrl.Result{}, err
	}

	// set the transporter
	switch config.TransporterProtocol() {
	case transport.StrimziTransporter:
		return r.reconcileStrimziKafka(ctx, mgh)
	case transport.SecretTransporter:
		return r.reconcileBYOKafka(ctx, mgh)
	}
	return ctrl.Result{}, nil
}

// reconcileStrimziKafka reconciles the strimzi kafka resources, it will create the kafka cluster and
// start the kafka controller.
func (r *TransportReconciler) reconcileStrimziKafka(ctx context.Context, mgh *v1alpha4.MulticlusterGlobalHub) (
	ctrl.Result, error,
) {
	var err error
	// it only requires to update the component for the strimzi installation, after the strimzi is installed,
	// the component will be updated by the kafka controller.
	requireUpdateComponent := true
	cond := v1alpha4.StatusCondition{
		Kind:    "StrimziKafka",
		Name:    config.COMPONENTS_KAFKA_NAME,
		Type:    config.COMPONENTS_AVAILABLE,
		Status:  config.CONDITION_STATUS_TRUE,
		Reason:  "StrimziKafkaReady",
		Message: "Strimzi Kafka is ready",
	}
	defer func() {
		if !requireUpdateComponent {
			return
		}

		if err != nil {
			cond.Status = config.CONDITION_STATUS_FALSE
			cond.Reason = config.RECONCILE_ERROR
			cond.Message = err.Error()
		}
		err = config.UpdateMGHComponent(ctx, r.GetClient(), cond, true)
		if err != nil {
			log.Errorf("failed to update mgh status, err:%v", err)
		}
	}()

	strimziTransporter := protocol.NewStrimziTransporter(
		r.Manager,
		mgh,
		protocol.WithContext(ctx),
		protocol.WithCommunity(operatorutils.IsCommunityMode()),
	)
	r.transporter = strimziTransporter

	// reconcile for the kafka resources based on the mgh configuration
	needRequeue, err := strimziTransporter.EnsureKafka()
	if err != nil {
		return ctrl.Result{}, err
	}
	if needRequeue {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	// deliver the transport resources and config secret to the kafka controller
	// TODO: may need to wait the kafka resources to be ready before delivering the config secret
	err = protocol.StartKafkaController(ctx, r.Manager, r.transporter)
	if err != nil {
		return ctrl.Result{}, err
	}
	requireUpdateComponent = false
	return ctrl.Result{}, nil
}

// reconcileBYOKafka reconciles the byo kafka resources, it will create the transport-config secret for manager.
func (r *TransportReconciler) reconcileBYOKafka(ctx context.Context, mgh *v1alpha4.MulticlusterGlobalHub) (
	ctrl.Result, error,
) {
	var err error
	cond := v1alpha4.StatusCondition{
		Kind:    "TransportConnection",
		Name:    config.COMPONENTS_KAFKA_NAME,
		Type:    config.COMPONENTS_AVAILABLE,
		Status:  config.CONDITION_STATUS_TRUE,
		Reason:  "TransportConnectionReady",
		Message: "Use customized credentials, the transport-config secret has been created",
	}
	defer func() {
		if err != nil {
			cond.Status = config.CONDITION_STATUS_FALSE
			cond.Reason = config.RECONCILE_ERROR
			cond.Message = err.Error()
		}
		err = config.UpdateMGHComponent(ctx, r.GetClient(), cond, true)
		if err != nil {
			log.Errorf("failed to update mgh status, err:%v", err)
		}
	}()

	r.transporter = protocol.NewBYOTransporter(ctx, types.NamespacedName{
		Namespace: mgh.Namespace,
		Name:      constants.GHTransportSecretName,
	}, r.GetClient())
	// all of hubs will get the same credential
	conn, err := r.transporter.GetConnCredential("")
	if err != nil {
		return ctrl.Result{}, err
	}
	if err != nil {
		return ctrl.Result{}, err
	}
	err = protocol.CreateManagerTransportSecret(ctx, mgh, conn, r.GetClient())
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}
