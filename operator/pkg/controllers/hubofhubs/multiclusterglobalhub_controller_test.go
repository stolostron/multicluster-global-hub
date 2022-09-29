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
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
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

var _ = Describe("MulticlusterGlobalHub controller", Ordered, func() {
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

	Context("When create MGH Instance without native data layer type", func() {
		It("Should not add finalizer to MGH instance and not deploy anything", func() {
			ctx := context.Background()
			By("By creating a new MGH instance with native data layer type")
			mgh := &operatorv1alpha2.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      MGHName,
					Namespace: config.GetDefaultNamespace(),
				},
				Spec: operatorv1alpha2.MulticlusterGlobalHubSpec{
					DataLayer: &operatorv1alpha2.DataLayerConfig{
						Type:   operatorv1alpha2.Native,
						Native: &operatorv1alpha2.NativeConfig{},
					},
				},
			}
			Expect(k8sClient.Create(ctx, mgh)).Should(Succeed())

			// after creating this MGH instance, check that the MGH instance's Spec fields are failed with default values.
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

			// check finalizer is not added to MGH instance
			By("By checking finalizer is not added to MGH instance")
			Expect(createdMGH.GetFinalizers()).Should(BeNil())

			// delete the testing MGH instance with native data layer type
			By("By deleting the testing MGH instance with native data layer type")
			Expect(k8sClient.Delete(ctx, mgh)).Should(Succeed())
		})
	})

	Context("When create MGH instance with invalid large scale data layer type", func() {
		It("Should not add finalizer to MGH instance and not deploy anything", func() {
			ctx := context.Background()
			// create a testing MGH instance with invalid large scale data layer setting
			By("By creating a new MGH instance with invalid large scale data layer setting")
			mgh := &operatorv1alpha2.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      MGHName,
					Namespace: config.GetDefaultNamespace(),
				},
				Spec: operatorv1alpha2.MulticlusterGlobalHubSpec{
					DataLayer: &operatorv1alpha2.DataLayerConfig{
						Type:       operatorv1alpha2.LargeScale,
						LargeScale: &operatorv1alpha2.LargeScaleConfig{},
					},
				},
			}
			Expect(k8sClient.Create(ctx, mgh)).Should(Succeed())

			// after creating this MGH instance, check that the MGH instance's Spec fields are failed with default values.
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

			// check finalizer is not added to MGH instance
			By("By checking finalizer is not added to MGH instance")
			Expect(createdMGH.GetFinalizers()).Should(BeNil())

			// delete the testing MGH instance with invalid large scale data layer setting
			By("By deleting the testing MGH instance with invalid large scale data layer setting")
			Expect(k8sClient.Delete(ctx, mgh)).Should(Succeed())
		})
	})

	Context("When create MGH instance with reference to nonexisting image override configmap", func() {
		It("Should add finalizer to MGH instance but not deploy anything", func() {
			ctx := context.Background()
			// create a testing MGH instance with reference to nonexisting image override configmap
			By("By creating a new MGH instance with reference to nonexisting image override configmap")
			mgh := &operatorv1alpha2.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      MGHName,
					Namespace: config.GetDefaultNamespace(),
					Annotations: map[string]string{
						constants.AnnotationImageOverridesCM: "noexisting-cm",
					},
				},
				Spec: operatorv1alpha2.MulticlusterGlobalHubSpec{
					DataLayer: &operatorv1alpha2.DataLayerConfig{
						Type: operatorv1alpha2.LargeScale,
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

			// after creating this MGH instance, check that the MGH instance's Spec fields are failed with default values.
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

			// check finalizer is added to MGH instance
			Eventually(func() bool {
				err := k8sClient.Get(ctx, mghLookupKey, createdMGH)
				return err == nil && len(createdMGH.GetFinalizers()) > 0
			}, timeout, interval).Should(BeTrue())

			// delete the testing MGH instance with reference to nonexisting image override configmap
			By("By deleting the testing MGH instance with reference to nonexisting image override configmap")
			Expect(k8sClient.Delete(ctx, mgh)).Should(Succeed())

			// check MGH instance is deleted
			Eventually(func() bool {
				err := k8sClient.Get(ctx, mghLookupKey, createdMGH)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When create MGH instance with large scale data layer type", func() {
		ctx := context.Background()
		storageSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: StorageSecretName,
			},
			Data: map[string][]byte{
				"database_uri": []byte("postgres://postgres:testpwd@hoh-primary.hoh-postgres.svc:/hoh"),
			},
			Type: corev1.SecretTypeOpaque,
		}
		transportSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: TransportSecretName,
			},
			Data: map[string][]byte{
				"CA":               []byte(kafkaCA),
				"bootstrap_server": []byte(kafkaBootstrapServer),
			},
			Type: corev1.SecretTypeOpaque,
		}
		mgh := &operatorv1alpha2.MulticlusterGlobalHub{
			ObjectMeta: metav1.ObjectMeta{
				Name: MGHName,
			},
			Spec: operatorv1alpha2.MulticlusterGlobalHubSpec{
				DataLayer: &operatorv1alpha2.DataLayerConfig{
					Type: operatorv1alpha2.LargeScale,
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
		It("Should add finalizer to MGH instance but fail to init database when MCH instance is created", func() {
			By("By creating a fake storage secret secret")
			storageSecret.SetNamespace(config.GetDefaultNamespace())
			Expect(k8sClient.Create(ctx, storageSecret)).Should(Succeed())

			By("By creating a fake transport secret")
			transportSecret.SetNamespace(config.GetDefaultNamespace())
			Expect(k8sClient.Create(ctx, transportSecret)).Should(Succeed())

			// create a testing MGH instance with valid large scale data layer setting
			By("By creating a new MGH instance without skipping databse initialization")
			mgh.SetNamespace(config.GetDefaultNamespace())
			Expect(k8sClient.Create(ctx, mgh)).Should(Succeed())

			// after creating this MGH instance, check that the MGH instance's Spec fields are failed with default values.
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

			By("By checking the MGH CR database init failure condition is added as expected")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, mghLookupKey, createdMGH)
				return err == nil && condition.ContainConditionStatus(createdMGH,
					condition.CONDITION_TYPE_DATABASE_INIT, metav1.ConditionFalse)
			}, timeout, interval).Should(BeTrue())
		})

		It("Should create Multicluster Global Hub resources when MGH instance is created", func() {
			// after creating this MGH instance, check that the MGH instance's Spec fields are failed with default values.
			mghLookupKey := types.NamespacedName{Namespace: config.GetDefaultNamespace(), Name: MGHName}
			createdMGH := &operatorv1alpha2.MulticlusterGlobalHub{}

			// get this newly created MGH instance, given that creation may not immediately happen.
			Eventually(func() bool {
				err := k8sClient.Get(ctx, mghLookupKey, createdMGH)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			Expect(k8sClient.Get(ctx, mghLookupKey, createdMGH)).Should(Succeed())

			By("By updating the MGH instance with skipping database initialization")
			originMCH := createdMGH.DeepCopy()
			createdMGH.SetAnnotations(map[string]string{
				constants.AnnotationMGHSkipDBInit: "true",
			})
			Expect(k8sClient.Patch(ctx, createdMGH, client.MergeFrom(originMCH))).Should(Succeed())

			// get this newly created MGH instance, given that creation may not immediately happen.
			Eventually(func() bool {
				err := k8sClient.Get(ctx, mghLookupKey, createdMGH)
				return err == nil
			}, timeout, interval).Should(BeTrue())

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

			By("By modifying the mutatingwebhookconfiguration to trigger the reconcile")
			Eventually(func() error {
				mutatingWebhookConfiguration := &admissionregistrationv1.MutatingWebhookConfiguration{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name: "multicluster-global-hub-mutator",
				}, mutatingWebhookConfiguration); err != nil {
					return err
				}

				namespacedScopeV1 := admissionregistrationv1.NamespacedScope
				mutatingWebhookConfiguration.Webhooks[0].Rules = []admissionregistrationv1.RuleWithOperations{
					{
						Operations: []admissionregistrationv1.OperationType{"CREATE", "UPDATE"},
						Rule: admissionregistrationv1.Rule{
							APIGroups:   []string{"cluster.open-cluster-management.io"},
							APIVersions: []string{"v1"},
							Resources:   []string{"test"},
							Scope:       &namespacedScopeV1,
						},
					},
				}
				if err := k8sClient.Update(ctx, mutatingWebhookConfiguration); err != nil {
					return err
				}
				return nil
			}, timeout, interval).ShouldNot(HaveOccurred())

			By("By checking the mutatingwebhookconfiguration is reconciled")
			Eventually(func() bool {
				newMutatingWebhookConfiguration := &admissionregistrationv1.MutatingWebhookConfiguration{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name: "multicluster-global-hub-mutator",
				}, newMutatingWebhookConfiguration); err != nil {
					return false
				}
				return len(newMutatingWebhookConfiguration.Webhooks[0].Rules) == 2
			}, timeout, interval).Should(BeTrue())

			By("Inject the caBundle to the mutatingwebhookconfiguration")
			Eventually(func() error {
				mutatingWebhookConfiguration := &admissionregistrationv1.MutatingWebhookConfiguration{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name: "multicluster-global-hub-mutator",
				}, mutatingWebhookConfiguration); err != nil {
					return err
				}
				mutatingWebhookConfiguration.Webhooks[0].ClientConfig.CABundle = []byte("test")
				if err := k8sClient.Update(ctx, mutatingWebhookConfiguration); err != nil {
					return err
				}
				return nil
			}, timeout, interval).ShouldNot(HaveOccurred())

			By("Check the caBundle of the mutatingwebhookconfiguration")
			Eventually(func() bool {
				mutatingWebhookConfiguration := &admissionregistrationv1.MutatingWebhookConfiguration{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name: "multicluster-global-hub-mutator",
				}, mutatingWebhookConfiguration); err != nil {
					return false
				}
				return string(mutatingWebhookConfiguration.Webhooks[0].ClientConfig.CABundle) == "test"
			}, timeout, interval).Should(BeTrue())

			By("By deleting the mutatingwebhookconfiguration")
			Eventually(func() error {
				mutatingWebhookConfiguration := &admissionregistrationv1.MutatingWebhookConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name: "multicluster-global-hub-mutator",
					},
				}
				if err := k8sClient.Delete(ctx, mutatingWebhookConfiguration); err != nil {
					return err
				}
				return nil
			}, timeout, interval).ShouldNot(HaveOccurred())

			By("mutatingwebhookconfiguration should be recreated")
			Eventually(func() error {
				newMutatingWebhookConfiguration := &admissionregistrationv1.MutatingWebhookConfiguration{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name: "multicluster-global-hub-mutator",
				}, newMutatingWebhookConfiguration); err != nil {
					return err
				}
				return nil
			}, timeout, interval).ShouldNot(HaveOccurred())

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
			testConditionType := "Test"
			testConditionReason := "ThisIsATest"
			testConditionMesage := "this is a test"
			testCondition := metav1.Condition{
				Type:               testConditionType,
				Status:             condition.CONDITION_STATUS_UNKNOWN,
				Reason:             testConditionReason,
				Message:            testConditionMesage,
				LastTransitionTime: metav1.Time{Time: time.Now()},
			}
			createdMGH.SetConditions(append(createdMGH.GetConditions(), testCondition))

			By("By checking the test condition is added to MGH instance")
			Expect(condition.ContainConditionStatusReason(createdMGH, testConditionType, testConditionReason,
				condition.CONDITION_STATUS_UNKNOWN)).Should(BeTrue())
		})

		It("Should remove finalizers added to MGH consumer resources", func() {
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
			Expect(k8sClient.Create(ctx, testManagedClusterSetBinding,
				&client.CreateOptions{})).Should(Succeed())

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

			By("By deleting the testing MGH instance")
			Expect(k8sClient.Delete(ctx, mgh)).Should(Succeed())

			By("By checking the multicluster-global-hub-config configmap is deleted")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: constants.HOHSystemNamespace,
					Name:      constants.HOHConfigName,
				}, &corev1.ConfigMap{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			By("By checking the clusterrole is deleted")
			Eventually(func() error {
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

			By("By checking the mutatingwebhookconfiguration is deleted")
			Eventually(func() error {
				listOpts := []client.ListOption{
					client.MatchingLabels(map[string]string{
						commonconstants.GlobalHubOwnerLabelKey: commonconstants.HoHOperatorOwnerLabelVal,
					}),
				}
				webhookList := &admissionregistrationv1.MutatingWebhookConfigurationList{}
				if err := k8sClient.List(ctx, webhookList, listOpts...); err != nil {
					return err
				}

				if len(webhookList.Items) > 0 {
					return fmt.Errorf("the mutatingwebhookconfiguration has not been removed")
				}
				return nil
			}, timeout, interval).ShouldNot(HaveOccurred())

			By("By checking the placement finalizer is deleted")
			Eventually(func() error {
				placements := &clusterv1beta1.PlacementList{}
				if err := k8sClient.List(ctx, placements, &client.ListOptions{}); err != nil {
					return err
				}
				for idx := range placements.Items {
					if utils.Contains(placements.Items[idx].GetFinalizers(),
						commonconstants.GlobalHubCleanupFinalizer) {
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
					if utils.Contains(applications.Items[idx].GetFinalizers(),
						commonconstants.GlobalHubCleanupFinalizer) {
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
					if utils.Contains(placementrules.Items[idx].GetFinalizers(),
						commonconstants.GlobalHubCleanupFinalizer) {
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
