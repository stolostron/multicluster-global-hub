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

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

type kafkaUserBackup struct {
	backupType string
	labelKey   string
	labelValue string
	backupSets sets.String
}

func NewKafkaUserBackup() *kafkaUserBackup {
	return &kafkaUserBackup{
		backupType: kafkaUserType,
		labelKey:   constants.BackupKey,
		labelValue: constants.BackupGlobalHubValue,
	}
}

func (r *kafkaUserBackup) AddLabelToOneObj(ctx context.Context,
	client client.Client,
	namespace, name string,
) error {
	obj := &kafkav1beta2.KafkaUser{}
	return utils.AddLabel(ctx, client, obj, namespace, name, r.labelKey, r.labelValue)
}

func (r *kafkaUserBackup) AddLabelToAllObjs(ctx context.Context, c client.Client, namespace string) error {
	objList := &kafkav1beta2.KafkaUserList{}
	err := c.List(ctx, objList, &client.ListOptions{
		Namespace: namespace,
	})
	if err != nil {
		return err
	}
	for _, obj := range objList.Items {
		if utils.HasLabel(obj.GetLabels(), r.labelKey, r.labelValue) {
			continue
		}
		kafkaUser := &kafkav1beta2.KafkaUser{}
		err := utils.AddLabel(ctx, c, kafkaUser, namespace, obj.Name, r.labelKey, r.labelValue)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *kafkaUserBackup) DeleteLabelOfAllObjs(ctx context.Context, c client.Client, namespace string) error {
	objList := &kafkav1beta2.KafkaUserList{}
	err := c.List(ctx, objList, &client.ListOptions{
		Namespace: namespace,
	})
	if err != nil {
		return err
	}
	for _, obj := range objList.Items {
		if !utils.HasLabel(obj.GetLabels(), r.labelKey, r.labelValue) {
			continue
		}
		kafkaUser := &kafkav1beta2.KafkaUser{}
		err := utils.DeleteLabel(ctx, c, kafkaUser, namespace, obj.Name, r.labelKey)
		if err != nil {
			return err
		}
	}
	return nil
}
