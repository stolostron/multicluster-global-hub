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

package hubofhubs_test

import (
	"context"
	"embed"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/kylelemons/godebug/diff"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/open-horizon/edge-utilities/logger/log"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	chnv1 "open-cluster-management.io/multicloud-operators-channel/pkg/apis/apps/v1"
	placementrulesv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	applicationv1beta1 "sigs.k8s.io/application/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	operatorv1alpha2 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha2"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/condition"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	commonconstants "github.com/stolostron/multicluster-global-hub/pkg/constants"
)

// +kubebuilder:docs-gen:collapse=Imports

//go:embed manifests
var testFS embed.FS

var _ = Describe("MulticlusterGlobalHub controller", func() {
	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		MGHName              = "test-mgh"
		StorageSecretName    = "storage-secret"
		TransportSecretName  = "transport-secret"
		kafkaCA              = "foobar"
		kafkaBootstrapServer = "https://test-kafka.example.com"

		timeout  = time.Second * 10
		duration = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When create MGH Instance", func() {
		It("Should create Multicluster Global Hub resources when MGH instance is created", func() {
			ctx := context.Background()

			By("By creating a fake storage secret secret")
			storageSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      StorageSecretName,
					Namespace: config.GetDefaultNamespace(),
				},
				Data: map[string][]byte{
					"database_uri": []byte("postgres://postgres:testpwd@hoh-primary.hoh-postgres.svc:/hoh"),
				},
				Type: corev1.SecretTypeOpaque,
			}
			Expect(k8sClient.Create(ctx, storageSecret)).Should(Succeed())

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

			By("By creating a new MGH instance")
			mgh := &operatorv1alpha2.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      MGHName,
					Namespace: config.GetDefaultNamespace(),
					Annotations: map[string]string{
						constants.AnnotationMGHSkipDBInit: "true",
					},
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
			mghLookupKey := types.NamespacedName{Namespace: config.GetDefaultNamespace(), Name: MGHName}
			createdMGH := &operatorv1alpha2.MulticlusterGlobalHub{}

			// get this newly created MGH instance, given that creation may not immediately happen.
			Eventually(func() bool {
				err := k8sClient.Get(ctx, mghLookupKey, createdMGH)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			// make sure the default values are filled
			Expect(createdMGH.Spec.AggregationLevel).Should(Equal(operatorv1alpha2.Full))
			Expect(createdMGH.Spec.EnableLocalPolicies).Should(Equal(true))
			Expect(createdMGH.Spec.DataLayer.LargeScale.Kafka.Name).Should(Equal(TransportSecretName))
			Expect(createdMGH.Spec.DataLayer.LargeScale.Postgres.Name).Should(Equal(StorageSecretName))

			By("By checking the MGH CR database init conditions are created as expected")
			condition.SetConditionDatabaseInit(ctx, k8sClient, createdMGH, condition.CONDITION_STATUS_TRUE)
			Expect(condition.GetConditionStatus(createdMGH,
				condition.CONDITION_TYPE_DATABASE_INIT)).Should(Equal(metav1.ConditionTrue))
			Eventually(func() bool {
				condition.SetConditionDatabaseInit(ctx, k8sClient, createdMGH, condition.CONDITION_STATUS_FALSE)
				return condition.GetConditionStatus(createdMGH,
					condition.CONDITION_TYPE_DATABASE_INIT) == metav1.ConditionFalse
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				condition.SetConditionDatabaseInit(ctx, k8sClient, createdMGH, condition.CONDITION_STATUS_UNKNOWN)
				return condition.GetConditionStatus(createdMGH,
					condition.CONDITION_TYPE_DATABASE_INIT) == metav1.ConditionUnknown
			}, timeout, interval).Should(BeTrue())

			By("By checking the MGH CR transport init conditions are created as expected")
			condition.SetConditionTransportInit(ctx, k8sClient, createdMGH, condition.CONDITION_STATUS_UNKNOWN)
			Expect(condition.GetConditionStatus(createdMGH,
				condition.CONDITION_TYPE_TRANSPORT_INIT)).Should(Equal(metav1.ConditionUnknown))
			Eventually(func() bool {
				condition.SetConditionTransportInit(ctx, k8sClient, createdMGH, condition.CONDITION_STATUS_FALSE)
				return condition.GetConditionStatus(createdMGH,
					condition.CONDITION_TYPE_TRANSPORT_INIT) == metav1.ConditionFalse
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				condition.SetConditionTransportInit(ctx, k8sClient, createdMGH, condition.CONDITION_STATUS_TRUE)
				return condition.GetConditionStatus(createdMGH,
					condition.CONDITION_TYPE_TRANSPORT_INIT) == metav1.ConditionTrue
			}, timeout, interval).Should(BeTrue())

			By("By checking the MGH CR manager deployed conditions are created as expected")
			condition.SetConditionManagerDeployed(ctx, k8sClient, createdMGH, condition.CONDITION_STATUS_FALSE)
			Expect(condition.GetConditionStatus(createdMGH,
				condition.CONDITION_TYPE_MANAGER_DEPLOY)).Should(Equal(metav1.ConditionFalse))
			Eventually(func() bool {
				condition.SetConditionManagerDeployed(ctx, k8sClient, createdMGH, condition.CONDITION_STATUS_TRUE)
				return condition.GetConditionStatus(createdMGH,
					condition.CONDITION_TYPE_MANAGER_DEPLOY) == metav1.ConditionTrue
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				condition.SetConditionManagerDeployed(ctx, k8sClient, createdMGH, condition.CONDITION_STATUS_UNKNOWN)
				return condition.GetConditionStatus(createdMGH,
					condition.CONDITION_TYPE_MANAGER_DEPLOY) == metav1.ConditionUnknown
			}, timeout, interval).Should(BeTrue())

			By("By checking the MGH CR regional hub deployed conditions are created as expected")
			condition.SetConditionLeafHubDeployed(ctx, k8sClient, createdMGH, "test", condition.CONDITION_STATUS_TRUE)
			Expect(condition.GetConditionStatus(createdMGH,
				condition.CONDITION_TYPE_LEAFHUB_DEPLOY)).Should(Equal(metav1.ConditionTrue))
			Eventually(func() bool {
				condition.SetConditionLeafHubDeployed(ctx, k8sClient, createdMGH,
					"test", condition.CONDITION_STATUS_FALSE)
				return condition.GetConditionStatus(createdMGH,
					condition.CONDITION_TYPE_LEAFHUB_DEPLOY) == metav1.ConditionFalse
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				condition.SetConditionLeafHubDeployed(ctx, k8sClient, createdMGH,
					"test", condition.CONDITION_STATUS_UNKNOWN)
				return condition.GetConditionStatus(createdMGH,
					condition.CONDITION_TYPE_LEAFHUB_DEPLOY) == metav1.ConditionUnknown
			}, timeout, interval).Should(BeTrue())

			// create hoh render for testing
			hohRenderer := renderer.NewHoHRenderer(testFS)

			checkResourceExistence := func(ctx context.Context, k8sClient client.Client, unsObj *unstructured.Unstructured,
			) error {
				objLookupKey := types.NamespacedName{Name: unsObj.GetName(), Namespace: unsObj.GetNamespace()}
				gvk := unsObj.GetObjectKind().GroupVersionKind()
				foundObj := &unstructured.Unstructured{}
				foundObj.SetGroupVersionKind(unsObj.GetObjectKind().GroupVersionKind())
				if err := k8sClient.Get(ctx, objLookupKey, foundObj); err != nil {
					return err
				}

				unsObjContent, foundObjContent := unsObj.Object, foundObj.Object
				if !apiequality.Semantic.DeepDerivative(unsObjContent, foundObjContent) {
					unsObjYaml, err := yaml.Marshal(unsObjContent)
					if err != nil {
						return err
					}
					foundObjYaml, err := yaml.Marshal(foundObjContent)
					if err != nil {
						return err
					}

					return fmt.Errorf("desired and found %s(%s) are not equal, difference:\n%v\n",
						gvk, objLookupKey, diff.Diff(string(unsObjYaml), string(foundObjYaml)))
				}

				return nil
			}

			By("By checking the multicluster-global-hub-manager resources are created as expected")
			managerObjects, err := hohRenderer.Render("manifests/manager", "", func(
				profile string,
			) (interface{}, error) {
				return struct {
					Image                string
					DBSecret             string
					KafkaCA              string
					KafkaBootstrapServer string
					Namespace            string
				}{
					Image:                config.GetImage("multicluster_global_hub_manager"),
					DBSecret:             mgh.Spec.DataLayer.LargeScale.Postgres.Name,
					KafkaCA:              base64.RawStdEncoding.EncodeToString([]byte(kafkaCA)),
					KafkaBootstrapServer: kafkaBootstrapServer,
					Namespace:            config.GetDefaultNamespace(),
				}, nil
			})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() bool {
				for _, unsObj := range managerObjects {
					err := checkResourceExistence(ctx, k8sClient, unsObj)
					if err != nil {
						fmt.Printf("failed to check manager resource: %v\n", err)
						return false
					}
				}
				return true
			}, timeout, interval).Should(BeTrue())

			By("By checking the multicluster-global-hub-config configmap is created")
			hohConfig := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: constants.HOHSystemNamespace,
					Name:      constants.HOHConfigName,
				}, hohConfig)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			By("By deleting the multicluster-global-hub-config configmap")
			Expect(k8sClient.Delete(ctx, hohConfig)).Should(Succeed())

			By("By checking the multicluster-global-hub-config configmap is recreated")
			Eventually(func() bool {
				hohConfig := &corev1.ConfigMap{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: constants.HOHSystemNamespace,
					Name:      constants.HOHConfigName,
				}, hohConfig)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			By("By deleting the owned objects by the MGH instance and checking it can be recreated")
			for _, obj := range managerObjects {
				obj.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
				Expect(k8sClient.Delete(ctx, obj)).Should(Succeed())
				Eventually(func() bool {
					unsObj := &unstructured.Unstructured{}
					unsObj.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
					err := k8sClient.Get(ctx, types.NamespacedName{
						Namespace: obj.GetNamespace(),
						Name:      obj.GetName(),
					}, unsObj)
					return err == nil
				}, timeout, interval).Should(BeTrue())
			}

			By("By checking the kafkaBootstrapServer")
			server, _, err := utils.GetKafkaConfig(ctx, kubeClient, createdMGH)
			Expect(err).NotTo(HaveOccurred())
			Expect(server).To(Equal(kafkaBootstrapServer))

			By("By checking the kafka secret is deleted")
			Expect(k8sClient.Delete(ctx, transportSecret)).Should(Succeed())
			_, _, err = utils.GetKafkaConfig(ctx, kubeClient, createdMGH)
			Expect(err).To(HaveOccurred())

			By("By setting a test condition")
			Expect(len(createdMGH.GetConditions())).Should(BeNumerically(">", 0))
			testCondition := metav1.Condition{
				Type: "Test", Status: condition.CONDITION_STATUS_UNKNOWN,
				Reason:             "ThisIsATest",
				Message:            "this is a test",
				LastTransitionTime: metav1.Time{Time: time.Now()},
			}
			createdMGH.SetConditions(append(createdMGH.GetConditions(), testCondition))

			By("By checking the test condition")
			Expect(createdMGH.GetConditions()).To(ContainElement(testCondition))

			By("By creating a finalizer placement")
			testPlacement := &clusterv1beta1.Placement{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-placement-1",
					Namespace:  config.GetDefaultNamespace(),
					Finalizers: []string{commonconstants.GlobalHubCleanupFinalizer},
				},
				Spec: clusterv1beta1.PlacementSpec{},
			}
			Expect(k8sClient.Create(ctx, testPlacement, &client.CreateOptions{})).Should(Succeed())

			By("By creating a finalizer application")
			testApplication := &applicationv1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-application-1",
					Namespace:  config.GetDefaultNamespace(),
					Finalizers: []string{commonconstants.GlobalHubCleanupFinalizer},
				},
				Spec: applicationv1beta1.ApplicationSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "nginx-app-details"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, testApplication, &client.CreateOptions{})).Should(Succeed())

			By("By creating a finalizer policy")
			testPolicy := &policyv1.Policy{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-policy-1",
					Namespace:  config.GetDefaultNamespace(),
					Finalizers: []string{commonconstants.GlobalHubCleanupFinalizer},
				},
				Spec: policyv1.PolicySpec{
					Disabled:        true,
					PolicyTemplates: []*policyv1.PolicyTemplate{},
				},
			}
			Expect(k8sClient.Create(ctx, testPolicy, &client.CreateOptions{})).Should(Succeed())

			By("By creating a finalizer palcementrule")
			testPlacementrule := &placementrulesv1.PlacementRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-placementrule-1",
					Namespace:  config.GetDefaultNamespace(),
					Finalizers: []string{commonconstants.GlobalHubCleanupFinalizer},
				},
				Spec: placementrulesv1.PlacementRuleSpec{
					SchedulerName: "test-schedulerName",
				},
			}
			Expect(k8sClient.Create(ctx, testPlacementrule, &client.CreateOptions{})).Should(Succeed())

			By("By creating a finalizer managedclustersetbinding")
			testManagedClusterSetBinding := &clusterv1beta1.ManagedClusterSetBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-managedclustersetbinding-1",
					Namespace:  config.GetDefaultNamespace(),
					Finalizers: []string{commonconstants.GlobalHubCleanupFinalizer},
				},
				Spec: clusterv1beta1.ManagedClusterSetBindingSpec{
					ClusterSet: "test-clusterset",
				},
			}
			Expect(k8sClient.Create(ctx, testManagedClusterSetBinding, &client.CreateOptions{})).Should(Succeed())

			By("By creating a finalizer channel")
			testChannel := &chnv1.Channel{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-channel-1",
					Namespace:  config.GetDefaultNamespace(),
					Finalizers: []string{commonconstants.GlobalHubCleanupFinalizer},
				},
				Spec: chnv1.ChannelSpec{
					Type:               chnv1.ChannelTypeGit,
					Pathname:           config.GetDefaultNamespace(),
					InsecureSkipVerify: true,
				},
			}
			Expect(k8sClient.Create(ctx, testChannel, &client.CreateOptions{})).Should(Succeed())

			// delete the testing MGH instance
			By("By deleting the testing MGH instance")
			Expect(k8sClient.Delete(ctx, mgh)).Should(Succeed())

			// check the multicluster-global-hub-config configmap is deleted
			By("By checking the multicluster-global-hub-config configmap is deleted")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: constants.HOHSystemNamespace,
					Name:      constants.HOHConfigName,
				}, hohConfig)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			By("By checking the clusterrole is deleted")
			Eventually(func() error {
				log.Info("clean up multicluster-global-hub global resources")
				listOpts := []client.ListOption{
					client.MatchingLabels(map[string]string{
						commonconstants.GlobalHubOwnerLabelKey: commonconstants.HoHOperatorOwnerLabelVal,
					}),
				}
				clusterRoleList := &rbacv1.ClusterRoleList{}
				if err := k8sClient.List(ctx, clusterRoleList, listOpts...); err != nil {
					return err
				}
				if len(clusterRoleList.Items) > 0 {
					return fmt.Errorf("the clusterrole has not been removed")
				}
				return nil
			}, timeout, interval).ShouldNot(HaveOccurred())

			By("By checking the palcement finalizer is deleted")
			Eventually(func() error {
				placements := &clusterv1beta1.PlacementList{}
				if err := k8sClient.List(ctx, placements, &client.ListOptions{}); err != nil {
					return err
				}
				for idx := range placements.Items {
					if utils.Contains(placements.Items[idx].GetFinalizers(), commonconstants.GlobalHubCleanupFinalizer) {
						return fmt.Errorf("the placements finalizer has not been removed")
					}
				}
				return nil
			}, timeout, interval).ShouldNot(HaveOccurred())

			By("By checking the application finalizer is deleted")
			Eventually(func() error {
				applications := &applicationv1beta1.ApplicationList{}
				if err := k8sClient.List(ctx, applications, &client.ListOptions{}); err != nil {
					return err
				}
				for idx := range applications.Items {
					if utils.Contains(applications.Items[idx].GetFinalizers(), commonconstants.GlobalHubCleanupFinalizer) {
						return fmt.Errorf("the applications finalizer has not been removed")
					}
				}
				return nil
			}, timeout, interval).ShouldNot(HaveOccurred())

			By("By checking the placementrule finalizer is deleted")
			Eventually(func() error {
				placementrules := &placementrulesv1.PlacementRuleList{}
				if err := k8sClient.List(ctx, placementrules, &client.ListOptions{}); err != nil {
					return err
				}
				for idx := range placementrules.Items {
					if utils.Contains(placementrules.Items[idx].GetFinalizers(), commonconstants.GlobalHubCleanupFinalizer) {
						return fmt.Errorf("the placementrules finalizer has not been removed")
					}
				}
				return nil
			}, timeout, interval).ShouldNot(HaveOccurred())

			By("By checking the managedclustersetbinding finalizer is deleted")
			Eventually(func() error {
				managedclustersetbindings := &clusterv1beta1.ManagedClusterSetBindingList{}
				if err := k8sClient.List(ctx, managedclustersetbindings, &client.ListOptions{}); err != nil {
					return err
				}
				for idx := range managedclustersetbindings.Items {
					if utils.Contains(managedclustersetbindings.Items[idx].GetFinalizers(),
						commonconstants.GlobalHubCleanupFinalizer) {
						return fmt.Errorf("the managedclustersetbindings finalizer has not been removed")
					}
				}
				return nil
			}, timeout, interval).ShouldNot(HaveOccurred())

			By("By checking the channel finalizer is deleted")
			Eventually(func() error {
				channels := &chnv1.ChannelList{}
				if err := k8sClient.List(ctx, channels, &client.ListOptions{}); err != nil {
					return err
				}
				for idx := range channels.Items {
					if utils.Contains(channels.Items[idx].GetFinalizers(),
						commonconstants.GlobalHubCleanupFinalizer) {
						return fmt.Errorf("the channels finalizer has not been removed")
					}
				}
				return nil
			}, timeout, interval).ShouldNot(HaveOccurred())

			By("By checking the policy finalizer is deleted")
			Eventually(func() error {
				policies := &policyv1.PolicyList{}
				if err := k8sClient.List(ctx, policies, &client.ListOptions{}); err != nil {
					return err
				}
				for idx := range policies.Items {
					if utils.Contains(policies.Items[idx].GetFinalizers(),
						commonconstants.GlobalHubCleanupFinalizer) {
						return fmt.Errorf("the policies finalizer has not been removed")
					}
				}
				return nil
			}, timeout, interval).ShouldNot(HaveOccurred())

			// check the owned objects are deleted
			// comment the following test cases becase there is no gc controller in envtest
			// see: https://book.kubebuilder.io/reference/envtest.html#testing-considerations
			// for _, obj := range managerObjects {
			// 	Eventually(func() bool {
			// 		unsObj := &unstructured.Unstructured{}
			// 		unsObj.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
			// 		err := k8sClient.Get(ctx, types.NamespacedName{
			// 			Namespace: obj.GetNamespace(),
			// 			Name:      obj.GetName(),
			// 		}, unsObj)
			// 		return errors.IsNotFound(err)
			// 	}, timeout, interval).Should(BeTrue())
			// }
		})
	})
})
