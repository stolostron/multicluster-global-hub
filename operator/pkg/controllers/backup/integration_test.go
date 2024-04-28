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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
				if !utils.HasItem(mgh.Labels, constants.BackupKey, constants.BackupActivationValue) {
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
			Expect(k8sClient.Update(ctx, mgh, &client.UpdateOptions{}))

			Eventually(func() bool {
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Namespace: mghNamespace,
					Name:      mghName,
				}, mgh, &client.GetOptions{})).Should(Succeed())
				return utils.HasItem(mgh.Labels, constants.BackupKey, constants.BackupActivationValue)
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

				if utils.HasItem(mgh.Labels, constants.BackupKey, constants.BackupActivationValue) {
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

				return utils.HasItem(cusSecret.Labels, constants.BackupKey, constants.BackupGlobalHubValue)
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
				return !utils.HasItem(cusSecret.Labels, constants.BackupKey, constants.BackupGlobalHubValue)
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

				return utils.HasItem(cusSecret.Labels, constants.BackupKey, constants.BackupGlobalHubValue)
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

				return utils.HasItem(cusSecret.Labels, constants.BackupKey, constants.BackupGlobalHubValue)
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
				return utils.HasItem(cusSecret.Labels, constants.BackupKey, constants.BackupGlobalHubValue)
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

				return utils.HasItem(cusConfigmap.Labels, constants.BackupKey, constants.BackupGlobalHubValue)
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
				return utils.HasItem(cusConfigmap.Labels, constants.BackupKey, constants.BackupGlobalHubValue)
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
				return !utils.HasItem(cusConfigmap.Labels, constants.BackupKey, constants.BackupGlobalHubValue)
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
					Resources: corev1.VolumeResourceRequirements{
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

				return utils.HasItem(pvc.Labels, constants.BackupVolumnKey, constants.BackupGlobalHubValue)
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
				if !utils.HasItem(pvc.Labels, constants.BackupVolumnKey, constants.BackupGlobalHubValue) {
					return false
				}
				if !utils.HasItem(pvc.Annotations, constants.BackupPvcUserCopyTrigger, constants.BackupGlobalHubValue) {
					return false
				}
				return true
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
				return !utils.HasItem(pvc.Labels, constants.BackupVolumnKey, constants.BackupGlobalHubValue)
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
