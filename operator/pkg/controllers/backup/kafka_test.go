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
	"encoding/json"
	"reflect"
	"testing"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func TestAddBackupLabelToKafkaTemplate(t *testing.T) {
	namespace := "default"
	tests := []struct {
		name          string
		existingKafka *kafkav1beta2.Kafka
		wantUpdate    bool
		wantErr       bool
	}{
		{
			name:          "kafka is nil",
			existingKafka: &kafkav1beta2.Kafka{},
			wantUpdate:    false,
			wantErr:       true,
		},
		{
			name:          "kafka spec is nil",
			existingKafka: &kafkav1beta2.Kafka{},
			wantUpdate:    false,
			wantErr:       true,
		},
		{
			name: "kafka do not have backup label",
			existingKafka: &kafkav1beta2.Kafka{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kafka",
					Namespace: namespace,
				},
				Spec: &kafkav1beta2.KafkaSpec{
					Kafka: kafkav1beta2.KafkaSpecKafka{
						Replicas: 1,
					},
				},
			},
			wantUpdate: true,
			wantErr:    false,
		},
		{
			name: "kafka do not have backup label with template",
			existingKafka: &kafkav1beta2.Kafka{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kafka",
					Namespace: namespace,
				},
				Spec: &kafkav1beta2.KafkaSpec{
					Kafka: kafkav1beta2.KafkaSpecKafka{
						Replicas: 1,
						Template: &kafkav1beta2.KafkaSpecKafkaTemplate{},
					},
				},
			},
			wantUpdate: true,
			wantErr:    false,
		},
		{
			name: "kafka do not have backup label with template",
			existingKafka: &kafkav1beta2.Kafka{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kafka",
					Namespace: namespace,
				},
				Spec: &kafkav1beta2.KafkaSpec{
					Kafka: kafkav1beta2.KafkaSpecKafka{
						Replicas: 1,
						Template: &kafkav1beta2.KafkaSpecKafkaTemplate{
							PersistentVolumeClaim: &kafkav1beta2.KafkaSpecKafkaTemplatePersistentVolumeClaim{},
						},
					},
				},
			},
			wantUpdate: true,
			wantErr:    false,
		},
		{
			name: "kafka do not have backup label with metadata template",
			existingKafka: &kafkav1beta2.Kafka{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kafka",
					Namespace: namespace,
				},
				Spec: &kafkav1beta2.KafkaSpec{
					Kafka: kafkav1beta2.KafkaSpecKafka{
						Replicas: 1,
						Template: &kafkav1beta2.KafkaSpecKafkaTemplate{
							PersistentVolumeClaim: &kafkav1beta2.KafkaSpecKafkaTemplatePersistentVolumeClaim{
								Metadata: &kafkav1beta2.KafkaSpecKafkaTemplatePersistentVolumeClaimMetadata{},
							},
						},
					},
				},
			},
			wantUpdate: true,
			wantErr:    false,
		},
		{
			name: "kafka have other label in template",
			existingKafka: &kafkav1beta2.Kafka{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kafka",
					Namespace: namespace,
				},
				Spec: &kafkav1beta2.KafkaSpec{
					Kafka: kafkav1beta2.KafkaSpecKafka{
						Replicas: 1,
						Template: &kafkav1beta2.KafkaSpecKafkaTemplate{
							PersistentVolumeClaim: &kafkav1beta2.KafkaSpecKafkaTemplatePersistentVolumeClaim{
								Metadata: &kafkav1beta2.KafkaSpecKafkaTemplatePersistentVolumeClaimMetadata{
									Labels: &apiextensions.JSON{
										Raw: []byte(`{
											"existlabel": "value"
										}`),
									},
								},
							},
						},
					},
					Zookeeper: kafkav1beta2.KafkaSpecZookeeper{
						Replicas: 1,
					},
				},
			},
			wantUpdate: true,
			wantErr:    false,
		},
		{
			name: "kafka have other label in template",
			existingKafka: &kafkav1beta2.Kafka{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kafka",
					Namespace: namespace,
				},
				Spec: &kafkav1beta2.KafkaSpec{
					Kafka: kafkav1beta2.KafkaSpecKafka{
						Replicas: 1,
						Template: &kafkav1beta2.KafkaSpecKafkaTemplate{
							PersistentVolumeClaim: &kafkav1beta2.KafkaSpecKafkaTemplatePersistentVolumeClaim{
								Metadata: &kafkav1beta2.KafkaSpecKafkaTemplatePersistentVolumeClaimMetadata{
									Labels: &apiextensions.JSON{
										Raw: []byte(ExcludeBackupLabelRaw),
									},
								},
							},
						},
					},
					Zookeeper: kafkav1beta2.KafkaSpecZookeeper{
						Replicas: 1,
					},
				},
			},
			wantUpdate: false,
			wantErr:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var kafkaPVCLabels map[string]string
			returnedKafka, updated, err := AddExcludeLabelToKafkaTemplate(tt.existingKafka)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Should have error, err: %v", err)
				}
				return
			}

			if !reflect.DeepEqual(updated, tt.wantUpdate) {
				t.Errorf("AddBackupLabelToKafkaTemplate() got = %v, want %v", updated, tt.wantUpdate)
			}
			kafkaPVCLabelsJson := returnedKafka.Spec.Kafka.Template.PersistentVolumeClaim.Metadata.Labels

			err = json.Unmarshal(kafkaPVCLabelsJson.Raw, &kafkaPVCLabels)
			if err != nil {
				t.Errorf("Should not have error, err: %v", err)
			}
			if !utils.HasLabel(kafkaPVCLabels, constants.BackupExcludeKey, "true") {
				t.Errorf("Should not have error, err: %v", err)
			}
		})
	}
}

func TestAddBackupLabelToZookeeperTemplate(t *testing.T) {
	namespace := "default"
	tests := []struct {
		name          string
		existingKafka *kafkav1beta2.Kafka
		wantUpdate    bool
		wantErr       bool
	}{
		{
			name:          "kafka is nil",
			existingKafka: &kafkav1beta2.Kafka{},
			wantUpdate:    false,
			wantErr:       true,
		},
		{
			name:          "kafka spec is nil",
			existingKafka: &kafkav1beta2.Kafka{},
			wantUpdate:    false,
			wantErr:       true,
		},
		{
			name: "kafka do not have backup label",
			existingKafka: &kafkav1beta2.Kafka{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kafka",
					Namespace: namespace,
				},
				Spec: &kafkav1beta2.KafkaSpec{
					Zookeeper: kafkav1beta2.KafkaSpecZookeeper{
						Replicas: 1,
					},
				},
			},
			wantUpdate: true,
			wantErr:    false,
		},
		{
			name: "kafka do not have backup label with template",
			existingKafka: &kafkav1beta2.Kafka{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kafka",
					Namespace: namespace,
				},
				Spec: &kafkav1beta2.KafkaSpec{
					Zookeeper: kafkav1beta2.KafkaSpecZookeeper{
						Replicas: 1,
						Template: &kafkav1beta2.KafkaSpecZookeeperTemplate{},
					},
				},
			},
			wantUpdate: true,
			wantErr:    false,
		},
		{
			name: "kafka do not have backup label with template",
			existingKafka: &kafkav1beta2.Kafka{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kafka",
					Namespace: namespace,
				},
				Spec: &kafkav1beta2.KafkaSpec{
					Zookeeper: kafkav1beta2.KafkaSpecZookeeper{
						Replicas: 1,
						Template: &kafkav1beta2.KafkaSpecZookeeperTemplate{
							PersistentVolumeClaim: &kafkav1beta2.KafkaSpecZookeeperTemplatePersistentVolumeClaim{},
						},
					},
				},
			},
			wantUpdate: true,
			wantErr:    false,
		},
		{
			name: "kafka do not have backup label with metadata template",
			existingKafka: &kafkav1beta2.Kafka{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kafka",
					Namespace: namespace,
				},
				Spec: &kafkav1beta2.KafkaSpec{
					Zookeeper: kafkav1beta2.KafkaSpecZookeeper{
						Replicas: 1,
						Template: &kafkav1beta2.KafkaSpecZookeeperTemplate{
							PersistentVolumeClaim: &kafkav1beta2.KafkaSpecZookeeperTemplatePersistentVolumeClaim{
								Metadata: &kafkav1beta2.KafkaSpecZookeeperTemplatePersistentVolumeClaimMetadata{},
							},
						},
					},
				},
			},
			wantUpdate: true,
			wantErr:    false,
		},
		{
			name: "kafka have other label in template",
			existingKafka: &kafkav1beta2.Kafka{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kafka",
					Namespace: namespace,
				},
				Spec: &kafkav1beta2.KafkaSpec{
					Zookeeper: kafkav1beta2.KafkaSpecZookeeper{
						Replicas: 1,
						Template: &kafkav1beta2.KafkaSpecZookeeperTemplate{
							PersistentVolumeClaim: &kafkav1beta2.KafkaSpecZookeeperTemplatePersistentVolumeClaim{
								Metadata: &kafkav1beta2.KafkaSpecZookeeperTemplatePersistentVolumeClaimMetadata{
									Labels: &apiextensions.JSON{
										Raw: []byte(`{
											"existlabel": "value"
										}`),
									},
								},
							},
						},
					},
				},
			},
			wantUpdate: true,
			wantErr:    false,
		},
		{
			name: "kafka have other label in template",
			existingKafka: &kafkav1beta2.Kafka{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kafka",
					Namespace: namespace,
				},
				Spec: &kafkav1beta2.KafkaSpec{
					Zookeeper: kafkav1beta2.KafkaSpecZookeeper{
						Replicas: 1,
						Template: &kafkav1beta2.KafkaSpecZookeeperTemplate{
							PersistentVolumeClaim: &kafkav1beta2.KafkaSpecZookeeperTemplatePersistentVolumeClaim{
								Metadata: &kafkav1beta2.KafkaSpecZookeeperTemplatePersistentVolumeClaimMetadata{
									Labels: &apiextensions.JSON{
										Raw: []byte(ExcludeBackupLabelRaw),
									},
								},
							},
						},
					},
				},
			},
			wantUpdate: false,
			wantErr:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var kafkaPVCLabels map[string]string
			returnedKafka, updated, err := AddExcludeLabelToZookeeperTemplate(tt.existingKafka)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Should have error, err: %v", err)
				}
				return
			}

			if !reflect.DeepEqual(updated, tt.wantUpdate) {
				t.Errorf("AddBackupLabelToZookeeperTemplate() got = %v, want %v", updated, tt.wantUpdate)
			}
			kafkaPVCLabelsJson := returnedKafka.Spec.Zookeeper.Template.PersistentVolumeClaim.Metadata.Labels

			err = json.Unmarshal(kafkaPVCLabelsJson.Raw, &kafkaPVCLabels)
			if err != nil {
				t.Errorf("Should not have error, err: %v", err)
			}
			if !utils.HasLabel(kafkaPVCLabels, constants.BackupExcludeKey, "true") {
				t.Errorf("Should not have error, err: %v", err)
			}
		})
	}
}

func TestAddBackupLabelToOperatorDeployTemplate(t *testing.T) {
	namespace := "default"
	tests := []struct {
		name          string
		existingKafka *kafkav1beta2.Kafka
		wantUpdate    bool
		wantErr       bool
	}{
		{
			name:          "kafka is nil",
			existingKafka: &kafkav1beta2.Kafka{},
			wantUpdate:    false,
			wantErr:       true,
		},
		{
			name:          "kafka spec is nil",
			existingKafka: &kafkav1beta2.Kafka{},
			wantUpdate:    false,
			wantErr:       true,
		},
		{
			name: "kafka do not have backup label",
			existingKafka: &kafkav1beta2.Kafka{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kafka",
					Namespace: namespace,
				},
				Spec: &kafkav1beta2.KafkaSpec{
					EntityOperator: &kafkav1beta2.KafkaSpecEntityOperator{},
				},
			},
			wantUpdate: true,
			wantErr:    false,
		},
		{
			name: "kafka do not have backup label with template",
			existingKafka: &kafkav1beta2.Kafka{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kafka",
					Namespace: namespace,
				},
				Spec: &kafkav1beta2.KafkaSpec{
					EntityOperator: &kafkav1beta2.KafkaSpecEntityOperator{
						Template: &kafkav1beta2.KafkaSpecEntityOperatorTemplate{},
					},
				},
			},
			wantUpdate: true,
			wantErr:    false,
		},
		{
			name: "kafka do not have backup label with metadata template",
			existingKafka: &kafkav1beta2.Kafka{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kafka",
					Namespace: namespace,
				},
				Spec: &kafkav1beta2.KafkaSpec{
					EntityOperator: &kafkav1beta2.KafkaSpecEntityOperator{
						Template: &kafkav1beta2.KafkaSpecEntityOperatorTemplate{
							Deployment: &kafkav1beta2.KafkaSpecEntityOperatorTemplateDeployment{
								Metadata: &kafkav1beta2.KafkaSpecEntityOperatorTemplateDeploymentMetadata{},
							},
						},
					},
				},
			},
			wantUpdate: true,
			wantErr:    false,
		},
		{
			name: "kafka have exclude labels in template",
			existingKafka: &kafkav1beta2.Kafka{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kafka",
					Namespace: namespace,
				},
				Spec: &kafkav1beta2.KafkaSpec{
					EntityOperator: &kafkav1beta2.KafkaSpecEntityOperator{
						Template: &kafkav1beta2.KafkaSpecEntityOperatorTemplate{
							Deployment: &kafkav1beta2.KafkaSpecEntityOperatorTemplateDeployment{
								Metadata: &kafkav1beta2.KafkaSpecEntityOperatorTemplateDeploymentMetadata{
									Labels: &apiextensions.JSON{
										Raw: []byte(ExcludeBackupLabelRaw),
									},
								},
							},
						},
					},
				},
			},
			wantUpdate: false,
			wantErr:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var kafkaOperatorLabels map[string]string
			returnedKafka, updated, err := AddExcludeLabelToEntityOperatorDeploymentTemplate(tt.existingKafka)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Should have error, err: %v", err)
				}
				return
			}

			if !reflect.DeepEqual(updated, tt.wantUpdate) {
				t.Errorf("AddExcludeLabelToEntityOperatorDeploymentTemplate() got = %v, want %v", updated, tt.wantUpdate)
			}
			operatorLabelsJson := returnedKafka.Spec.EntityOperator.Template.Deployment.Metadata.Labels

			err = json.Unmarshal(operatorLabelsJson.Raw, &kafkaOperatorLabels)
			if err != nil {
				t.Errorf("Should not have error, err: %v", err)
			}
			if !utils.HasLabel(kafkaOperatorLabels, constants.BackupExcludeKey, "true") {
				t.Errorf("Should have exclude label, labels: %v", kafkaOperatorLabels)
			}
		})
	}
}
