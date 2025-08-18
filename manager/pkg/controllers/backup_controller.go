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

package controllers

import (
	"context"
	"database/sql"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var backupLog = logger.ZapLogger("backup-ctrl")

// BackupReconciler reconciles a MulticlusterGlobalHub object
type BackupPVCReconciler struct {
	manager.Manager
	client.Client
	sqlConn *sql.Conn
}

func NewBackupPVCReconciler(mgr manager.Manager, sqlConn *sql.Conn) *BackupPVCReconciler {
	return &BackupPVCReconciler{
		Manager: mgr,
		Client:  mgr.GetClient(),
		sqlConn: sqlConn,
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *BackupPVCReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).Named("backupPvcController").
		For(&corev1.PersistentVolumeClaim{},
			builder.WithPredicates(pvcPred)).
		Complete(r)
}

var pvcPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return IfPvcNeedBackup(e.Object)
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return IfPvcNeedBackup(e.ObjectNew)
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return false
	},
}

func IfPvcNeedBackup(pvc client.Object) bool {
	// Only watch pvcs which need backup
	if !utils.HasItem(pvc.GetLabels(), constants.BackupVolumnKey, constants.BackupGlobalHubValue) {
		return false
	}
	// Only run backup when volsync is waiting for trigger
	if !utils.HasItem(pvc.GetAnnotations(), constants.BackupPvcLatestCopyStatus, constants.BackupPvcWaitingForTrigger) {
		return false
	}
	// If volsync is in progress, just wait volsync finish
	if utils.HasItemKey(pvc.GetAnnotations(), constants.BackupPvcLatestCopyTrigger) {
		// Wait volsync process
		if pvc.GetAnnotations()[constants.BackupPvcLatestCopyTrigger] !=
			pvc.GetAnnotations()[constants.BackupPvcCopyTrigger] {
			return false
		}
	}
	return true
}

func (r *BackupPVCReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	isBackupEnabled, err := utils.IsBackupEnabled(ctx, r.Client)
	if err != nil {
		backupLog.Error(err, "failed to get backup enabled", "req", req)
		return ctrl.Result{}, err
	}
	database.IsBackupEnabled = isBackupEnabled
	if !isBackupEnabled {
		backupLog.Debug("Backup is not enabled")
		return ctrl.Result{}, nil
	}

	backupLog.Debug("Start backup pvc", "req", req)

	err = database.Lock(r.sqlConn)
	if err != nil {
		backupLog.Error(err, "failed to get db lock")
		return ctrl.Result{}, err
	}

	defer database.Unlock(r.sqlConn)

	triggerTime := time.Now().Format(time.RFC3339)
	formatTriggerTime := strings.ReplaceAll(triggerTime, ":", ".")

	pvc := &corev1.PersistentVolumeClaim{}
	err = r.Get(ctx, req.NamespacedName, pvc)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = utils.AddAnnotation(ctx,
		r.Client, pvc, pvc.Namespace, pvc.Name,
		constants.BackupPvcCopyTrigger,
		formatTriggerTime)
	if err != nil {
		return ctrl.Result{}, err
	}
	backupLog.Debugw("Start wait pvc backup finish", "time", time.Now())
	err = wait.PollUntilContextTimeout(ctx, 5*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		pvc := &corev1.PersistentVolumeClaim{}
		err := r.Get(ctx, req.NamespacedName, pvc)
		if err != nil {
			return false, nil
		}
		if !utils.HasItem(pvc.GetAnnotations(), constants.BackupPvcLatestCopyStatus, constants.BackupPvcCompletedTrigger) {
			return false, nil
		}
		if !utils.HasItem(pvc.GetAnnotations(), constants.BackupPvcLatestCopyTrigger, formatTriggerTime) {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		backupLog.Error(err, "Time out to wait backup pvc finished")
		return ctrl.Result{}, err
	}
	backupLog.Debug("pvc backup finish", "time", time.Now())
	return ctrl.Result{}, nil
}
