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

package backup_test

import (
	"encoding/json"
	"time"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/condition"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const (
	timeout  = time.Second * 15
	duration = time.Second * 10
	interval = time.Millisecond * 250

	mghName      = "mgh"
	mghNamespace = "default"
	mchName      = "mch"
	mchNamespace = "open-cluster-management"
)

var mghObj = &globalhubv1alpha4.MulticlusterGlobalHub{
	ObjectMeta: metav1.ObjectMeta{
		Name:      mghName,
		Namespace: mghNamespace,
	},
	Spec: globalhubv1alpha4.MulticlusterGlobalHubSpec{},
}

var mchObj = &mchv1.MultiClusterHub{
	ObjectMeta: metav1.ObjectMeta{
		Name:      mchName,
		Namespace: mchNamespace,
	},
	Spec: mchv1.MultiClusterHubSpec{
		Overrides: &mchv1.Overrides{
			Components: []mchv1.ComponentConfig{
				{
					Name:    "cluster-backup",
					Enabled: true,
				},
			},
		},
	},
}

var _ = Describe("Backup controller", Ordered, func() {
	BeforeAll(func() {
		config.SetMGHNamespacedName(types.NamespacedName{
			Namespace: mghNamespace,
			Name:      mghName,
		})
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: mchNamespace,
			},
		})).Should(Succeed())
		Expect(k8sClient.Create(ctx, mchObj)).Should(Succeed())
	})
	BeforeEach(func() {
		// enable backup before each case
		Eventually(func() error {
			existingMch := &mchv1.MultiClusterHub{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Namespace: mchNamespace,
				Name:      mchName,
			}, existingMch, &client.GetOptions{})
			if err != nil {
				return err
			}
			existingMch.Spec = mchObj.Spec
			err = k8sClient.Update(ctx, existingMch)
			return err
		}, timeout, interval).Should(Succeed())
	})
	Context("add backup label to mgh instance", Ordered, func() {
		It("Should create the MGH instance with backup label", func() {
			Expect(k8sClient.Create(ctx, mghObj)).Should(Succeed())
			Eventually(func() bool {
				mgh := &globalhubv1alpha4.MulticlusterGlobalHub{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Namespace: mghNamespace,
					Name:      mghName,
				}, mgh, &client.GetOptions{})).Should(Succeed())
				mch := &mchv1.MultiClusterHub{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Namespace: mchNamespace,
					Name:      mchName,
				}, mch, &client.GetOptions{})).Should(Succeed())
				if !utils.HasLabel(mgh.Labels, constants.BackupKey, constants.BackupActivationValue) {
					return false
				}
				if !meta.IsStatusConditionTrue(mgh.Status.Conditions, condition.CONDITION_TYPE_BACKUP) {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())
		})

		It("update the mgh label, it should be reconciled", func() {
			mgh := &globalhubv1alpha4.MulticlusterGlobalHub{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Namespace: mghNamespace,
				Name:      mghName,
			}, mgh, &client.GetOptions{})).Should(Succeed())
			mgh.Labels = nil
			Expect(k8sClient.Update(ctx, mgh, &client.UpdateOptions{}))

			Eventually(func() bool {
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Namespace: mghNamespace,
					Name:      mghName,
				}, mgh, &client.GetOptions{})).Should(Succeed())
				return utils.HasLabel(mgh.Labels, constants.BackupKey, constants.BackupActivationValue)
			}, timeout, interval).Should(BeTrue())
		})

		It("disable backup, mgh should remove backup label and have disable condition", func() {
			// Disable backup
			disableBackup()
			Eventually(func() bool {
				mgh := &globalhubv1alpha4.MulticlusterGlobalHub{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Namespace: mghNamespace,
					Name:      mghName,
				}, mgh, &client.GetOptions{})).Should(Succeed())

				if utils.HasLabel(mgh.Labels, constants.BackupKey, constants.BackupActivationValue) {
					return false
				}
				if meta.IsStatusConditionTrue(mgh.Status.Conditions, condition.CONDITION_TYPE_BACKUP) {
					return false
				}
				return true
			}, timeout, 3*time.Second).Should(BeTrue())
		})
	})

	Context("add backup label to customize grafana secret", Ordered, func() {
		It("Should create the customize secret with backup label", func() {
			cusSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.CustomGrafanaIniName,
					Namespace: mghNamespace,
				},
			}
			Expect(k8sClient.Create(ctx, cusSecret)).Should(Succeed())

			Eventually(func() bool {
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Namespace: mghNamespace,
					Name:      constants.CustomGrafanaIniName,
				}, cusSecret, &client.GetOptions{})).Should(Succeed())

				return utils.HasLabel(cusSecret.Labels, constants.BackupKey, constants.BackupGlobalHubValue)
			}, timeout, interval).Should(BeTrue())
		})

		It("Disable backup, backup label should be deleted.", func() {
			disableBackup()
			Eventually(func() bool {
				cusSecret := &corev1.Secret{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Namespace: mghNamespace,
					Name:      constants.CustomGrafanaIniName,
				}, cusSecret, &client.GetOptions{})).Should(Succeed())
				return !utils.HasLabel(cusSecret.Labels, constants.BackupKey, constants.BackupGlobalHubValue)
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("add backup label to customize storage secret", Ordered, func() {
		It("Should create the customize secret with backup label", func() {
			cusSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.GHStorageSecretName,
					Namespace: mghNamespace,
				},
			}
			Expect(k8sClient.Create(ctx, cusSecret)).Should(Succeed())

			Eventually(func() bool {
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Namespace: mghNamespace,
					Name:      constants.GHStorageSecretName,
				}, cusSecret, &client.GetOptions{})).Should(Succeed())

				return utils.HasLabel(cusSecret.Labels, constants.BackupKey, constants.BackupGlobalHubValue)
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("add backup label to customize transport secret", Ordered, func() {
		It("Should create the customize secret with backup label", func() {
			cusSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.GHTransportSecretName,
					Namespace: mghNamespace,
				},
			}
			Expect(k8sClient.Create(ctx, cusSecret)).Should(Succeed())

			Eventually(func() bool {
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Namespace: mghNamespace,
					Name:      constants.GHTransportSecretName,
				}, cusSecret, &client.GetOptions{})).Should(Succeed())

				return utils.HasLabel(cusSecret.Labels, constants.BackupKey, constants.BackupGlobalHubValue)
			}, timeout, interval).Should(BeTrue())
		})

		It("update the Secret label, it should be reconciled", func() {
			cusSecret := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Namespace: mghNamespace,
				Name:      constants.GHTransportSecretName,
			}, cusSecret, &client.GetOptions{})).Should(Succeed())

			cusSecret.Labels = nil

			Expect(k8sClient.Update(ctx, cusSecret, &client.UpdateOptions{}))

			Eventually(func() bool {
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Namespace: mghNamespace,
					Name:      constants.GHTransportSecretName,
				}, cusSecret, &client.GetOptions{})).Should(Succeed())
				return utils.HasLabel(cusSecret.Labels, constants.BackupKey, constants.BackupGlobalHubValue)
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("add backup label to customize configmap", Ordered, func() {
		It("Should create the customize configmap with backup label", func() {
			cusConfigmap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.CustomAlertName,
					Namespace: mghNamespace,
				},
			}
			Expect(k8sClient.Create(ctx, cusConfigmap)).Should(Succeed())

			Eventually(func() bool {
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Namespace: mghNamespace,
					Name:      constants.CustomAlertName,
				}, cusConfigmap, &client.GetOptions{})).Should(Succeed())

				return utils.HasLabel(cusConfigmap.Labels, constants.BackupKey, constants.BackupGlobalHubValue)
			}, timeout, interval).Should(BeTrue())
		})

		It("update the configmap backup label, it should be reconciled", func() {
			cusConfigmap := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Namespace: mghNamespace,
				Name:      constants.CustomAlertName,
			}, cusConfigmap, &client.GetOptions{})).Should(Succeed())

			cusConfigmap.Labels = nil

			Expect(k8sClient.Update(ctx, cusConfigmap, &client.UpdateOptions{}))

			Eventually(func() bool {
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Namespace: mghNamespace,
					Name:      constants.CustomAlertName,
				}, cusConfigmap, &client.GetOptions{})).Should(Succeed())
				return utils.HasLabel(cusConfigmap.Labels, constants.BackupKey, constants.BackupGlobalHubValue)
			}, timeout, interval).Should(BeTrue())
		})

		It("Disable backup, backup label should be deleted", func() {
			disableBackup()
			Eventually(func() bool {
				cusConfigmap := &corev1.ConfigMap{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Namespace: mghNamespace,
					Name:      constants.CustomAlertName,
				}, cusConfigmap, &client.GetOptions{})).Should(Succeed())
				return !utils.HasLabel(cusConfigmap.Labels, constants.BackupKey, constants.BackupGlobalHubValue)
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("add backup label to kafka", Ordered, func() {
		It("Should create the kafka with backup label", func() {
			kafka := &kafkav1beta2.Kafka{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kafka",
					Namespace: mghNamespace,
				},
				Spec: &kafkav1beta2.KafkaSpec{
					Kafka: kafkav1beta2.KafkaSpecKafka{
						Replicas: 1,
						Listeners: []kafkav1beta2.KafkaSpecKafkaListenersElem{
							{
								Name: "plain",
								Port: 9092,
								Tls:  false,
								Type: kafkav1beta2.KafkaSpecKafkaListenersElemTypeInternal,
							},
						},
						Storage: kafkav1beta2.KafkaSpecKafkaStorage{
							Type: kafkav1beta2.KafkaSpecKafkaStorageTypeJbod,
						},
					},
					Zookeeper: kafkav1beta2.KafkaSpecZookeeper{
						Replicas: 1,
						Storage: kafkav1beta2.KafkaSpecZookeeperStorage{
							Type: kafkav1beta2.KafkaSpecZookeeperStorageTypePersistentClaim,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, kafka)).Should(Succeed())

			Eventually(func() bool {
				var kafkaPVCLabels map[string]string
				var zookeeperPVCLabels map[string]string

				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Namespace: mghNamespace,
					Name:      "kafka",
				}, kafka, &client.GetOptions{})).Should(Succeed())
				_, err := backupReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: "Kafka",
						Name:      "kafka",
					}})
				if err != nil {
					return false
				}
				if !utils.HasLabel(kafka.Labels, constants.BackupKey, constants.BackupGlobalHubValue) {
					klog.Errorf("kafka do not have backup label:%v", kafka.Labels)
					return false
				}
				if kafka.Spec.Kafka.Template == nil {
					klog.Errorf("kafka spec template is nil:%v", kafka.Spec.Kafka)
					return false
				}
				if kafka.Spec.Kafka.Template.PersistentVolumeClaim == nil {
					klog.Errorf("kafka spec pvc template is nil:%v", kafka.Spec.Kafka.Template)
					return false
				}
				kafkaPVCLabelsJson := kafka.Spec.Kafka.Template.PersistentVolumeClaim.Metadata.Labels

				err = json.Unmarshal(kafkaPVCLabelsJson.Raw, &kafkaPVCLabels)
				if err != nil {
					klog.Errorf("Failed to unmarshal kafkapvc labels, error:%v", err)
					return false
				}
				if !utils.HasLabel(kafkaPVCLabels, constants.BackupExcludeKey, "true") {
					klog.Errorf("kafka pvc do not have excludd label:%v", kafkaPVCLabels)
					return false
				}

				zookeeperPVCLabelsJson := kafka.Spec.Zookeeper.Template.PersistentVolumeClaim.Metadata.Labels

				err = json.Unmarshal(zookeeperPVCLabelsJson.Raw, &zookeeperPVCLabels)
				if err != nil {
					klog.Errorf("Failed to unmarshal kafkapvc labels, error:%v", err)
					return false
				}
				if !utils.HasLabel(zookeeperPVCLabels, constants.BackupExcludeKey, "true") {
					klog.Errorf("kafka zookeeper do not have excludd label:%v", kafkaPVCLabels)
					return false
				}

				return true
			}, timeout, interval).Should(BeTrue())
		})

		It("update the kafka backup label, it should be reconciled", func() {
			kafka := &kafkav1beta2.Kafka{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Namespace: mghNamespace,
				Name:      "kafka",
			}, kafka, &client.GetOptions{})).Should(Succeed())

			kafka.Labels = nil

			Expect(k8sClient.Update(ctx, kafka, &client.UpdateOptions{})).Should(Succeed())

			_, err := backupReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "Kafka",
					Name:      "kafka",
				}})

			Expect(err).Should(BeNil())
			Eventually(func() bool {
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Namespace: mghNamespace,
					Name:      "kafka",
				}, kafka, &client.GetOptions{})).Should(Succeed())
				klog.Infof("kafka labels:%v", kafka.Labels)
				return utils.HasLabel(kafka.Labels, constants.BackupKey, constants.BackupGlobalHubValue)
			}, timeout, interval).Should(BeTrue())
		})

		It("Disable backup, backup label should be deleted from kafka", func() {
			disableBackup()
			Eventually(func() bool {
				kafka := &kafkav1beta2.Kafka{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Namespace: mghNamespace,
					Name:      "kafka",
				}, kafka, &client.GetOptions{})).Should(Succeed())
				return !utils.HasLabel(kafka.Labels, constants.BackupKey, constants.BackupGlobalHubValue)
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("add backup label to kafka user", Ordered, func() {
		It("Should create the kafkauser with backup label", func() {
			kafkaUser := &kafkav1beta2.KafkaUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kafkauser",
					Namespace: mghNamespace,
				},
			}
			Expect(k8sClient.Create(ctx, kafkaUser)).Should(Succeed())

			Eventually(func() bool {
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Namespace: mghNamespace,
					Name:      "kafkauser",
				}, kafkaUser, &client.GetOptions{})).Should(Succeed())

				return utils.HasLabel(kafkaUser.Labels, constants.BackupKey, constants.BackupGlobalHubValue)
			}, timeout, interval).Should(BeTrue())
		})

		It("update the kafka user backup label, it should be reconciled", func() {
			kafkaUser := &kafkav1beta2.KafkaUser{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Namespace: mghNamespace,
				Name:      "kafkauser",
			}, kafkaUser, &client.GetOptions{})).Should(Succeed())

			kafkaUser.Labels = nil

			Expect(k8sClient.Update(ctx, kafkaUser, &client.UpdateOptions{})).Should(Succeed())

			Eventually(func() bool {
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Namespace: mghNamespace,
					Name:      "kafkauser",
				}, kafkaUser, &client.GetOptions{})).Should(Succeed())
				return utils.HasLabel(kafkaUser.Labels, constants.BackupKey, constants.BackupGlobalHubValue)
			}, timeout, interval).Should(BeTrue())
		})

		It("Disable backup, backup label should be deleted from kafkaUser", func() {
			disableBackup()
			Eventually(func() bool {
				kafkaUser := &kafkav1beta2.KafkaUser{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Namespace: mghNamespace,
					Name:      "kafkauser",
				}, kafkaUser, &client.GetOptions{})).Should(Succeed())
				return !utils.HasLabel(kafkaUser.Labels, constants.BackupKey, constants.BackupGlobalHubValue)
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("add backup label to kafka topic", Ordered, func() {
		It("Should create the kafkatopic with backup label", func() {
			kafkaTopic := &kafkav1beta2.KafkaTopic{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kafkatopic",
					Namespace: mghNamespace,
				},
			}
			Expect(k8sClient.Create(ctx, kafkaTopic)).Should(Succeed())

			Eventually(func() bool {
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Namespace: mghNamespace,
					Name:      "kafkatopic",
				}, kafkaTopic, &client.GetOptions{})).Should(Succeed())

				return utils.HasLabel(kafkaTopic.Labels, constants.BackupKey, constants.BackupGlobalHubValue)
			}, timeout, interval).Should(BeTrue())
		})

		It("update the kafka user backup label, it should be reconciled", func() {
			kafkaTopic := &kafkav1beta2.KafkaTopic{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Namespace: mghNamespace,
				Name:      "kafkatopic",
			}, kafkaTopic, &client.GetOptions{})).Should(Succeed())

			kafkaTopic.Labels = nil

			Expect(k8sClient.Update(ctx, kafkaTopic, &client.UpdateOptions{}))

			Eventually(func() bool {
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Namespace: mghNamespace,
					Name:      "kafkatopic",
				}, kafkaTopic, &client.GetOptions{})).Should(Succeed())
				return utils.HasLabel(kafkaTopic.Labels, constants.BackupKey, constants.BackupGlobalHubValue)
			}, timeout, interval).Should(BeTrue())
		})

		It("Disable backup, backup label should be deleted from kafkaUser", func() {
			disableBackup()
			Eventually(func() bool {
				kafkaTopic := &kafkav1beta2.KafkaTopic{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Namespace: mghNamespace,
					Name:      "kafkatopic",
				}, kafkaTopic, &client.GetOptions{})).Should(Succeed())
				return !utils.HasLabel(kafkaTopic.Labels, constants.BackupKey, constants.BackupGlobalHubValue)
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("add backup label to postgres PVC", Ordered, func() {
		It("Should create the PVC with backup label", func() {
			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "postgrespvc",
					Namespace: mghNamespace,
					Labels: map[string]string{
						constants.PostgresPvcLabelKey: constants.PostgresPvcLabelValue,
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("5Gi"),
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pvc)).Should(Succeed())

			Eventually(func() bool {
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Namespace: mghNamespace,
					Name:      "postgrespvc",
				}, pvc, &client.GetOptions{})).Should(Succeed())

				return utils.HasLabel(pvc.Labels, constants.BackupVolumnKey, constants.BackupGlobalHubValue)
			}, timeout, interval).Should(BeTrue())
		})

		It("update the pvc backup label, it should be reconciled", func() {
			pvc := &corev1.PersistentVolumeClaim{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Namespace: mghNamespace,
				Name:      "postgrespvc",
			}, pvc, &client.GetOptions{})).Should(Succeed())

			delete(pvc.Labels, constants.BackupVolumnKey)
			Expect(k8sClient.Update(ctx, pvc, &client.UpdateOptions{}))

			Eventually(func() bool {
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Namespace: mghNamespace,
					Name:      "postgrespvc",
				}, pvc, &client.GetOptions{})).Should(Succeed())
				return utils.HasLabel(pvc.Labels, constants.BackupVolumnKey, constants.BackupGlobalHubValue)
			}, timeout, interval).Should(BeTrue())
		})

		It("Disable backup, backup label should be deleted from pvc", func() {
			disableBackup()
			Eventually(func() bool {
				pvc := &corev1.PersistentVolumeClaim{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Namespace: mghNamespace,
					Name:      "postgrespvc",
				}, pvc, &client.GetOptions{})).Should(Succeed())
				return !utils.HasLabel(pvc.Labels, constants.BackupVolumnKey, constants.BackupGlobalHubValue)
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("add backup label to crd", Ordered, func() {
		It("Should create the crd with backup label", func() {
			crd := &apiextensionsv1.CustomResourceDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kafkas.kafka.strimzi.io",
					Namespace: mghNamespace,
				},
				Spec: apiextensionsv1.CustomResourceDefinitionSpec{
					Group: "kafka.strimzi.io",
					Scope: "Namespaced",
					Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
						{
							Name: "v1",
							Schema: &apiextensionsv1.CustomResourceValidation{
								OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
									Type:        "object",
									Description: "des",
								},
							},
							Served:  true,
							Storage: true,
						},
					},
					Names: apiextensionsv1.CustomResourceDefinitionNames{
						Kind:     "Kafka",
						ListKind: "KafkaList",
						Plural:   "kafkas",
					},
				},
				Status: apiextensionsv1.CustomResourceDefinitionStatus{
					StoredVersions: []string{
						"v1",
					},
				},
			}

			err := k8sClient.Create(ctx, crd)
			if err != nil {
				Expect(errors.IsAlreadyExists(err)).Should(BeTrue())
			}

			Eventually(func() bool {
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Namespace: mghNamespace,
					Name:      "kafkas.kafka.strimzi.io",
				}, crd, &client.GetOptions{})).Should(Succeed())

				return utils.HasLabel(crd.Labels, constants.BackupKey, constants.BackupGlobalHubValue)
			}, timeout, interval).Should(BeTrue())
		})

		It("update the crd backup label, it should be reconciled", func() {
			crd := &apiextensionsv1.CustomResourceDefinition{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Namespace: mghNamespace,
				Name:      "kafkas.kafka.strimzi.io",
			}, crd, &client.GetOptions{})).Should(Succeed())

			delete(crd.Labels, constants.BackupKey)
			Expect(k8sClient.Update(ctx, crd, &client.UpdateOptions{}))

			Eventually(func() bool {
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Namespace: mghNamespace,
					Name:      "kafkas.kafka.strimzi.io",
				}, crd, &client.GetOptions{})).Should(Succeed())
				return utils.HasLabel(crd.Labels, constants.BackupKey, constants.BackupGlobalHubValue)
			}, timeout, interval).Should(BeTrue())
		})

		It("Disable backup, backup label should be deleted from crd", func() {
			disableBackup()
			Eventually(func() bool {
				crd := &apiextensionsv1.CustomResourceDefinition{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Namespace: mghNamespace,
					Name:      "kafkas.kafka.strimzi.io",
				}, crd, &client.GetOptions{})).Should(Succeed())
				return !utils.HasLabel(crd.Labels, constants.BackupKey, constants.BackupGlobalHubValue)
			}, timeout, interval).Should(BeTrue())
		})
	})
})

func disableBackup() {
	Eventually(func() error {
		existingMch := &mchv1.MultiClusterHub{}
		err := k8sClient.Get(ctx, types.NamespacedName{
			Namespace: mchNamespace,
			Name:      mchName,
		}, existingMch, &client.GetOptions{})
		if err != nil {
			return err
		}
		existingMch.Spec.Overrides = nil
		err = k8sClient.Update(ctx, existingMch)
		return err
	}, timeout, interval).Should(Succeed())
}
