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

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

const (
	timeout  = time.Second * 15
	duration = time.Second * 10
	interval = time.Millisecond * 250

	mghName      = "mgh"
	mghNamespace = "default"
)

var _ = Describe("Backup controller", Ordered, func() {
	BeforeAll(func() {
		config.SetMGHNamespacedName(types.NamespacedName{
			Namespace: mghNamespace,
			Name:      mghName,
		})
	})
	Context("add backup label to mgh instance", Ordered, func() {
		It("Should create the MGH instance with backup label", func() {
			mgh := &globalhubv1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      mghName,
					Namespace: mghNamespace,
				},
				Spec: globalhubv1alpha4.MulticlusterGlobalHubSpec{},
			}
			Expect(k8sClient.Create(ctx, mgh)).Should(Succeed())

			Eventually(func() bool {
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Namespace: mghNamespace,
					Name:      mghName,
				}, mgh, &client.GetOptions{})).Should(Succeed())

				return utils.HasLabel(mgh.Labels, constants.BackupKey, constants.BackupActivationValue)
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
	})
	Context("add backup label to kafka", Ordered, func() {
		It("Should create the kafka with backup label", func() {
			kafka := &kafkav1beta2.Kafka{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kafka",
					Namespace: mghNamespace,
				},
			}
			Expect(k8sClient.Create(ctx, kafka)).Should(Succeed())

			Eventually(func() bool {
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Namespace: mghNamespace,
					Name:      "kafka",
				}, kafka, &client.GetOptions{})).Should(Succeed())

				return utils.HasLabel(kafka.Labels, constants.BackupKey, constants.BackupGlobalHubValue)
			}, timeout, interval).Should(BeTrue())
		})

		It("update the kafka backup label, it should be reconciled", func() {
			kafka := &kafkav1beta2.Kafka{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Namespace: mghNamespace,
				Name:      "kafka",
			}, kafka, &client.GetOptions{})).Should(Succeed())

			kafka.Labels = nil

			Expect(k8sClient.Update(ctx, kafka, &client.UpdateOptions{}))

			Eventually(func() bool {
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Namespace: mghNamespace,
					Name:      "kafka",
				}, kafka, &client.GetOptions{})).Should(Succeed())
				return utils.HasLabel(kafka.Labels, constants.BackupKey, constants.BackupGlobalHubValue)
			}, timeout, interval).Should(BeTrue())
		})
	})
})
