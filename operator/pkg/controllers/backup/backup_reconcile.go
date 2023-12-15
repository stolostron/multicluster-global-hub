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

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
var firstTime = true

// As we need to watch mgh, secret, configmap. they should be in the same namespace.
// So for request.Namespace, we set it as request type, like "Secret","Configmap","MulticlusterGlobalHub" and so on.
// In the reconcile, we identy the request kind and get it by request.Name.
func (r *BackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	//Add label to all resources in first reconcile
	if firstTime {
		r.AddLableToAllResources(ctx)
		firstTime = false
	}
	r.Log.Info("backup reconcile:", "requestType", req.Namespace, "name", req.Name)
	err := allResourcesBackup[req.Namespace].AddLabelToOneObj(
		ctx, r.Client,
		config.GetMGHNamespacedName().Namespace, req.Name)
	return ctrl.Result{}, err
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

func (r *BackupReconciler) AddLableToAllResources(ctx context.Context) {
	r.Log.Info("Add backup label to resources", "namespace", config.GetMGHNamespacedName().Namespace)
	for k, v := range allResourcesBackup {
		r.Log.V(2).Info("Add label to", "kind", k)
		err := v.AddLabelToAllObjs(ctx, r.Client, config.GetMGHNamespacedName().Namespace)
		if err != nil {
			r.Log.Error(err, "Failed to add backup label", "Type", k)
		}
	}
}
