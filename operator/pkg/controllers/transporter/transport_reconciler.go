package transporter

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	protocolTransport protocol.Transporter
}

func GetKafkaConfig(clusterName string) (bool, *transport.KafkaConfig, error) {
	if transportReconciler == nil || transportReconciler.protocolTransport == nil {
		return false, nil, fmt.Errorf("transport reconciler or protocol transport is not initialized")
	}

	ready, kafkaConfig, err := transportReconciler.protocolTransport.GetConnCredential(clusterName)
	if err != nil {
		return false, nil, fmt.Errorf("failed to get kafka config, err:%v", err)
	}
	if !ready {
		return false, nil, nil
	}
	return true, kafkaConfig, nil
}

func Cleanup(clusterName string) error {
	if transportReconciler == nil || transportReconciler.protocolTransport == nil {
		return fmt.Errorf("transport reconciler or protocol transport is not initialized")
	}
	return transportReconciler.protocolTransport.Prune(clusterName)
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
	log.Infof("init transport controller")
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
		isResourceRemoved = protocol.IsResourceRemoved()
		if !isResourceRemoved {
			log.Info("Wait kafka resource removed")
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
		r.protocolTransport = nil
		return ctrl.Result{}, nil
	}

	transportComponentStatus := v1alpha4.StatusCondition{
		Kind:    "Transport",
		Name:    config.COMPONENTS_KAFKA_NAME,
		Type:    config.COMPONENTS_AVAILABLE,
		Status:  config.CONDITION_STATUS_TRUE,
		Reason:  "TransportInitialized",
		Message: "built-in kafka transport is initialized",
	}

	defer func() {
		if config.IsBYOKafka() {
			transportComponentStatus.Message = "byo kafka transport is initialized, using provided secret"
		}
		if err != nil {
			transportComponentStatus.Reason = config.RECONCILE_ERROR
			transportComponentStatus.Message = err.Error()
			transportComponentStatus.Status = config.CONDITION_STATUS_FALSE
		}

		if mgh.Status.Components == nil {
			mgh.Status.Components = map[string]v1alpha4.StatusCondition{}
		}

		_, ok := mgh.Status.Components[transportComponentStatus.Name]
		if ok {
			return
		}
		// initialize the transport component status, it will be updated by the kafka controller
		mgh.Status.Components[transportComponentStatus.Name] = transportComponentStatus
		err := r.GetClient().Status().Update(ctx, mgh)
		if err != nil {
			log.Errorf("failed to update transport component status, err:%v", err)
		}
	}()

	// initialize the protocol transport
	err = r.initTransport(ctx, mgh)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.protocolTransport.Validate()
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("the transport configuration is not set correctly, err %v", err)
	}

	err = r.protocolTransport.Initialize()
	if err != nil {
		log.Infof("waiting for the transport resources to be ready: %s", err.Error())
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

func (r *TransportReconciler) initTransport(ctx context.Context, mgh *v1alpha4.MulticlusterGlobalHub) error {
	protocolType, err := getProtocol(ctx, r.GetClient(), mgh.Namespace)
	if err != nil {
		return fmt.Errorf("failed to get transport protocol, err:%v", err)
	}
	switch protocolType {
	case protocol.StrimziTransport:
		strimiziTransport := protocol.EnsureStrimziTransport(
			r.Manager,
			mgh,
			protocol.WithContext(ctx),
			protocol.WithCommunity(operatorutils.IsCommunityMode()),
		)
		// this controller also will update the transport connection
		err = protocol.StartKafkaController(ctx, r.Manager)
		if err != nil {
			return err
		}
		r.protocolTransport = strimiziTransport
		config.SetBYOKafka(false)
	case protocol.BYOTransport:
		r.protocolTransport = protocol.EnsureBYOTransport(ctx, mgh, r.GetClient())
		config.SetBYOKafka(true)
	default:
		return fmt.Errorf("invalid protocol type: %v", protocolType)
	}
	return nil
}

func getProtocol(ctx context.Context, runtimeClient client.Client, namespace string) (protocol.TransportType, error) {
	kafkaSecret := &corev1.Secret{}
	err := runtimeClient.Get(ctx, types.NamespacedName{
		Name:      constants.GHTransportSecretName,
		Namespace: namespace,
	}, kafkaSecret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return protocol.StrimziTransport, nil
		}
		return protocol.BYOTransport, err
	}
	return protocol.BYOTransport, err
}
