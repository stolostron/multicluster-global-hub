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
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/condition"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
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

var allResourcesBackup = map[string]Backup{
	secretType:     NewSecretBackup(),
	configmapType:  NewConfigmapBackup(),
	mghType:        NewMghBackup(),
	kafkaType:      NewKafkaBackup(),
	kafkaUserType:  NewKafkaUserBackup(),
	kafkaTopicType: NewKafkaTopicBackup(),
	crdType:        NewCrdBackup(),
	pvcType:        NewPvcBackup(),
}

// As we need to watch mgh, secret, configmap. they should be in the same namespace.
// So for request.Namespace, we set it as request type, like "Secret","Configmap","MulticlusterGlobalHub" and so on.
// In the reconcile, we identy the request kind and get it by request.Name.
func (r *BackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	//mgh is used to update backup condition
	mghList := &globalhubv1alpha4.MulticlusterGlobalHubList{}
	err := r.Client.List(ctx, mghList)
	if err != nil {
		klog.Error(err, "Failed to list MulticlusterGlobalHub")
		return ctrl.Result{}, err
	}
	if len(mghList.Items) == 0 {
		return ctrl.Result{}, nil
	}
	mgh := mghList.Items[0].DeepCopy()
	//Backup condition means added backup label to all resources already
	backuped := meta.IsStatusConditionTrue(mgh.Status.Conditions, condition.CONDITION_TYPE_BACKUP)

	//Check if backup is enabled
	backupEnabled, err := isBackupEnabled(ctx, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	//If backup is not enable, need to clean up the backup labels
	if !backupEnabled {
		if !backuped {
			return addDisableCondition(ctx, r.Client, mgh, nil)
		}
		err := r.deleteLableOfAllResources(ctx)
		if err != nil {
			r.Log.Error(err, "Failed to delete backup labels")
		}
		return addDisableCondition(ctx, r.Client, mgh, err)
	}

	//If backup is enable, need to add backup label to all resources
	if !backuped {
		err := r.addLableToAllResources(ctx)
		if err != nil {
			r.Log.Error(err, "Failed to add backup labels")
		}
		return addBackupCondition(ctx, r.Client, mgh, err)
	}

	//Watch the changed resources, then update the backuplabel
	r.Log.Info("backup reconcile:", "requestType", req.Namespace, "name", req.Name)
	_, found := allResourcesBackup[req.Namespace]
	if !found {
		return ctrl.Result{}, nil
	}
	err = allResourcesBackup[req.Namespace].AddLabelToOneObj(
		ctx, r.Client,
		config.GetMGHNamespacedName().Namespace, req.Name)
	return addBackupCondition(ctx, r.Client, mgh, err)
}

func addDisableCondition(ctx context.Context, client client.Client,
	mgh *globalhubv1alpha4.MulticlusterGlobalHub, err error,
) (ctrl.Result, error) {
	msg := condition.CONDITION_MESSAGE_BACKUP_DISABLED
	if err != nil {
		msg = fmt.Sprintf("Backup is diabled with error: %v.", err.Error())
	}
	if err := condition.SetCondition(ctx, client, mgh,
		condition.CONDITION_TYPE_BACKUP,
		condition.CONDITION_STATUS_FALSE,
		condition.CONDITION_REASON_BACKUP_DISABLED,
		msg,
	); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func addBackupCondition(ctx context.Context, client client.Client,
	mgh *globalhubv1alpha4.MulticlusterGlobalHub, err error,
) (ctrl.Result, error) {
	msg := condition.CONDITION_MESSAGE_BACKUP
	if err != nil {
		msg = fmt.Sprintf("Added backup labels with error: %v.", err.Error())
	}
	if err := condition.SetCondition(ctx, client, mgh,
		condition.CONDITION_TYPE_BACKUP,
		condition.CONDITION_STATUS_TRUE,
		condition.CONDITION_REASON_BACKUP,
		msg,
	); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func addLabel(
	ctx context.Context, client client.Client, obj client.Object,
	namespace, name string,
	labelKey string, labelValue string) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		err := client.Get(ctx, types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		}, obj)
		if err != nil {
			if errors.IsNotFound(err) {
				return nil
			}
			klog.Errorf("Failed to get %v/%v, err:%v", namespace, name, err)
			return err
		}

		if utils.HasLabel(obj.GetLabels(), labelKey, labelValue) {
			return nil
		}

		objNewLabels := obj.GetLabels()
		if objNewLabels == nil {
			objNewLabels = make(map[string]string)
		}
		objNewLabels[labelKey] = labelValue
		obj.SetLabels(objNewLabels)

		if err := client.Update(ctx, obj); err != nil {
			klog.Errorf("Failed to update %v/%v, err:%v", namespace, name, err)
			return err
		}
		return nil
	})
}

func deleteLabel(
	ctx context.Context, client client.Client, obj client.Object,
	namespace, name string,
	labelKey string) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		err := client.Get(ctx, types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		}, obj)
		if err != nil {
			if errors.IsNotFound(err) {
				return nil
			}
			klog.Errorf("Failed to get %v/%v, err:%v", namespace, name, err)
			return err
		}

		if !utils.HasLabelKey(obj.GetLabels(), labelKey) {
			return nil
		}

		objNewLabels := obj.GetLabels()
		delete(objNewLabels, labelKey)
		obj.SetLabels(objNewLabels)

		if err := client.Update(ctx, obj); err != nil {
			klog.Errorf("Failed to update %v/%v, err:%v", namespace, name, err)
			return err
		}
		return nil
	})
}

func (r *BackupReconciler) addLableToAllResources(ctx context.Context) error {
	var errs []error
	r.Log.Info("Add backup label to resources", "namespace", config.GetMGHNamespacedName().Namespace)
	for k, v := range allResourcesBackup {
		r.Log.V(2).Info("Add label to", "kind", k)
		err := v.AddLabelToAllObjs(ctx, r.Client, config.GetMGHNamespacedName().Namespace)
		if err != nil {
			r.Log.Error(err, "Failed to add backup label", "Type", k)
			errs = append(errs, err)
		}
	}
	return utilerrors.NewAggregate(errs)
}

func (r *BackupReconciler) deleteLableOfAllResources(ctx context.Context) error {
	r.Log.Info("Remove backup label of resources")
	var errs []error
	for k, v := range allResourcesBackup {
		err := v.DeleteLabelOfAllObjs(ctx, r.Client, config.GetMGHNamespacedName().Namespace)
		if err != nil {
			r.Log.Error(err, "Failed to add backup label", "Type", k)
			errs = append(errs, err)
		}
	}
	return utilerrors.NewAggregate(errs)
}

func isBackupEnabled(ctx context.Context, client client.Client) (bool, error) {
	mch, err := utils.ListMCH(ctx, client)
	if err != nil {
		return false, err
	}
	if mch == nil {
		return false, nil
	}
	if mch.Spec.Overrides == nil {
		return false, nil
	}
	for _, c := range mch.Spec.Overrides.Components {
		if c.Name == "cluster-backup" && c.Enabled {
			return true, nil
		}
	}
	return false, nil
}
