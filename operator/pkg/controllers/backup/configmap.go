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
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var configmapList = sets.New(
	constants.CustomAlertName,
)

type configmapBackup struct {
	backupType string
	labelKey   string
	labelValue string
	backupSets sets.Set[string]
}

func NewConfigmapBackup() *configmapBackup {
	return &configmapBackup{
		backupType: configmapType,
		backupSets: configmapList,
		labelKey:   constants.BackupKey,
		labelValue: constants.BackupGlobalHubValue,
	}
}

func (r *configmapBackup) AddLabelToOneObj(ctx context.Context,
	client client.Client,
	namespace, name string,
) error {
	obj := &corev1.ConfigMap{}
	return utils.AddLabel(ctx, client, obj, namespace, name, r.labelKey, r.labelValue)
}

func (r *configmapBackup) AddLabelToAllObjs(ctx context.Context, client client.Client, namespace string) error {
	for name := range configmapList {
		obj := &corev1.ConfigMap{}
		err := utils.AddLabel(ctx, client, obj, namespace, name, r.labelKey, r.labelValue)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *configmapBackup) DeleteLabelOfAllObjs(ctx context.Context, client client.Client, namespace string) error {
	for name := range configmapList {
		obj := &corev1.ConfigMap{}
		err := utils.DeleteLabel(ctx, client, obj, namespace, name, r.labelKey)
		if err != nil {
			return err
		}
	}
	return nil
}
