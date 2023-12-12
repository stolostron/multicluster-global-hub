/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package backup

import (
	"context"
	"reflect"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	// pmcontroller "github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/packagemanifest"
	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	"github.com/go-logr/logr"
	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

const (
	secretType     = "Secret"
	configmapType  = "ConfigMap"
	mghType        = "MulticlusterGlobalHub"
	kafkaType      = "Kafka"
	kafkaUserType  = "KafkaUser"
	kafkaTopicType = "KafkaTopic"
	crdType        = "CustomResourceDefinition"
	pvcType        = "PersistentVolumeClaim"
)

var secretList = sets.NewString(
	operatorconstants.CustomGrafanaIniName,
	operatorconstants.GHTransportSecretName,
	operatorconstants.GHStorageSecretName,
)

var configmapList = sets.NewString(
	operatorconstants.CustomAlertName,
)

var crdList = sets.NewString(
	"kafkas.kafka.strimzi.io",
	"kafkatopics.kafka.strimzi.io",
	"kafkausers.kafka.strimzi.io",
)

// BackupReconciler reconciles a MulticlusterGlobalHub object
type BackupReconciler struct {
	manager.Manager
	client.Client
	Log logr.Logger
}

func NewBackupReconciler(mgr manager.Manager, log logr.Logger) *BackupReconciler {
	return &BackupReconciler{
		Manager: mgr,
		Client:  mgr.GetClient(),
		Log:     log,
	}
}

// As we need to watch mgh, secret, configmap. they should be in the same namespace.
// So for request.Namespace, we set it as request type, like "Secret","Configmap","MulticlusterGlobalHub" and so on.
// In the reconcile, we identy the request kind and get it by request.Name.
func (r *BackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Log.Info("backup reconcile:", "requestType", req.Namespace, "name", req.Name)
	switch req.Namespace {
	case mghType:
		mgh := &globalhubv1alpha4.MulticlusterGlobalHub{}
		return r.addLabel(ctx, req, mgh, constants.BackupKey, constants.BackupActivationValue)
	case secretType:
		secretObj := &corev1.Secret{}
		return r.addLabel(ctx, req, secretObj, constants.BackupKey, constants.BackupGlobalHubValue)
	case configmapType:
		configmapObj := &corev1.ConfigMap{}
		return r.addLabel(ctx, req, configmapObj, constants.BackupKey, constants.BackupGlobalHubValue)
	case kafkaType:
		kafkaObj := &kafkav1beta2.Kafka{}
		return r.addLabel(ctx, req, kafkaObj, constants.BackupKey, constants.BackupGlobalHubValue)
	case kafkaUserType:
		kafkaObj := &kafkav1beta2.KafkaUser{}
		return r.addLabel(ctx, req, kafkaObj, constants.BackupKey, constants.BackupGlobalHubValue)
	case kafkaTopicType:
		kafkaObj := &kafkav1beta2.KafkaTopic{}
		return r.addLabel(ctx, req, kafkaObj, constants.BackupKey, constants.BackupGlobalHubValue)
	case crdType:
		crdObj := &apiextensionsv1.CustomResourceDefinition{}
		return r.addLabel(ctx, req, crdObj, constants.BackupKey, constants.BackupGlobalHubValue)
	case pvcType:
		pvcObj := &corev1.PersistentVolumeClaim{}
		return r.addLabel(ctx, req, pvcObj, constants.BackupVolumnKey, constants.BackupGlobalHubValue)
	}
	return ctrl.Result{}, nil
}

func (r *BackupReconciler) addLabel(
	ctx context.Context,
	req ctrl.Request,
	obj client.Object,
	labelKey string,
	labelValue string) (ctrl.Result, error) {
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: config.GetMGHNamespacedName().Namespace,
		Name:      req.Name,
	}, obj); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	objNewLabels := obj.GetLabels()
	if objNewLabels == nil {
		objNewLabels = make(map[string]string)
	}
	objNewLabels[labelKey] = labelValue
	obj.SetLabels(objNewLabels)

	if err := r.Update(ctx, obj, &client.UpdateOptions{}); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

var mghPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return !utils.HasLabel(e.Object.GetLabels(), constants.BackupKey, constants.BackupActivationValue)
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return !utils.HasLabel(e.ObjectNew.GetLabels(), constants.BackupKey, constants.BackupActivationValue)
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return false
	},
}

var secretPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		if !secretList.Has(e.Object.GetName()) {
			return false
		}
		return !utils.HasLabel(e.Object.GetLabels(), constants.BackupKey, constants.BackupActivationValue)
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		if !secretList.Has(e.ObjectNew.GetName()) {
			return false
		}
		return !utils.HasLabel(e.ObjectNew.GetLabels(), constants.BackupKey, constants.BackupActivationValue)
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return false
	},
}

var configmapPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		if !configmapList.Has(e.Object.GetName()) {
			return false
		}
		return !utils.HasLabel(e.Object.GetLabels(), constants.BackupKey, constants.BackupActivationValue)
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		if !configmapList.Has(e.ObjectNew.GetName()) {
			return false
		}
		return !utils.HasLabel(e.ObjectNew.GetLabels(), constants.BackupKey, constants.BackupActivationValue)

	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return false
	},
}

var commonPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return !utils.HasLabel(e.Object.GetLabels(), constants.BackupKey, constants.BackupActivationValue)
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return !utils.HasLabel(e.ObjectNew.GetLabels(), constants.BackupKey, constants.BackupActivationValue)
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return false
	},
}

var crdPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		if !crdList.Has(e.Object.GetName()) {
			return false
		}
		return !utils.HasLabel(e.Object.GetLabels(), constants.BackupKey, constants.BackupActivationValue)
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		if !crdList.Has(e.ObjectNew.GetName()) {
			return false
		}
		return !utils.HasLabel(e.ObjectNew.GetLabels(), constants.BackupKey, constants.BackupActivationValue)
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return false
	},
}

// SetupWithManager sets up the controller with the Manager.
func (r *BackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).Named("backupController").
		Watches(&globalhubv1alpha4.MulticlusterGlobalHub{},
			objEventHandler,
			builder.WithPredicates(mghPred)).
		Watches(&corev1.Secret{},
			objEventHandler,
			builder.WithPredicates(secretPred)).
		Watches(&corev1.ConfigMap{},
			objEventHandler,
			builder.WithPredicates(configmapPred)).
		Watches(&kafkav1beta2.Kafka{},
			objEventHandler,
			builder.WithPredicates(commonPred)).
		Watches(&kafkav1beta2.KafkaUser{},
			objEventHandler,
			builder.WithPredicates(commonPred)).
		Watches(&kafkav1beta2.KafkaTopic{},
			objEventHandler,
			builder.WithPredicates(commonPred)).
		Watches(&apiextensionsv1.CustomResourceDefinition{},
			objEventHandler,
			builder.WithPredicates(crdPred)).
		Watches(&corev1.PersistentVolumeClaim{},
			objEventHandler,
			builder.WithPredicates(commonPred)).
		Complete(r)
}

func (r *BackupReconciler) Start(ctx context.Context) error {
	//Only when global hub started, then start the backup controller
	_, err := utils.WaitGlobalHubReady(ctx, r.Client, 5*time.Second)
	if err != nil {
		return err
	}

	if err = r.SetupWithManager(r.Manager); err != nil {
		return err
	}
	return nil
}

var objEventHandler = handler.EnqueueRequestsFromMapFunc(
	func(ctx context.Context, obj client.Object) []reconcile.Request {
		t := reflect.TypeOf(obj)
		//Only watch the global hub namespace resources or cluster scope resources
		if len(obj.GetNamespace()) != 0 && (obj.GetNamespace() != config.GetMGHNamespacedName().Namespace) {
			return []reconcile.Request{}
		}
		return []reconcile.Request{
			{
				NamespacedName: types.NamespacedName{
					Namespace: t.Elem().Name(),
					Name:      obj.GetName(),
				},
			},
		}
	},
)
