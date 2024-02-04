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
	"encoding/json"
	"fmt"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const (
	ExcludeBackupLabelRaw = `{
		"velero.io/exclude-from-backup": "true"
	}`
)

type kafkaBackup struct {
	backupType string
	labelKey   string
	labelValue string
	backupSets sets.String
}

func NewKafkaBackup() *kafkaBackup {
	return &kafkaBackup{
		backupType: kafkaType,
		labelKey:   constants.BackupKey,
		labelValue: constants.BackupGlobalHubValue,
	}
}

func (r *kafkaBackup) AddLabelToOneObj(ctx context.Context,
	client client.Client,
	namespace, name string,
) error {
	obj := &kafkav1beta2.Kafka{}
	err := utils.AddLabel(ctx, client, obj, namespace, name, r.labelKey, r.labelValue)
	if err != nil {
		return err
	}
	err = AddBackupLabelToTemplate(ctx, client, namespace, name)
	return err
}

// AddTemplateBackupLabels add backup label to kafka pvc template
func AddBackupLabelToTemplate(ctx context.Context,
	client client.Client,
	namespace, name string,
) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		existingKafka := &kafkav1beta2.Kafka{}
		err := client.Get(ctx, types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		}, existingKafka)
		if err != nil {
			return err
		}

		updatedKafka, updateBackupLabelInKafkaPVC, err := AddExcludeLabelToKafkaTemplate(existingKafka)
		if err != nil {
			return err
		}

		updatedKafka, updateBackupLabelInZookeeperPVC, err := AddExcludeLabelToZookeeperTemplate(updatedKafka)
		if err != nil {
			return err
		}

		updatedKafka, updateExcludLabelInOperatorDeploy, err := AddExcludeLabelToEntityOperatorDeploymentTemplate(
			updatedKafka)
		if err != nil {
			return err
		}

		if !updateBackupLabelInZookeeperPVC &&
			!updateBackupLabelInKafkaPVC &&
			!updateExcludLabelInOperatorDeploy {
			return nil
		}

		err = client.Update(ctx, updatedKafka)
		if err != nil {
			return err
		}
		return nil
	})
}

func AddExcludeLabelToEntityOperatorDeploymentTemplate(existingKafka *kafkav1beta2.Kafka) (
	*kafkav1beta2.Kafka,
	bool,
	error,
) {
	var operatorLabels map[string]string
	if existingKafka == nil || existingKafka.Spec == nil {
		return nil, false, fmt.Errorf("kafka spec should not be nil")
	}
	desiredOperator := &kafkav1beta2.KafkaSpecEntityOperator{
		Template: &kafkav1beta2.KafkaSpecEntityOperatorTemplate{
			Deployment: &kafkav1beta2.KafkaSpecEntityOperatorTemplateDeployment{
				Metadata: &kafkav1beta2.KafkaSpecEntityOperatorTemplateDeploymentMetadata{
					Labels: &apiextensions.JSON{
						Raw: []byte(ExcludeBackupLabelRaw),
					},
				},
			},
		},
	}

	updatedKafka := existingKafka.DeepCopy()

	if existingKafka.Spec.EntityOperator == nil {
		updatedKafka.Spec.EntityOperator = desiredOperator
		return updatedKafka, true, nil
	}

	if existingKafka.Spec.EntityOperator.Template == nil {
		updatedKafka.Spec.EntityOperator.Template = desiredOperator.Template
		return updatedKafka, true, nil
	}

	if existingKafka.Spec.EntityOperator.Template.Deployment == nil {
		updatedKafka.Spec.EntityOperator.Template.Deployment = desiredOperator.Template.Deployment
		return updatedKafka, true, nil
	}

	if existingKafka.Spec.EntityOperator.Template.Deployment.Metadata == nil {
		updatedKafka.Spec.EntityOperator.Template.Deployment.Metadata = desiredOperator.Template.Deployment.Metadata
		return updatedKafka, true, nil
	}

	if existingKafka.Spec.EntityOperator.Template.Deployment.Metadata.Labels == nil {
		updatedKafka.Spec.EntityOperator.Template.Deployment.Metadata.Labels = desiredOperator.Template.Deployment.Metadata.Labels
		return updatedKafka, true, nil
	}

	operatorLabelsJson := existingKafka.Spec.EntityOperator.Template.Deployment.Metadata.Labels

	err := json.Unmarshal(operatorLabelsJson.Raw, &operatorLabels)
	if err != nil {
		return nil, true, err
	}
	if utils.HasLabel(operatorLabels, constants.BackupExcludeKey, "true") {
		return updatedKafka, false, nil
	}

	if operatorLabels == nil {
		operatorLabels = make(map[string]string)
	}

	operatorLabels[constants.BackupExcludeKey] = "true"

	operatorLabelJSON, err := json.Marshal(operatorLabels)
	if err != nil {
		return nil, true, err
	}
	updatedKafka.Spec.EntityOperator.Template.Deployment.Metadata.Labels = &apiextensions.JSON{
		Raw: operatorLabelJSON,
	}
	return updatedKafka, true, nil
}

func AddExcludeLabelToKafkaTemplate(existingKafka *kafkav1beta2.Kafka) (*kafkav1beta2.Kafka, bool, error) {
	var kafkaPVCLabels map[string]string
	updatedKafka := existingKafka.DeepCopy()
	if existingKafka == nil || existingKafka.Spec == nil {
		return nil, false, fmt.Errorf("kafka spec should not be nil, name: %s}", existingKafka.Name)
	}

	desiredTemplate := &kafkav1beta2.KafkaSpecKafkaTemplate{
		PersistentVolumeClaim: &kafkav1beta2.KafkaSpecKafkaTemplatePersistentVolumeClaim{
			Metadata: &kafkav1beta2.KafkaSpecKafkaTemplatePersistentVolumeClaimMetadata{
				Labels: &apiextensions.JSON{
					Raw: []byte(ExcludeBackupLabelRaw),
				},
			},
		},
	}
	if existingKafka.Spec.Kafka.Template == nil {
		updatedKafka.Spec.Kafka.Template = desiredTemplate
		return updatedKafka, true, nil
	}

	if existingKafka.Spec.Kafka.Template.PersistentVolumeClaim == nil {
		updatedKafka.Spec.Kafka.Template.PersistentVolumeClaim = desiredTemplate.PersistentVolumeClaim
		return updatedKafka, true, nil
	}

	if existingKafka.Spec.Kafka.Template.PersistentVolumeClaim.Metadata == nil {
		updatedKafka.Spec.Kafka.Template.PersistentVolumeClaim.Metadata = desiredTemplate.PersistentVolumeClaim.Metadata
		return updatedKafka, true, nil
	}

	if existingKafka.Spec.Kafka.Template.PersistentVolumeClaim.Metadata.Labels == nil {
		updatedKafka.Spec.Kafka.Template.PersistentVolumeClaim.Metadata.Labels = desiredTemplate.PersistentVolumeClaim.Metadata.Labels
		return updatedKafka, true, nil
	}

	kafkaPVCLabelsJson := existingKafka.Spec.Kafka.Template.PersistentVolumeClaim.Metadata.Labels

	err := json.Unmarshal(kafkaPVCLabelsJson.Raw, &kafkaPVCLabels)
	if err != nil {
		return nil, true, err
	}
	if utils.HasLabel(kafkaPVCLabels, constants.BackupExcludeKey, "true") {
		return updatedKafka, false, nil
	}

	if kafkaPVCLabels == nil {
		kafkaPVCLabels = make(map[string]string)
	}

	kafkaPVCLabels[constants.BackupExcludeKey] = "true"

	kafkaLabelJSON, err := json.Marshal(kafkaPVCLabels)
	if err != nil {
		return nil, true, err
	}
	updatedKafka.Spec.Kafka.Template.PersistentVolumeClaim.Metadata.Labels = &apiextensions.JSON{
		Raw: kafkaLabelJSON,
	}
	return updatedKafka, true, nil
}

func AddExcludeLabelToZookeeperTemplate(existingKafka *kafkav1beta2.Kafka) (*kafkav1beta2.Kafka, bool, error) {
	var zookeeperPVCLabels map[string]string
	if existingKafka == nil || existingKafka.Spec == nil {
		return nil, false, fmt.Errorf("kafka spec should not be nil")
	}
	desiredTemplate := &kafkav1beta2.KafkaSpecZookeeperTemplate{
		PersistentVolumeClaim: &kafkav1beta2.KafkaSpecZookeeperTemplatePersistentVolumeClaim{
			Metadata: &kafkav1beta2.KafkaSpecZookeeperTemplatePersistentVolumeClaimMetadata{
				Labels: &apiextensions.JSON{
					Raw: []byte(ExcludeBackupLabelRaw),
				},
			},
		},
	}

	updatedKafka := existingKafka.DeepCopy()

	if existingKafka.Spec.Zookeeper.Template == nil {
		updatedKafka.Spec.Zookeeper.Template = desiredTemplate
		return updatedKafka, true, nil
	}

	if existingKafka.Spec.Zookeeper.Template.PersistentVolumeClaim == nil {
		updatedKafka.Spec.Zookeeper.Template.PersistentVolumeClaim = desiredTemplate.PersistentVolumeClaim
		return updatedKafka, true, nil
	}

	if existingKafka.Spec.Zookeeper.Template.PersistentVolumeClaim.Metadata == nil {
		updatedKafka.Spec.Zookeeper.Template.PersistentVolumeClaim.Metadata = desiredTemplate.PersistentVolumeClaim.Metadata
		return updatedKafka, true, nil
	}

	if existingKafka.Spec.Zookeeper.Template.PersistentVolumeClaim.Metadata.Labels == nil {
		updatedKafka.Spec.Zookeeper.Template.PersistentVolumeClaim.Metadata.Labels = desiredTemplate.PersistentVolumeClaim.Metadata.Labels
		return updatedKafka, true, nil
	}

	zookeeperPVCLabelsJson := existingKafka.Spec.Zookeeper.Template.PersistentVolumeClaim.Metadata.Labels

	err := json.Unmarshal(zookeeperPVCLabelsJson.Raw, &zookeeperPVCLabels)
	if err != nil {
		return nil, true, err
	}
	if utils.HasLabel(zookeeperPVCLabels, constants.BackupExcludeKey, "true") {
		return updatedKafka, false, nil
	}

	if zookeeperPVCLabels == nil {
		zookeeperPVCLabels = make(map[string]string)
	}

	zookeeperPVCLabels[constants.BackupExcludeKey] = "true"

	zookeeperLabelJSON, err := json.Marshal(zookeeperPVCLabels)
	if err != nil {
		return nil, true, err
	}
	updatedKafka.Spec.Zookeeper.Template.PersistentVolumeClaim.Metadata.Labels = &apiextensions.JSON{
		Raw: zookeeperLabelJSON,
	}
	return updatedKafka, true, nil
}

func DeleteExcludeLabelToKafkaTemplate(existingKafka *kafkav1beta2.Kafka) (*kafkav1beta2.Kafka, bool, error) {
	var kafkaPVCLabels map[string]string

	updatedKafka := existingKafka.DeepCopy()
	if existingKafka.Spec == nil {
		return updatedKafka, false, nil
	}

	if existingKafka.Spec.Kafka.Template == nil ||
		existingKafka.Spec.Kafka.Template.PersistentVolumeClaim == nil ||
		existingKafka.Spec.Kafka.Template.PersistentVolumeClaim.Metadata == nil ||
		existingKafka.Spec.Kafka.Template.PersistentVolumeClaim.Metadata.Labels == nil {
		return updatedKafka, false, nil
	}

	kafkaPVCLabelsJson := existingKafka.Spec.Kafka.Template.PersistentVolumeClaim.Metadata.Labels
	err := json.Unmarshal(kafkaPVCLabelsJson.Raw, &kafkaPVCLabels)
	if err != nil {
		return updatedKafka, false, err
	}
	if !utils.HasLabel(kafkaPVCLabels, constants.BackupExcludeKey, "true") {
		return updatedKafka, false, nil
	}

	delete(kafkaPVCLabels, constants.BackupExcludeKey)
	kafkaLabelJSON, err := json.Marshal(kafkaPVCLabels)
	if err != nil {
		return updatedKafka, false, err
	}
	updatedKafka.Spec.Kafka.Template.PersistentVolumeClaim.Metadata.Labels = &apiextensions.JSON{
		Raw: kafkaLabelJSON,
	}
	return updatedKafka, true, err
}

func DeleteExcludeLabelToZookeeperTemplate(existingKafka *kafkav1beta2.Kafka) (*kafkav1beta2.Kafka, bool, error) {
	var zookeeperPVCLabels map[string]string

	updatedKafka := existingKafka.DeepCopy()
	if existingKafka.Spec == nil {
		return updatedKafka, false, nil
	}

	if existingKafka.Spec.Zookeeper.Template == nil ||
		existingKafka.Spec.Zookeeper.Template.PersistentVolumeClaim == nil ||
		existingKafka.Spec.Zookeeper.Template.PersistentVolumeClaim.Metadata == nil ||
		existingKafka.Spec.Zookeeper.Template.PersistentVolumeClaim.Metadata.Labels == nil {
		return updatedKafka, false, nil
	}

	zookeeperPVCLabelsJson := existingKafka.Spec.Zookeeper.Template.PersistentVolumeClaim.Metadata.Labels
	err := json.Unmarshal(zookeeperPVCLabelsJson.Raw, &zookeeperPVCLabels)
	if err != nil {
		return updatedKafka, false, err
	}
	if !utils.HasLabel(zookeeperPVCLabels, constants.BackupExcludeKey, "true") {
		return updatedKafka, false, nil
	}

	delete(zookeeperPVCLabels, constants.BackupExcludeKey)
	zookeeperLabelJSON, err := json.Marshal(zookeeperPVCLabels)
	if err != nil {
		return updatedKafka, false, err
	}
	updatedKafka.Spec.Zookeeper.Template.PersistentVolumeClaim.Metadata.Labels = &apiextensions.JSON{
		Raw: zookeeperLabelJSON,
	}
	return updatedKafka, true, err
}

// DeleteTemplateBackupLabels delete backup label to kafka pvc template
func DeleteTemplateBackupLabels(ctx context.Context,
	client client.Client,
	namespace, name string,
) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		existingKafka := &kafkav1beta2.Kafka{}

		err := client.Get(ctx, types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		}, existingKafka)
		if err != nil {
			return err
		}

		updatedKafka, updateBackupLabelInKafkaPVC, err := DeleteExcludeLabelToKafkaTemplate(existingKafka)
		if err != nil {
			return err
		}

		updatedKafka, updateBackupLabelInZookeeperPVC, err := DeleteExcludeLabelToZookeeperTemplate(updatedKafka)
		if err != nil {
			return err
		}

		if !updateBackupLabelInZookeeperPVC && !updateBackupLabelInKafkaPVC {
			return nil
		}

		err = client.Update(ctx, updatedKafka)
		if err != nil {
			return err
		}
		return nil
	})
}

func (r *kafkaBackup) AddLabelToAllObjs(ctx context.Context, c client.Client, namespace string) error {
	objList := &kafkav1beta2.KafkaList{}
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
		kafka := &kafkav1beta2.Kafka{}
		err := utils.AddLabel(ctx, c, kafka, namespace, obj.Name, r.labelKey, r.labelValue)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *kafkaBackup) DeleteLabelOfAllObjs(ctx context.Context, c client.Client, namespace string) error {
	objList := &kafkav1beta2.KafkaList{}
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
		kafka := &kafkav1beta2.Kafka{}
		err := utils.DeleteLabel(ctx, c, kafka, namespace, obj.Name, r.labelKey)
		if err != nil {
			return err
		}
	}
	return nil
}
