/*
Copyright 2022.

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

package leafhub

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"

	operatorv1alpha2 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha2"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
)

// +kubebuilder:docs-gen:collapse=Imports

var _ = Describe("LeafHub controller", func() {
	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		MGHNamespace         = "default"
		MGHName              = "test-mgh"
		StorageSecretName    = "storage-secret"
		TransportSecretName  = "transport-secret"
		kafkaCA              = "foobar"
		kafkaBootstrapServer = "https://test-kafka.example.com"

		testingOCPManageClusterName   = "hub1"
		testingHyperManageClusterName = "hyper1"

		timeout  = time.Second * 10
		duration = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When create MGH Instance", func() {
		It("Should create Multicluster Global Hub resources when MGH instance is created", func() {
			By("By creating a new MGH instance")
			ctx := context.Background()

			transportSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      TransportSecretName,
					Namespace: MGHNamespace,
				},
				Data: map[string][]byte{
					"CA":               []byte(kafkaCA),
					"bootstrap_server": []byte(kafkaBootstrapServer),
				},
				Type: corev1.SecretTypeOpaque,
			}
			Expect(k8sClient.Create(ctx, transportSecret)).Should(Succeed())

			mgh := &operatorv1alpha2.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      MGHName,
					Namespace: MGHNamespace,
				},
				Spec: operatorv1alpha2.MulticlusterGlobalHubSpec{
					DataLayer: operatorv1alpha2.DataLayerConfig{
						Type: "largeScale",
						LargeScale: &operatorv1alpha2.LargeScaleConfig{
							Kafka: corev1.LocalObjectReference{
								Name: TransportSecretName,
							},
							Postgres: corev1.LocalObjectReference{
								Name: StorageSecretName,
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, mgh)).Should(Succeed())

			// 	After creating this MGH instance, check that the MGH instance's Spec fields are failed with default values.
			mghLookupKey := types.NamespacedName{Namespace: MGHNamespace, Name: MGHName}
			config.SetHoHMGHNamespacedName(mghLookupKey)
			createdMGH := &operatorv1alpha2.MulticlusterGlobalHub{}

			// get this newly created MGH instance, given that creation may not immediately happen.
			Eventually(func() bool {
				err := k8sClient.Get(ctx, mghLookupKey, createdMGH)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			// make sure the clustermanagementaddon is created
			Eventually(func() bool {
				clusterManagementAddOn := &addonv1alpha1.ClusterManagementAddOn{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name: constants.HoHClusterManagementAddonName,
				}, clusterManagementAddOn)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			// create a testing managedcluster
			By("By creating a new OCP Managed Cluster")
			testingOCPManagedCluster := &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: testingOCPManageClusterName,
					Labels: map[string]string{
						"vendor": "OpenShift",
					},
					Finalizers: []string{"cluster.open-cluster-management.io/api-resource-cleanup"},
				},
			}
			Expect(k8sClient.Create(ctx, testingOCPManagedCluster)).Should(Succeed())

			// update the managedcluster status to be available
			By("By updating the managedcluster status to be available")
			testingOCPManagedCluster.Status.Conditions = []metav1.Condition{
				{
					Type:               "ManagedClusterConditionAvailable",
					Reason:             "ManagedClusterAvailable",
					Message:            "Managed cluster is available",
					Status:             "True",
					LastTransitionTime: metav1.Time{Time: time.Now()},
				},
			}
			Expect(k8sClient.Status().Update(ctx, testingOCPManagedCluster)).Should(Succeed())

			// create managedcluster namespace
			By("By namespace for the managed cluster")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: testingOCPManageClusterName,
				},
			}
			Expect(k8sClient.Create(ctx, namespace)).Should(Succeed())

			// set fake packagemenifestwork information
			By("By setting fake packagemanifest")
			setPackageManifestConfig("release-2.6", "advanced-cluster-management.v2.6.0",
				"stable-2.0", "multicluster-engine.v2.0.1",
				map[string]string{"multiclusterhub-operator": "example.com/registration-operator:test"},
				map[string]string{"registration-operator": "example.com/registration-operator:test"})

			// set fake packagemenifestwork information
			By("By creating a fake imagepull secret")
			imagePullSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.DefaultImagePullSecretName,
					Namespace: MGHNamespace,
				},
				Data: map[string][]byte{
					".dockerconfigjson": []byte("{}"),
				},
				Type: corev1.SecretTypeDockerConfigJson,
			}
			Expect(k8sClient.Create(ctx, imagePullSecret)).Should(Succeed())

			// check the subscription manifestwork is created
			By("By checking the subscription manifestwork is created for the new managed cluster")
			subWork := &workv1.ManifestWork{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name: fmt.Sprintf("%s-%s", testingOCPManageClusterName,
						constants.HOHHubSubscriptionWorkSuffix),
					Namespace: testingOCPManageClusterName,
				}, subWork)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			// set the status of the subscription manifestwork
			By("By setting the status of the subscription manifestwork for the new managed cluster")
			stateValueStr := "AtLatestKnown"
			subWork.Status.ResourceStatus = workv1.ManifestResourceStatus{
				Manifests: []workv1.ManifestCondition{
					{
						ResourceMeta: workv1.ManifestResourceMeta{
							Ordinal:   4,
							Group:     "operators.coreos.com",
							Kind:      "Subscription",
							Name:      "acm-operator-subscription",
							Namespace: "open-cluster-management",
							Resource:  "subscriptions",
							Version:   "v1alpha1",
						},
						StatusFeedbacks: workv1.StatusFeedbackResult{
							Values: []workv1.FeedbackValue{
								{Name: "state", Value: workv1.FieldValue{Type: workv1.String, String: &stateValueStr}},
							},
						},
						Conditions: []metav1.Condition{},
					},
				},
			}
			Expect(k8sClient.Status().Update(ctx, subWork)).Should(Succeed())

			// check the mch manifestwork is created
			By("By checking the mch manifestwork is created for the new managed cluster")
			mchWork := &workv1.ManifestWork{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      fmt.Sprintf("%s-%s", testingOCPManageClusterName, constants.HoHHubMCHWorkSuffix),
					Namespace: testingOCPManageClusterName,
				}, mchWork)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			// set the status of the mch manifestwork
			By("By setting the status of the mch manifestwork for the new managed cluster")
			stateValueStr = "True"
			mchWork.Status.ResourceStatus = workv1.ManifestResourceStatus{
				Manifests: []workv1.ManifestCondition{
					{
						ResourceMeta: workv1.ManifestResourceMeta{
							Ordinal:   0,
							Group:     "operator.open-cluster-management.io",
							Kind:      "MultiClusterHub",
							Name:      "multiclusterhub",
							Namespace: "open-cluster-management",
							Resource:  "multiclusterhubs",
							Version:   "v1",
						},
						StatusFeedbacks: workv1.StatusFeedbackResult{
							Values: []workv1.FeedbackValue{
								{Name: "state", Value: workv1.FieldValue{Type: workv1.String, String: &stateValueStr}},
								{Name: "currentVersion", Value: workv1.FieldValue{Type: workv1.String, String: &stateValueStr}},
								{
									Name:  "multicluster-engine-status",
									Value: workv1.FieldValue{Type: workv1.String, String: &stateValueStr},
								},
								{Name: "grc-sub-status", Value: workv1.FieldValue{Type: workv1.String, String: &stateValueStr}},
							},
						},
						Conditions: []metav1.Condition{},
					},
				},
			}
			Expect(k8sClient.Status().Update(ctx, mchWork)).Should(Succeed())

			// check the agent manifestwork is created
			By("By checking the agent manifestwork is created for the new managed cluster")
			Eventually(func() bool {
				agentWork := &workv1.ManifestWork{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      fmt.Sprintf("%s-%s", testingOCPManageClusterName, constants.HoHAgentWorkSuffix),
					Namespace: testingOCPManageClusterName,
				}, agentWork)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			// check the ManagedClusterAddon is created for the new managed cluster
			By("By checking the ManagedClusterAddon is created for the new managed cluster")
			Eventually(func() bool {
				managedClusterAddon := &addonv1alpha1.ManagedClusterAddOn{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      constants.HoHManagedClusterAddonName,
					Namespace: testingOCPManageClusterName,
				}, managedClusterAddon)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			// delete the testing managedcluster
			By("By deleting a new OCP Managed Cluster")
			Expect(k8sClient.Delete(ctx, testingOCPManagedCluster)).Should(Succeed())

			// checking the ManagedClusterAddon is deleted for the managed cluster
			By("By checking the ManagedClusterAddon is deleted for the managed cluster")
			Eventually(func() bool {
				managedClusterAddon := &addonv1alpha1.ManagedClusterAddOn{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      constants.HoHManagedClusterAddonName,
					Namespace: testingOCPManageClusterName,
				}, managedClusterAddon)
				if errors.IsNotFound(err) {
					return true
				}
				return false
			}, timeout, interval).Should(BeTrue())

			// delete the testing MGH instance
			By("By deleting the testing MGH instance")
			Expect(k8sClient.Delete(ctx, mgh)).Should(Succeed())

			// checking the ClusterManagementAddon is deleted
			By("By checking the ClusterManagementAddon is deleted")
			Eventually(func() bool {
				clusterManagementAddOn := &addonv1alpha1.ClusterManagementAddOn{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name: constants.HoHClusterManagementAddonName,
				}, clusterManagementAddOn)
				if errors.IsNotFound(err) {
					return true
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})
	})
})
