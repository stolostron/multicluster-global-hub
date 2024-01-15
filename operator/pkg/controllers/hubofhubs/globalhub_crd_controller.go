// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubofhubs

import (
	"context"
	"sync"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	// KafkaCRDName is the name of the kafka crd
	KafkaCRDName = "kafkas.kafka.strimzi.io"
	// KafkaTopicCRDName is the name of the kafka topic crd
	KafkaTopicCRDName = "kafkatopics.kafka.strimzi.io"
	// KafkaUserCRDName is the name of the kafka user crd
	KafkaUserCRDName = "kafkausers.kafka.strimzi.io"
)

type crdController struct {
	mgr         ctrl.Manager
	gcontroller *builder.Builder
}

var kafkaPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return false
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion()
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return e.Object.GetNamespace() == utils.GetDefaultNamespace()
	},
}

var kafkaOnce sync.Once
var kafkaTopicOnce sync.Once
var kafkaUserOnce sync.Once

func (c *crdController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	// start to watch kafka/kafkatopic/kafkauser custom resource controller
	if request.Name == KafkaCRDName {
		kafkaOnce.Do(func() {
			c.gcontroller.Watches(&kafkav1beta2.Kafka{},
				&handler.EnqueueRequestForObject{}, builder.WithPredicates(kafkaPred))
		})
	} else if request.Name == KafkaTopicCRDName {
		kafkaTopicOnce.Do(func() {
			c.gcontroller.Watches(&kafkav1beta2.KafkaTopic{},
				&handler.EnqueueRequestForObject{}, builder.WithPredicates(kafkaPred))
		})
	} else if request.Name == KafkaUserCRDName {
		kafkaUserOnce.Do(func() {
			c.gcontroller.Watches(&kafkav1beta2.KafkaUser{},
				&handler.EnqueueRequestForObject{}, builder.WithPredicates(kafkaPred))
		})
	}
	return ctrl.Result{}, nil
}

// this controller is used to watch the Kafka/KafkaTopic/KafkaUser crd
// if the crd exists, then add controllers to the manager dynamically
func StartCRDController(mgr ctrl.Manager, controller *builder.Builder) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&apiextensionsv1.CustomResourceDefinition{}, builder.WithPredicates(predicate.Funcs{
			// trigger to watch the CR only if the crd is created
			CreateFunc: func(e event.CreateEvent) bool {
				if e.Object.GetName() == KafkaCRDName || e.Object.GetName() == KafkaTopicCRDName ||
					e.Object.GetName() == KafkaUserCRDName {
					return true
				}
				return false
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				return false
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return false
			},
			GenericFunc: func(e event.GenericEvent) bool {
				return false
			},
		})).
		Complete(&crdController{
			mgr:         mgr,
			gcontroller: controller,
		})
}
