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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

type pvcBackup struct {
	backupType string
	labelKey   string
	labelValue string
	prehookKey string
}

func NewPvcBackup() *pvcBackup {
	return &pvcBackup{
		backupType: pvcType,
		labelKey:   constants.BackupVolumnKey,
		labelValue: constants.BackupGlobalHubValue,
		prehookKey: constants.BackupPvcUserCopyTrigger,
	}
}

func (r *pvcBackup) AddLabelToOneObj(ctx context.Context,
	client client.Client,
	namespace, name string,
) error {
	obj := &corev1.PersistentVolumeClaim{}
	err := client.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}, obj)
	if err != nil {
		return err
	}

	if !utils.HasLabel(obj.GetLabels(), r.prehookKey, r.labelValue) {
		err := utils.AddLabel(ctx, client, obj, namespace, name, r.prehookKey, r.labelValue)
		if err != nil {
			return err
		}
	}
	return utils.AddLabel(ctx, client, obj, namespace, name, r.labelKey, r.labelValue)
}

func (r *pvcBackup) AddLabelToAllObjs(ctx context.Context, c client.Client, namespace string) error {
	postgresList := &corev1.PersistentVolumeClaimList{}
	err := c.List(ctx, postgresList, &client.ListOptions{
		Namespace: namespace,
		LabelSelector: labels.SelectorFromSet(
			labels.Set{
				constants.PostgresPvcLabelKey: constants.PostgresPvcLabelValue,
			},
		),
	})
	if err != nil {
		return err
	}

	var objs []corev1.PersistentVolumeClaim
	objs = append(objs, postgresList.Items...)

	for _, obj := range objs {
		pvc := &corev1.PersistentVolumeClaim{}
		if !utils.HasLabel(obj.GetLabels(), r.prehookKey, r.labelValue) {
			err := utils.AddLabel(ctx, c, pvc, namespace, obj.Name, r.prehookKey, r.labelValue)
			if err != nil {
				return err
			}
		}

		if !utils.HasLabel(obj.GetLabels(), r.labelKey, r.labelValue) {
			err = utils.AddLabel(ctx, c, pvc, namespace, obj.Name, r.labelKey, r.labelValue)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *pvcBackup) DeleteLabelOfAllObjs(ctx context.Context, c client.Client, namespace string) error {
	postgresList := &corev1.PersistentVolumeClaimList{}
	err := c.List(ctx, postgresList, &client.ListOptions{
		Namespace: namespace,
		LabelSelector: labels.SelectorFromSet(
			labels.Set{
				constants.PostgresPvcLabelKey: constants.PostgresPvcLabelValue,
				r.labelKey:                    r.labelValue,
			},
		),
	})
	if err != nil {
		return err
	}

	var objs []corev1.PersistentVolumeClaim
	objs = append(objs, postgresList.Items...)

	for _, obj := range objs {
		pvc := &corev1.PersistentVolumeClaim{}
		if utils.HasLabelKey(obj.GetLabels(), r.prehookKey) {
			err := utils.DeleteLabel(ctx, c, pvc, namespace, obj.Name, r.labelKey)
			if err != nil {
				return err
			}
		}
		if utils.HasLabel(obj.GetLabels(), r.labelKey, r.labelValue) {
			err := utils.DeleteLabel(ctx, c, pvc, namespace, obj.Name, r.labelKey)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
