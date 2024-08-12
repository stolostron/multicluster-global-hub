// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package protocol

import (
	"context"
	"embed"
	"time"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorutils "github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

//go:embed manifests
var manifests embed.FS

// KafkaController reconciles the kafka crd
type KafkaController struct {
	ctrl.Manager
}

func (r *KafkaController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	mgh := &v1alpha4.MulticlusterGlobalHub{}
	if err := r.GetClient().Get(ctx, config.GetMGHNamespacedName(), mgh); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// kafkaCluster, it will be blocking until the status is ready
	trans, err := NewStrimziTransporter(
		r.Manager,
		mgh,
		WithContext(ctx),
		WithCommunity(operatorutils.IsCommunityMode()),
	)
	if err != nil {
		return ctrl.Result{}, err
	}
	// update the transporter
	config.SetTransporter(trans)

	// update the transport connection
	conn, err := waitTransportConn(ctx, trans, DefaultGlobalHubKafkaUserName)
	if err != nil {
		return ctrl.Result{}, err
	}
	config.SetTransporterConn(conn)

	return ctrl.Result{}, nil
}

var kafkaPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return false
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return e.ObjectNew.GetGeneration() != e.ObjectOld.GetGeneration()
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return e.Object.GetNamespace() == utils.GetDefaultNamespace()
	},
}

var mghPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return true
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return e.ObjectNew.GetGeneration() != e.ObjectOld.GetGeneration()
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return false
	},
}

func StartKafkaController(ctx context.Context, mgr ctrl.Manager) (*KafkaController, error) {
	r := &KafkaController{Manager: mgr}

	// we need to ensure the tranport it ready, then reconcile others(synchoronized)
	_, err := r.Reconcile(ctx, ctrl.Request{})
	if err != nil {
		return nil, err
	}

	// even if the following controller will reconcile the transport, but it's asynchoronized
	err = ctrl.NewControllerManagedBy(mgr).
		Named("kafka_controller").
		For(&v1alpha4.MulticlusterGlobalHub{}, builder.WithPredicates(mghPred)).
		Watches(&kafkav1beta2.Kafka{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(kafkaPred)).
		Watches(&kafkav1beta2.KafkaUser{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(kafkaPred)).
		Watches(&kafkav1beta2.KafkaTopic{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(kafkaPred)).
		Complete(r)
	if err != nil {
		return nil, err
	}
	klog.Info("kafka controller is started")
	return r, nil
}

func waitTransportConn(ctx context.Context, trans *strimziTransporter, kafkaUserSecret string) (
	*transport.ConnCredential, error,
) {
	// set transporter connection
	var conn *transport.ConnCredential
	var err error
	err = wait.PollUntilContextTimeout(ctx, 2*time.Second, 10*time.Minute, true,
		func(ctx context.Context) (bool, error) {
			conn, err = trans.getConnCredentailByCluster()
			if err != nil {
				klog.Info("waiting the kafka cluster credential to be ready...", "message", err.Error())
				return false, err
			}

			if err := trans.loadUserCredentail(kafkaUserSecret, conn); err != nil {
				klog.Info("waiting the kafka user credential to be ready...", "message", err.Error())
				return false, err
			}
			return true, nil
		})
	if err != nil {
		return nil, err
	}
	return conn, nil
}
