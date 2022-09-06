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
	hypershiftdeploymentv1alpha1 "github.com/stolostron/hypershift-deployment-controller/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorv1alpha2 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha2"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	commonconstants "github.com/stolostron/multicluster-global-hub/pkg/constants"
)

// +kubebuilder:docs-gen:collapse=Imports

// ****************************************************************
// Test Cases Description:
//
// Before all cases:
//     create a MGH instance, a fake transport secret,
//     fake packagemanifest configuratoion and fake image pull secret.
//     check MGH instance and clustermanagedmentaddon are created
//
// After all case:
//     check clustermanagedmentaddon and all manifestworks are deleted
//
// Run two cases in series:
//
// 1. Create two fake standard OCP managed clusters, make sure manifestworks for ACM hub and agent are created,
//    Then deleted one of the managed clusters, make sure the corresponding manifestworks are deleted
//
// 2. Create two fake hypershift managed clusters, make sure manifestworks for ACM hub and agent are created,
//    Then deleted one of the hypershift managed clusters, make sure the corresponding manifestworks are deleted
//
// ****************************************************************
var _ = Describe("LeafHub controller", Ordered, func() {
	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		MGHName              = "test-mgh"
		StorageSecretName    = "storage-secret"
		TransportSecretName  = "transport-secret"
		kafkaCA              = "foobar"
		kafkaBootstrapServer = "https://test-kafka.example.com"

		ocpManagedCluster1    = "hub1"
		ocpManagedCluster2    = "hub2"
		hyperManagedCluster1  = "hyper1"
		hyperManagedCluster2  = "hyper2"
		hyperHostingCluster   = "hypermgt"
		hyperHostingNamespace = "clusters"

		timeout  = time.Second * 10
		duration = time.Second * 10
		interval = time.Millisecond * 250
	)

	var (
		ctx = context.Background()
		mgh = &operatorv1alpha2.MulticlusterGlobalHub{
			ObjectMeta: metav1.ObjectMeta{
				Name: MGHName,
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
	)

	BeforeAll(func() {
		// create a fake transport secret
		By("By creating a fake transport secret")
		transportSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      TransportSecretName,
				Namespace: config.GetDefaultNamespace(),
			},
			Data: map[string][]byte{
				"CA":               []byte(kafkaCA),
				"bootstrap_server": []byte(kafkaBootstrapServer),
			},
			Type: corev1.SecretTypeOpaque,
		}
		Expect(k8sClient.Create(ctx, transportSecret)).Should(Succeed())

		// create a MGH instance
		By("By creating a new MGH instance")
		mgh.SetNamespace(config.GetDefaultNamespace())
		Expect(k8sClient.Create(ctx, mgh)).Should(Succeed())

		// 	After creating this MGH instance, check that the MGH instance's Spec fields are failed with default values.
		mghLookupKey := types.NamespacedName{Namespace: config.GetDefaultNamespace(), Name: MGHName}
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

		// set fake packagemenifestwork configuration
		By("By setting a fake packagemanifest configuration")
		setPackageManifestConfig("release-2.6", "advanced-cluster-management.v2.6.0",
			"stable-2.0", "multicluster-engine.v2.0.1",
			map[string]string{"multiclusterhub-operator": "example.com/registration-operator:test"},
			map[string]string{"registration-operator": "example.com/registration-operator:test"})

		// create a fake image pull secret
		By("By creating a fake image pull secret")
		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.DefaultImagePullSecretName,
				Namespace: config.GetDefaultNamespace(),
			},
			Data: map[string][]byte{
				".dockerconfigjson": []byte("{}"),
			},
			Type: corev1.SecretTypeDockerConfigJson,
		})).Should(Succeed())
	})

	AfterAll(func() {
		// delete the testing MGH instance
		By("By deleting the testing MGH instance")
		Expect(k8sClient.Delete(ctx, mgh)).Should(Succeed())

		// check all the manifestworks for the managed clusters are deleted
		By("By checking all the manifestworks are deleted for all the managed clusters")
		Eventually(func() bool {
			workList := &workv1.ManifestWorkList{}
			err := k8sClient.List(ctx, workList, client.MatchingLabels(map[string]string{
				commonconstants.GlobalHubOwnerLabelKey: commonconstants.HoHOperatorOwnerLabelVal,
			}))
			return err == nil && len(workList.Items) == 0
		}, timeout, interval).Should(BeTrue())

		// checking the ClusterManagementAddon is deleted
		By("By checking the ClusterManagementAddon is deleted")
		Eventually(func() bool {
			clusterManagementAddOn := &addonv1alpha1.ClusterManagementAddOn{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name: constants.HoHClusterManagementAddonName,
			}, clusterManagementAddOn)
			return errors.IsNotFound(err)
		}, timeout, interval).Should(BeTrue())
	})

	// standard OCP managed clusters case
	Context("When new standard OCP managed clusters are created", func() {
		It("Should create Multicluster Global Hub resources when standard OCP managed clusters are found", func() {
			// create two testing managed clusters
			By("By creating two new OCP Managed Clusters")
			testingOCPManagedCluster1, testingOCPManagedCluster2 := &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: ocpManagedCluster1,
					Labels: map[string]string{
						"vendor": "OpenShift",
					},
					// add finalizer so that leafhub controller has time to clean up resources before the managed cluster is deleted
					Finalizers: []string{"cluster.open-cluster-management.io/api-resource-cleanup"},
				},
			}, &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: ocpManagedCluster2,
					Labels: map[string]string{
						"vendor": "OpenShift",
					},
					// add finalizer so that leafhub controller has time to clean up resources before the managed cluster is deleted
					Finalizers: []string{"cluster.open-cluster-management.io/api-resource-cleanup"},
				},
			}
			Expect(k8sClient.Create(ctx, testingOCPManagedCluster1)).Should(Succeed())
			Expect(k8sClient.Create(ctx, testingOCPManagedCluster2)).Should(Succeed())

			// update the status of managed clusters to available
			By("By updating the status of newly created managed clusters to available")
			managedClusterAvailableCondition := metav1.Condition{
				Type:               "ManagedClusterConditionAvailable",
				Reason:             "ManagedClusterAvailable",
				Message:            "Managed cluster is available",
				Status:             "True",
				LastTransitionTime: metav1.Time{Time: time.Now()},
			}
			testingOCPManagedCluster1.Status.Conditions = []metav1.Condition{
				managedClusterAvailableCondition,
			}
			testingOCPManagedCluster2.Status.Conditions = []metav1.Condition{
				managedClusterAvailableCondition,
			}
			Expect(k8sClient.Status().Update(ctx, testingOCPManagedCluster1)).Should(Succeed())
			Expect(k8sClient.Status().Update(ctx, testingOCPManagedCluster2)).Should(Succeed())

			// create managedcluster namespace
			By("By creating namespaces for the newly created managed clusters")
			Expect(k8sClient.Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: ocpManagedCluster1,
				},
			})).Should(Succeed())
			Expect(k8sClient.Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: ocpManagedCluster2,
				},
			})).Should(Succeed())

			// check the subscription manifestworks are created
			By("By checking the subscription manifestworks are created for the newly created managed clusters")
			subWork1, subWork2 := &workv1.ManifestWork{}, &workv1.ManifestWork{}
			Eventually(func() bool {
				err1, err2 := k8sClient.Get(ctx, types.NamespacedName{
					Name: fmt.Sprintf("%s-%s", ocpManagedCluster1,
						constants.HOHHubSubscriptionWorkSuffix),
					Namespace: ocpManagedCluster1,
				}, subWork1), k8sClient.Get(ctx, types.NamespacedName{
					Name: fmt.Sprintf("%s-%s", ocpManagedCluster2,
						constants.HOHHubSubscriptionWorkSuffix),
					Namespace: ocpManagedCluster2,
				}, subWork2)
				return err1 == nil && err2 == nil
			}, timeout, interval).Should(BeTrue())

			// set the status of the subscription manifestwork
			By("By setting the status of the subscription manifestworks for the newly created managed clusters")
			stateValueStr := "AtLatestKnown"
			SubWorkManifestResourceStatus := workv1.ManifestResourceStatus{
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
			subWork1.Status.ResourceStatus = SubWorkManifestResourceStatus
			subWork2.Status.ResourceStatus = SubWorkManifestResourceStatus
			Expect(k8sClient.Status().Update(ctx, subWork1)).Should(Succeed())
			Expect(k8sClient.Status().Update(ctx, subWork2)).Should(Succeed())

			// check the mch manifestwork are created
			By("By checking the mch manifestworks are created for the newly created managed clusters")
			mchWork1, mchWork2 := &workv1.ManifestWork{}, &workv1.ManifestWork{}
			Eventually(func() bool {
				err1, err2 := k8sClient.Get(ctx, types.NamespacedName{
					Name:      fmt.Sprintf("%s-%s", ocpManagedCluster1, constants.HoHHubMCHWorkSuffix),
					Namespace: ocpManagedCluster1,
				}, mchWork1), k8sClient.Get(ctx, types.NamespacedName{
					Name:      fmt.Sprintf("%s-%s", ocpManagedCluster2, constants.HoHHubMCHWorkSuffix),
					Namespace: ocpManagedCluster2,
				}, mchWork2)
				return err1 == nil && err2 == nil
			}, timeout, interval).Should(BeTrue())

			// set the status of the mch manifestwork
			By("By setting the status of the mch manifestworks for the newly created managed clusters")
			stateValueStr = "True"
			mchWorkManifestResourceStatus := workv1.ManifestResourceStatus{
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
			mchWork1.Status.ResourceStatus = mchWorkManifestResourceStatus
			mchWork2.Status.ResourceStatus = mchWorkManifestResourceStatus
			Expect(k8sClient.Status().Update(ctx, mchWork1)).Should(Succeed())
			Expect(k8sClient.Status().Update(ctx, mchWork2)).Should(Succeed())

			// check the agent manifestwork are created
			By("By checking the agent manifestworks are created for the newly created managed cluster")
			Eventually(func() bool {
				agentWork1, agentWork2 := &workv1.ManifestWork{}, &workv1.ManifestWork{}
				err1, err2 := k8sClient.Get(ctx, types.NamespacedName{
					Name:      fmt.Sprintf("%s-%s", ocpManagedCluster1, constants.HoHAgentWorkSuffix),
					Namespace: ocpManagedCluster1,
				}, agentWork1), k8sClient.Get(ctx, types.NamespacedName{
					Name:      fmt.Sprintf("%s-%s", ocpManagedCluster2, constants.HoHAgentWorkSuffix),
					Namespace: ocpManagedCluster2,
				}, agentWork2)
				return err1 == nil && err2 == nil
			}, timeout, interval).Should(BeTrue())

			// check the ManagedClusterAddon are created for the newly created managed clusters
			By("By checking the ManagedClusterAddon is created for the newly created managed cluster")
			Eventually(func() bool {
				managedClusterAddon1 := &addonv1alpha1.ManagedClusterAddOn{}
				managedClusterAddon2 := &addonv1alpha1.ManagedClusterAddOn{}
				err1, err2 := k8sClient.Get(ctx, types.NamespacedName{
					Name:      constants.HoHManagedClusterAddonName,
					Namespace: ocpManagedCluster1,
				}, managedClusterAddon1), k8sClient.Get(ctx, types.NamespacedName{
					Name:      constants.HoHManagedClusterAddonName,
					Namespace: ocpManagedCluster2,
				}, managedClusterAddon2)
				return err1 == nil && err2 == nil
			}, timeout, interval).Should(BeTrue())

			// delete one of the testing managedclusters
			By("By deleting one of the created OCP Managed Clusters")
			Expect(k8sClient.Delete(ctx, testingOCPManagedCluster1)).Should(Succeed())

			// checking the ManagedClusterAddon is deleted for the deleted managed cluster
			By("By checking the ManagedClusterAddon is deleted for the managed cluster")
			Eventually(func() bool {
				managedClusterAddon := &addonv1alpha1.ManagedClusterAddOn{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      constants.HoHManagedClusterAddonName,
					Namespace: ocpManagedCluster1,
				}, managedClusterAddon)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			// delete the finalizer for the managed cluster
			By("By deleting finalizers for the deleted managed cluster")
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ocpManagedCluster1,
			}, testingOCPManagedCluster1)).Should(Succeed())
			testingOCPManagedCluster1.SetFinalizers(nil)
			Expect(k8sClient.Status().Update(ctx, testingOCPManagedCluster1)).Should(Succeed())

			// delete all manifestworks for the deleted managed cluster
			// in real cluster, this is done by the cluster-manager-registration-controller
			Expect(k8sClient.DeleteAllOf(ctx, &workv1.ManifestWork{}, client.InNamespace(ocpManagedCluster1),
				client.MatchingLabels(map[string]string{
					commonconstants.GlobalHubOwnerLabelKey: commonconstants.HoHOperatorOwnerLabelVal,
				}))).Should(Succeed())
		})
	})

	// hypershift managed clusters case
	Context("When new hypershift managed clusters are created", func() {
		It("Should create Multicluster Global Hub resources when hypershift managed clusters are found", func() {
			// create two hypershiftdeployment
			By("By creating two new hypershiftdeployment")
			Expect(k8sClient.Create(ctx, &hypershiftdeploymentv1alpha1.HypershiftDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      hyperManagedCluster1,
					Namespace: config.GetDefaultNamespace(),
				},
				Spec: hypershiftdeploymentv1alpha1.HypershiftDeploymentSpec{
					HostingNamespace: hyperHostingNamespace,
				},
			})).Should(Succeed())
			Expect(k8sClient.Create(ctx, &hypershiftdeploymentv1alpha1.HypershiftDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      hyperManagedCluster2,
					Namespace: config.GetDefaultNamespace(),
				},
				Spec: hypershiftdeploymentv1alpha1.HypershiftDeploymentSpec{
					HostingNamespace: hyperHostingNamespace,
				},
			})).Should(Succeed())

			// create two testing hypershift managed clusters
			By("By creating two new hypershift Managed Clusters")
			testingHyperShiftManagedCluster1, testingHyperShiftManagedCluster2 := &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: hyperManagedCluster1,
					Annotations: map[string]string{
						"cluster.open-cluster-management.io/hypershiftdeployment":  "default/" + hyperManagedCluster1,
						"import.open-cluster-management.io/hosting-cluster-name":   hyperHostingCluster,
						"import.open-cluster-management.io/klusterlet-deploy-mode": "Hosted",
					},
					Labels: map[string]string{
						"vendor": "OpenShift",
					},
					// add finalizer so that leafhub controller has time to clean up resources before the managed cluster is deleted
					Finalizers: []string{"cluster.open-cluster-management.io/api-resource-cleanup"},
				},
			}, &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: hyperManagedCluster2,
					Annotations: map[string]string{
						"cluster.open-cluster-management.io/hypershiftdeployment":  "default/" + hyperManagedCluster2,
						"import.open-cluster-management.io/hosting-cluster-name":   hyperHostingCluster,
						"import.open-cluster-management.io/klusterlet-deploy-mode": "Hosted",
					},
					Labels: map[string]string{
						"vendor": "OpenShift",
					},
					// add finalizer so that leafhub controller has time to clean up resources before the managed cluster is deleted
					Finalizers: []string{"cluster.open-cluster-management.io/api-resource-cleanup"},
				},
			}
			Expect(k8sClient.Create(ctx, testingHyperShiftManagedCluster1)).Should(Succeed())
			Expect(k8sClient.Create(ctx, testingHyperShiftManagedCluster2)).Should(Succeed())

			// update the status of managed clusters to available
			By("By updating the status of newly created managed clusters to available")
			managedClusterAvailableCondition := metav1.Condition{
				Type:               "ManagedClusterConditionAvailable",
				Reason:             "ManagedClusterAvailable",
				Message:            "Managed cluster is available",
				Status:             "True",
				LastTransitionTime: metav1.Time{Time: time.Now()},
			}
			testingHyperShiftManagedCluster1.Status.Conditions = []metav1.Condition{
				managedClusterAvailableCondition,
			}
			testingHyperShiftManagedCluster2.Status.Conditions = []metav1.Condition{
				managedClusterAvailableCondition,
			}
			Expect(k8sClient.Status().Update(ctx, testingHyperShiftManagedCluster1)).Should(Succeed())
			Expect(k8sClient.Status().Update(ctx, testingHyperShiftManagedCluster2)).Should(Succeed())

			// create managedcluster namespace
			By("By creating namespaces for the newly created managed clusters")
			Expect(k8sClient.Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: hyperManagedCluster1,
				},
			})).Should(Succeed())
			Expect(k8sClient.Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: hyperManagedCluster2,
				},
			})).Should(Succeed())
			Expect(k8sClient.Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: hyperHostingCluster,
				},
			})).Should(Succeed())

			// check the hub manifestworks are created
			By("By checking the hub manifestworks are created for the newly created managed clusters")
			hubHostingWork1, hubHostedWork1 := &workv1.ManifestWork{}, &workv1.ManifestWork{}
			Eventually(func() bool {
				err1, err2 := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: hyperHostingCluster,
					Name:      fmt.Sprintf("%s-%s", hyperManagedCluster1, constants.HoHHostingHubWorkSuffix),
				}, hubHostingWork1), k8sClient.Get(ctx, types.NamespacedName{
					Namespace: hyperManagedCluster1,
					Name:      fmt.Sprintf("%s-%s", hyperManagedCluster1, constants.HoHHostedHubWorkSuffix),
				}, hubHostedWork1)
				return err1 == nil && err2 == nil
			}, timeout, interval).Should(BeTrue())

			hubHostingWork2, hubHostedWork2 := &workv1.ManifestWork{}, &workv1.ManifestWork{}
			Eventually(func() bool {
				err1, err2 := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: hyperHostingCluster,
					Name:      fmt.Sprintf("%s-%s", hyperManagedCluster2, constants.HoHHostingHubWorkSuffix),
				}, hubHostingWork2), k8sClient.Get(ctx, types.NamespacedName{
					Namespace: hyperManagedCluster2,
					Name:      fmt.Sprintf("%s-%s", hyperManagedCluster2, constants.HoHHostedHubWorkSuffix),
				}, hubHostedWork2)
				return err1 == nil && err2 == nil
			}, timeout, interval).Should(BeTrue())

			// set the status of the hub hosting manifestwork
			By("By setting the status of the hub hosting manifestworks for the newly created managed clusters")
			clusterIPValueStr := "172.30.223.23"
			resourceMeta := workv1.ManifestResourceMeta{
				Ordinal:  20,
				Group:    "",
				Kind:     "Service",
				Name:     "channels-apps-open-cluster-management-webhook-svc",
				Resource: "services",
				Version:  "v1",
			}
			resourceMeta.Namespace = fmt.Sprintf("%s-%s", hyperHostingNamespace, hyperManagedCluster1)
			hubHostingWorkManifestResourceStatus1 := workv1.ManifestResourceStatus{
				Manifests: []workv1.ManifestCondition{
					{
						ResourceMeta: resourceMeta,
						StatusFeedbacks: workv1.StatusFeedbackResult{
							Values: []workv1.FeedbackValue{
								{Name: "clusterIP", Value: workv1.FieldValue{Type: workv1.String, String: &clusterIPValueStr}},
							},
						},
						Conditions: []metav1.Condition{},
					},
				},
			}
			hubHostingWork1.Status.ResourceStatus = hubHostingWorkManifestResourceStatus1
			Expect(k8sClient.Status().Update(ctx, hubHostingWork1)).Should(Succeed())

			resourceMeta.Namespace = fmt.Sprintf("%s-%s", hyperHostingNamespace, hyperManagedCluster2)
			hubHostingWorkManifestResourceStatus2 := workv1.ManifestResourceStatus{
				Manifests: []workv1.ManifestCondition{
					{
						ResourceMeta: resourceMeta,
						StatusFeedbacks: workv1.StatusFeedbackResult{
							Values: []workv1.FeedbackValue{
								{Name: "clusterIP", Value: workv1.FieldValue{Type: workv1.String, String: &clusterIPValueStr}},
							},
						},
						Conditions: []metav1.Condition{},
					},
				},
			}
			hubHostingWork2.Status.ResourceStatus = hubHostingWorkManifestResourceStatus2
			Expect(k8sClient.Status().Update(ctx, hubHostingWork2)).Should(Succeed())

			// check the subscription manifestworks are created
			By("By checking the agent manifestworks are created for the newly created managed clusters")
			Eventually(func() bool {
				agentHostingWork1, agentHostedWork1 := &workv1.ManifestWork{}, &workv1.ManifestWork{}
				err1, err2 := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: hyperHostingCluster,
					Name:      fmt.Sprintf("%s-%s", hyperManagedCluster1, constants.HoHHostingHubWorkSuffix),
				}, agentHostingWork1), k8sClient.Get(ctx, types.NamespacedName{
					Namespace: hyperManagedCluster1,
					Name:      fmt.Sprintf("%s-%s", hyperManagedCluster1, constants.HoHHostedHubWorkSuffix),
				}, agentHostedWork1)
				return err1 == nil && err2 == nil
			}, timeout, interval).Should(BeTrue())

			Eventually(func() bool {
				agentHostingWork2, agentHostedWork2 := &workv1.ManifestWork{}, &workv1.ManifestWork{}
				err1, err2 := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: hyperHostingCluster,
					Name:      fmt.Sprintf("%s-%s", hyperManagedCluster1, constants.HoHHostingAgentWorkSuffix),
				}, agentHostingWork2), k8sClient.Get(ctx, types.NamespacedName{
					Namespace: hyperManagedCluster1,
					Name:      fmt.Sprintf("%s-%s", hyperManagedCluster1, constants.HoHHostedAgentWorkSuffix),
				}, agentHostedWork2)
				return err1 == nil && err2 == nil
			}, timeout, interval).Should(BeTrue())

			// check the ManagedClusterAddon are created for the newly created managed clusters
			By("By checking the ManagedClusterAddon is created for the newly created managed cluster")
			Eventually(func() bool {
				managedClusterAddon1 := &addonv1alpha1.ManagedClusterAddOn{}
				managedClusterAddon2 := &addonv1alpha1.ManagedClusterAddOn{}
				err1, err2 := k8sClient.Get(ctx, types.NamespacedName{
					Name:      constants.HoHManagedClusterAddonName,
					Namespace: hyperManagedCluster1,
				}, managedClusterAddon1), k8sClient.Get(ctx, types.NamespacedName{
					Name:      constants.HoHManagedClusterAddonName,
					Namespace: hyperManagedCluster2,
				}, managedClusterAddon2)
				return err1 == nil && err2 == nil
			}, timeout, interval).Should(BeTrue())

			// delete one of the testing hypershift managed clusters
			By("By deleting one of the created hypershift Managed Clusters")
			Expect(k8sClient.Delete(ctx, testingHyperShiftManagedCluster1)).Should(Succeed())

			// checking the ManagedClusterAddon is deleted for the deleted hypershift managed cluster
			By("By checking the ManagedClusterAddon is deleted for the managed cluster")
			Eventually(func() bool {
				managedClusterAddon := &addonv1alpha1.ManagedClusterAddOn{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      constants.HoHManagedClusterAddonName,
					Namespace: hyperManagedCluster1,
				}, managedClusterAddon)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			// checking hosting manifestworks are deleted for the deleted hypershift managed cluster
			By("By checking hosting manifestworks are deleted for the managed cluster")
			Eventually(func() bool {
				hubHostingWork1, agentHostingWork1 := &workv1.ManifestWork{}, &workv1.ManifestWork{}
				err1, err2 := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: hyperHostingCluster,
					Name:      fmt.Sprintf("%s-%s", hyperManagedCluster1, constants.HoHHostingHubWorkSuffix),
				}, hubHostingWork1), k8sClient.Get(ctx, types.NamespacedName{
					Namespace: hyperHostingCluster,
					Name:      fmt.Sprintf("%s-%s", hyperManagedCluster1, constants.HoHHostingAgentWorkSuffix),
				}, agentHostingWork1)
				return errors.IsNotFound(err1) && errors.IsNotFound(err2)
			}, timeout, interval).Should(BeTrue())

			// delete finalizers for the deleted hypershift managed cluster
			By("By deleting finalizers for the deleted hypershift managed cluster")
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: hyperManagedCluster1,
			}, testingHyperShiftManagedCluster1)).Should(Succeed())
			testingHyperShiftManagedCluster1.SetFinalizers(nil)
			Expect(k8sClient.Status().Update(ctx, testingHyperShiftManagedCluster1)).Should(Succeed())

			// delete all manifestworks for the deleted hypershift managed cluster
			// in real cluster, this is done by the cluster-manager-registration-controller
			Expect(k8sClient.DeleteAllOf(ctx, &workv1.ManifestWork{},
				client.InNamespace(hyperManagedCluster1),
				client.MatchingLabels(map[string]string{
					commonconstants.GlobalHubOwnerLabelKey: commonconstants.HoHOperatorOwnerLabelVal,
				}))).Should(Succeed())
		})
	})
})
