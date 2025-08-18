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

	"sigs.k8s.io/controller-runtime/pkg/client"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

type mghBackup struct {
	backupType string
	labelKey   string
	labelValue string
}

func NewMghBackup() *mghBackup {
	return &mghBackup{
		backupType: mghType,
		labelKey:   constants.BackupKey,
		labelValue: constants.BackupActivationValue,
	}
}

func (r *mghBackup) AddLabelToOneObj(ctx context.Context,
	client client.Client,
	namespace, name string,
) error {
	obj := &globalhubv1alpha4.MulticlusterGlobalHub{}
	return utils.AddLabel(ctx, client, obj, namespace, name, r.labelKey, r.labelValue)
}

func (r *mghBackup) AddLabelToAllObjs(ctx context.Context, c client.Client, namespace string) error {
	mghList := &globalhubv1alpha4.MulticlusterGlobalHubList{}
	err := c.List(ctx, mghList, &client.ListOptions{
		Namespace: namespace,
	})
	if err != nil {
		return err
	}
	for _, mgh := range mghList.Items {
		if utils.HasItem(mgh.GetLabels(), r.labelKey, r.labelValue) {
			continue
		}
		obj := &globalhubv1alpha4.MulticlusterGlobalHub{}
		err := utils.AddLabel(ctx, c, obj, namespace, mgh.Name, r.labelKey, r.labelValue)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *mghBackup) DeleteLabelOfAllObjs(ctx context.Context, c client.Client, namespace string) error {
	mghList := &globalhubv1alpha4.MulticlusterGlobalHubList{}
	err := c.List(ctx, mghList, &client.ListOptions{
		Namespace: namespace,
	})
	if err != nil {
		return err
	}
	for _, mgh := range mghList.Items {
		if !utils.HasItem(mgh.GetLabels(), r.labelKey, r.labelValue) {
			continue
		}
		obj := &globalhubv1alpha4.MulticlusterGlobalHub{}
		err := utils.DeleteLabel(ctx, c, obj, namespace, mgh.Name, r.labelKey)
		if err != nil {
			return err
		}
	}
	return nil
}
