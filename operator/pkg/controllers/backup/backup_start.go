/*
Copyright 2023.

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

	"github.com/go-logr/logr"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var (
	postgresPvcLabelKey   = "component"
	postgresPvcLabelValue = "multicluster-global-hub-operator"
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

// SetupWithManager sets up the controller with the Manager.
func (r *BackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).Named("backupController").
		For(&globalhubv1alpha4.MulticlusterGlobalHub{},
			builder.WithPredicates(mghPred)).
		Watches(&corev1.Secret{},
			objEventHandler,
			builder.WithPredicates(secretPred)).
		Watches(&corev1.ConfigMap{},
			objEventHandler,
			builder.WithPredicates(configmapPred)).
		Watches(&corev1.PersistentVolumeClaim{},
			objEventHandler,
			builder.WithPredicates(pvcPred)).
		Watches(&mchv1.MultiClusterHub{},
			mchEventHandler,
			builder.WithPredicates(mchPred)).
		Complete(r)
}

var mchEventHandler = handler.EnqueueRequestsFromMapFunc(
	func(ctx context.Context, obj client.Object) []reconcile.Request {
		t := reflect.TypeOf(obj)
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

var mghPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return !utils.HasItem(e.Object.GetLabels(), constants.BackupKey, constants.BackupActivationValue)
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return !utils.HasItem(e.ObjectNew.GetLabels(), constants.BackupKey, constants.BackupActivationValue)
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
		return !utils.HasItem(e.Object.GetLabels(), constants.BackupKey, constants.BackupActivationValue)
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		if !secretList.Has(e.ObjectNew.GetName()) {
			return false
		}
		return !utils.HasItem(e.ObjectNew.GetLabels(), constants.BackupKey, constants.BackupActivationValue)
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
		return !utils.HasItem(e.Object.GetLabels(), constants.BackupKey, constants.BackupActivationValue)
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		if !configmapList.Has(e.ObjectNew.GetName()) {
			return false
		}
		return !utils.HasItem(e.ObjectNew.GetLabels(), constants.BackupKey, constants.BackupActivationValue)
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return false
	},
}

var mchPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return true
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		// only requeue when spec change, if the resource do not have spec field, the generation is always 0
		if e.ObjectNew.GetGeneration() == 0 {
			return true
		}
		return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return true
	},
}

var pvcPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		// only watch postgres pvc
		return utils.HasItem(e.Object.GetLabels(), postgresPvcLabelKey, postgresPvcLabelValue)
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return utils.HasItem(e.ObjectNew.GetLabels(), postgresPvcLabelKey, postgresPvcLabelValue)
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return false
	},
}

var objEventHandler = handler.EnqueueRequestsFromMapFunc(
	func(ctx context.Context, obj client.Object) []reconcile.Request {
		t := reflect.TypeOf(obj)
		// Only watch the global hub namespace resources or cluster scope resources
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
