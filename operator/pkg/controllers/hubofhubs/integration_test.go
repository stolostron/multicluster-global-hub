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
	"encoding/json"
	"fmt"
	"time"

	"github.com/kylelemons/godebug/diff"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v2"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	clusterv1beta2 "open-cluster-management.io/api/cluster/v1beta2"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	chnv1 "open-cluster-management.io/multicloud-operators-channel/pkg/apis/apps/v1"
	placementrulesv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	applicationv1beta1 "sigs.k8s.io/application/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorv1alpha2 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha2"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/condition"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/hubofhubs"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

// +kubebuilder:docs-gen:collapse=Imports

//go:embed manifests
var testFS embed.FS

// Define utility constants for object names and testing timeouts/durations and intervals.
const (
	MGHName              = "test-mgh"
	StorageSecretName    = "storage-secret"
	TransportSecretName  = "transport-secret"
	kafkaCACert          = "foobar"
	kafkaBootstrapServer = "https://test-kafka.example.com"
	datasourceSecretName = "multicluster-global-hub-grafana-datasources"

	timeout  = time.Second * 10
	duration = time.Second * 10
	interval = time.Millisecond * 250
)

var _ = Describe("MulticlusterGlobalHub controller", Ordered, func() {
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
						operatorconstants.AnnotationImageOverridesCM: "noexisting-cm",
					},
				},
				Spec: operatorv1alpha2.MulticlusterGlobalHubSpec{
					DataLayer: &operatorv1alpha2.DataLayerConfig{
						Type: operatorv1alpha2.LargeScale,
						LargeScale: &operatorv1alpha2.LargeScaleConfig{
							Kafka: &operatorv1alpha2.KafkaConfig{
								Name:            TransportSecretName,
								TransportFormat: operatorv1alpha2.CloudEvents,
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

			// check finalizer should not be added to MGH instance
			Eventually(func() error {
				if err := k8sClient.Get(ctx, mghLookupKey, createdMGH); err != nil {
					return err
				}
				if len(createdMGH.GetFinalizers()) > 0 {
					return fmt.Errorf("the finalizer should not be added if mgh controller has an error occurred")
				}
				return nil
			}, timeout, interval).ShouldNot(HaveOccurred())

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
		mgh := &operatorv1alpha2.MulticlusterGlobalHub{
			ObjectMeta: metav1.ObjectMeta{
				Name: MGHName,
			},
			Spec: operatorv1alpha2.MulticlusterGlobalHubSpec{
				DataLayer: &operatorv1alpha2.DataLayerConfig{
					Type: operatorv1alpha2.LargeScale,
					LargeScale: &operatorv1alpha2.LargeScaleConfig{
						Kafka: &operatorv1alpha2.KafkaConfig{
							Name:            TransportSecretName,
							TransportFormat: operatorv1alpha2.CloudEvents,
						},
						Postgres: corev1.LocalObjectReference{
							Name: StorageSecretName,
						},
					},
				},
				NodeSelector: map[string]string{"foo": "bar"},
				Tolerations: []corev1.Toleration{
					{
						Key:      "dedicated",
						Operator: corev1.TolerationOpEqual,
						Effect:   corev1.TaintEffectNoSchedule,
						Value:    "infra",
					},
				},
			},
		}

		var managerObjects []*unstructured.Unstructured
		var grafanaObjects []*unstructured.Unstructured
		It("Should update the conditions and mgh finalizer when MCH instance is created", func() {
			By("By creating a storage secret")
			Expect(k8sClient.Create(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      StorageSecretName,
					Namespace: config.GetDefaultNamespace(),
				},
				Data: map[string][]byte{
					"database_uri": []byte(testPostgres.URI),
					"ca.crt":       []byte(""),
				},
				Type: corev1.SecretTypeOpaque,
			})).Should(Succeed())

			By("By creating a fake transport secret")
			Expect(k8sClient.Create(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      TransportSecretName,
					Namespace: config.GetDefaultNamespace(),
				},
				Data: map[string][]byte{
					"ca.crt":           []byte(kafkaCACert),
					"bootstrap_server": []byte(kafkaBootstrapServer),
				},
				Type: corev1.SecretTypeOpaque,
			})).Should(Succeed())

			By("By creating a new MGH instance")
			mgh.SetNamespace(config.GetDefaultNamespace())
			Expect(k8sClient.Create(ctx, mgh)).Should(Succeed())

			createdMGH := &operatorv1alpha2.MulticlusterGlobalHub{}
			// get this newly created MGH instance, given that creation may not immediately happen.
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(mgh), createdMGH)
			}, timeout, interval).Should(Succeed())

			// make sure the default values are filled
			Expect(createdMGH.Spec.AggregationLevel).Should(Equal(operatorv1alpha2.Full))
			Expect(createdMGH.Spec.EnableLocalPolicies).Should(Equal(true))
			Expect(createdMGH.Spec.DataLayer.LargeScale.Kafka.Name).Should(Equal(TransportSecretName))
			Expect(createdMGH.Spec.DataLayer.LargeScale.Postgres.Name).Should(Equal(StorageSecretName))

			Eventually(func() error {
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mgh), createdMGH)).Should(Succeed())
				if condition.GetConditionStatus(createdMGH,
					condition.CONDITION_TYPE_DATABASE_INIT) !=
					condition.CONDITION_STATUS_TRUE {
					return fmt.Errorf("the database init condition is not set to true")
				}
				if condition.GetConditionStatus(createdMGH,
					condition.CONDITION_TYPE_TRANSPORT_INIT) !=
					condition.CONDITION_STATUS_TRUE {
					return fmt.Errorf("the transport init condition is not set to true")
				}
				if condition.GetConditionStatus(createdMGH,
					condition.CONDITION_TYPE_MANAGER_AVAILABLE) !=
					condition.CONDITION_STATUS_FALSE {
					return fmt.Errorf("the manager available condition is not set")
				}
				if condition.GetConditionStatus(createdMGH,
					condition.CONDITION_TYPE_GRAFANA_AVAILABLE) !=
					condition.CONDITION_STATUS_FALSE {
					return fmt.Errorf("the grafana available condition is not set")
				}
				if !utils.Contains(createdMGH.GetFinalizers(), constants.GlobalHubCleanupFinalizer) {
					return fmt.Errorf("the finalizer(%s) should be added if mgh controller has no error occurred",
						constants.GlobalHubCleanupFinalizer)
				}
				return nil
			}, timeout, interval).Should(Succeed())
			prettyPrint(createdMGH.Status)
		})

		It("Should create Multicluster Global Manager resources when MGH instance is created", func() {
			// create hoh render for testing
			hohRenderer := renderer.NewHoHRenderer(testFS)

			By("By checking the multicluster-global-hub-manager resources are created as expected")
			messageCompressionType := string(mgh.Spec.MessageCompressionType)
			if messageCompressionType == "" {
				messageCompressionType = string(operatorv1alpha2.GzipCompressType)
			}

			imagePullPolicy := corev1.PullAlways
			if mgh.Spec.ImagePullPolicy != "" {
				imagePullPolicy = mgh.Spec.ImagePullPolicy
			}

			var err error
			managerObjects, err = hohRenderer.Render("manifests/manager", "", func(
				profile string,
			) (interface{}, error) {
				return struct {
					Image                  string
					ProxyImage             string
					ImagePullPolicy        string
					ImagePullSecret        string
					ProxySessionSecret     string
					DBSecret               string
					KafkaCACert            string
					KafkaBootstrapServer   string
					TransportType          string
					TransportFormat        string
					MessageCompressionType string
					Namespace              string
					LeaseDuration          string
					RenewDeadline          string
					RetryPeriod            string
					NodeSelector           map[string]string
					Tolerations            []corev1.Toleration
				}{
					Image:                  config.GetImage(config.GlobalHubManagerImageKey),
					ProxyImage:             config.GetImage(config.OauthProxyImageKey),
					ImagePullPolicy:        string(imagePullPolicy),
					ImagePullSecret:        mgh.Spec.ImagePullSecret,
					ProxySessionSecret:     "testing",
					DBSecret:               mgh.Spec.DataLayer.LargeScale.Postgres.Name,
					KafkaCACert:            base64.RawStdEncoding.EncodeToString([]byte(kafkaCACert)),
					KafkaBootstrapServer:   kafkaBootstrapServer,
					MessageCompressionType: messageCompressionType,
					TransportType:          string(transport.Kafka),
					TransportFormat:        string(mgh.Spec.DataLayer.LargeScale.Kafka.TransportFormat),
					Namespace:              config.GetDefaultNamespace(),
					LeaseDuration:          "137",
					RenewDeadline:          "107",
					RetryPeriod:            "26",
					NodeSelector:           map[string]string{"foo": "bar"},
					Tolerations: []corev1.Toleration{
						{
							Key:      "dedicated",
							Operator: corev1.TolerationOpEqual,
							Effect:   corev1.TaintEffectNoSchedule,
							Value:    "infra",
						},
					},
				}, nil
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() error {
				for _, desiredObj := range managerObjects {
					err := checkResourceExistence(ctx, k8sClient, desiredObj)
					if err != nil {
						return err
					}
				}
				fmt.Printf("all manager resources(%d) are created as expected \n", len(managerObjects))
				return nil
			}, timeout, interval).Should(Succeed())

			// get the grafana objects
			By("By checking the multicluster-global-hub-grafana resources are created as expected")
			grafanaObjects, err = hohRenderer.Render("manifests/grafana", "", func(profile string) (interface{}, error) {
				return struct {
					Namespace            string
					SessionSecret        string
					ProxyImage           string
					GrafanaImage         string
					DatasourceSecretName string
					ImagePullPolicy      string
					ImagePullSecret      string
					NodeSelector         map[string]string
					Tolerations          []corev1.Toleration
				}{
					Namespace:            config.GetDefaultNamespace(),
					SessionSecret:        "testing",
					ProxyImage:           config.GetImage(config.OauthProxyImageKey),
					GrafanaImage:         config.GetImage(config.GrafanaImageKey),
					ImagePullPolicy:      string(imagePullPolicy),
					ImagePullSecret:      mgh.Spec.ImagePullSecret,
					DatasourceSecretName: datasourceSecretName,
					NodeSelector:         map[string]string{"foo": "bar"},
					Tolerations: []corev1.Toleration{
						{
							Key:      "dedicated",
							Operator: corev1.TolerationOpEqual,
							Effect:   corev1.TaintEffectNoSchedule,
							Value:    "infra",
						},
					},
				}, nil
			})

			Expect(err).NotTo(HaveOccurred())
			Eventually(func() error {
				for _, desiredObj := range grafanaObjects {
					objLookupKey := types.NamespacedName{Name: desiredObj.GetName(), Namespace: desiredObj.GetNamespace()}
					foundObj := &unstructured.Unstructured{}
					foundObj.SetGroupVersionKind(desiredObj.GetObjectKind().GroupVersionKind())
					if err := k8sClient.Get(ctx, objLookupKey, foundObj); err != nil {
						return err
					}
					fmt.Printf("found grafana resource: %s(%s) \n",
						desiredObj.GetObjectKind().GroupVersionKind().Kind, objLookupKey)
				}
				fmt.Printf("all grafana resources(%d) are created as expected \n", len(grafanaObjects))
				return nil
			}, timeout, interval).Should(Succeed())
		})

		It("Should reconcile the resources when MGH instance is created", func() {
			By("By checking the multicluster-global-hub-config configmap is created")
			hohConfig := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: constants.GHSystemNamespace,
					Name:      constants.GHConfigCMName,
				}, hohConfig)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			By("By deleting the multicluster-global-hub-config configmap")
			Expect(k8sClient.Delete(ctx, hohConfig)).Should(Succeed())

			By("By checking the multicluster-global-hub-config configmap is recreated")
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Namespace: constants.GHSystemNamespace,
					Name:      constants.GHConfigCMName,
				}, &corev1.ConfigMap{})
			}, timeout, interval).Should(Succeed())

			By("By deleting the owned objects by the MGH instance and checking it can be recreated")
			for _, obj := range managerObjects {
				obj.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
				Expect(k8sClient.Delete(ctx, obj)).Should(Succeed())
				Eventually(func() error {
					desiredObj := &unstructured.Unstructured{}
					desiredObj.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
					return k8sClient.Get(ctx, types.NamespacedName{
						Namespace: obj.GetNamespace(),
						Name:      obj.GetName(),
					}, desiredObj)
				}, timeout, interval).Should(Succeed())
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
				return k8sClient.Delete(ctx, mutatingWebhookConfiguration)
			}, timeout, interval).Should(Succeed())

			By("mutatingwebhookconfiguration should be recreated")
			Eventually(func() error {
				newMutatingWebhookConfiguration := &admissionregistrationv1.MutatingWebhookConfiguration{}
				return k8sClient.Get(ctx, types.NamespacedName{
					Name: "multicluster-global-hub-mutator",
				},
					newMutatingWebhookConfiguration)
			}, timeout, interval).ShouldNot(HaveOccurred())

			By("By checking the kafkaBootstrapServer")
			createdMGH := &operatorv1alpha2.MulticlusterGlobalHub{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mgh), createdMGH)).Should(Succeed())
			server, _, _, _, err := utils.GetKafkaConfig(ctx, kubeClient, createdMGH)
			Expect(err).NotTo(HaveOccurred())
			Expect(server).To(Equal(kafkaBootstrapServer))

			By("By checking the kafka secret is deleted")
			Expect(k8sClient.Delete(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      TransportSecretName,
					Namespace: config.GetDefaultNamespace(),
				},
			})).Should(Succeed())
			_, _, _, _, err = utils.GetKafkaConfig(ctx, kubeClient, createdMGH)
			Expect(err).To(HaveOccurred())
		})

		It("Should remove finalizer added to MGH consumer resources", func() {
			By("By creating a finalizer placement")
			testPlacement := &clusterv1beta1.Placement{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-placement-1",
					Namespace:  config.GetDefaultNamespace(),
					Finalizers: []string{constants.GlobalHubCleanupFinalizer},
				},
				Spec: clusterv1beta1.PlacementSpec{},
			}
			Expect(k8sClient.Create(ctx, testPlacement, &client.CreateOptions{})).Should(Succeed())

			By("By creating a finalizer application")
			testApplication := &applicationv1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-application-1",
					Namespace:  config.GetDefaultNamespace(),
					Finalizers: []string{constants.GlobalHubCleanupFinalizer},
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
					Finalizers: []string{constants.GlobalHubCleanupFinalizer},
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
					Finalizers: []string{constants.GlobalHubCleanupFinalizer},
				},
				Spec: placementrulesv1.PlacementRuleSpec{
					SchedulerName: "test-schedulerName",
				},
			}
			Expect(k8sClient.Create(ctx, testPlacementrule, &client.CreateOptions{})).Should(Succeed())

			By("By creating a finalizer managedclustersetbinding")
			testManagedClusterSetBinding := &clusterv1beta2.ManagedClusterSetBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-managedclustersetbinding-1",
					Namespace:  config.GetDefaultNamespace(),
					Finalizers: []string{constants.GlobalHubCleanupFinalizer},
				},
				Spec: clusterv1beta2.ManagedClusterSetBindingSpec{
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
					Finalizers: []string{constants.GlobalHubCleanupFinalizer},
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
				cm := &corev1.ConfigMap{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: constants.GHSystemNamespace,
					Name:      constants.GHConfigCMName,
				}, cm)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			By("By checking the clusterrole is deleted")
			Eventually(func() error {
				listOpts := []client.ListOption{
					client.MatchingLabels(map[string]string{
						constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
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
						constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
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
						constants.GlobalHubCleanupFinalizer) {
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
						constants.GlobalHubCleanupFinalizer) {
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
						constants.GlobalHubCleanupFinalizer) {
						return fmt.Errorf("the placementrules finalizer has not been removed")
					}
				}
				return nil
			}, timeout, interval).ShouldNot(HaveOccurred())

			By("By checking the managedclustersetbinding finalizer is deleted")
			Eventually(func() error {
				managedclustersetbindings := &clusterv1beta2.ManagedClusterSetBindingList{}
				if err := k8sClient.List(ctx, managedclustersetbindings, &client.ListOptions{}); err != nil {
					return err
				}
				for idx := range managedclustersetbindings.Items {
					if utils.Contains(managedclustersetbindings.Items[idx].GetFinalizers(),
						constants.GlobalHubCleanupFinalizer) {
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
						constants.GlobalHubCleanupFinalizer) {
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
						constants.GlobalHubCleanupFinalizer) {
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
			// 		desiredObj := &unstructured.Unstructured{}
			// 		desiredObj.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
			// 		err := k8sClient.Get(ctx, types.NamespacedName{
			// 			Namespace: obj.GetNamespace(),
			// 			Name:      obj.GetName(),
			// 		}, desiredObj)
			// 		return errors.IsNotFound(err)
			// 	}, timeout, interval).Should(BeTrue())
			// }
		})
	})

	Context("When get the basic postgres connections", func() {
		It("Get the connection with default sslmode", func() {
			conn, err := database.PostgresConnection(context.TODO(), testPostgres.URI, nil)
			Expect(err).Should(Succeed())
			Expect(conn.Close(context.TODO())).Should(Succeed())
		})

		It("Get the connection with sslmode verify-ca", func() {
			_, err := database.PostgresConnection(context.TODO(),
				fmt.Sprintf("%s?sslmode=verify-ca", testPostgres.URI), nil)
			Expect(err).ShouldNot(Succeed())
		})

		It("Get the connection with invalid ca.crt", func() {
			_, err := database.PostgresConnection(context.TODO(), testPostgres.URI, []byte("test"))
			Expect(err).ShouldNot(Succeed())
		})

		It("Should get the datasource config", func() {
			_, err := hubofhubs.GrafanaDataSource(testPostgres.URI, []byte("test"))
			Expect(err).Should(Succeed())
		})
	})
})

func prettyPrint(v interface{}) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

func checkResourceExistence(ctx context.Context, k8sClient client.Client, desiredObj *unstructured.Unstructured) error {
	// skip session secret and kafka-ca secret check because they contain random value
	if desiredObj.GetName() == "nonk8s-apiserver-cookie-secret" ||
		desiredObj.GetName() == "kafka-ca-secret" ||
		desiredObj.GetName() == "multicluster-global-hub-grafana-cookie-secret" {
		return nil
	}
	objLookupKey := types.NamespacedName{Name: desiredObj.GetName(), Namespace: desiredObj.GetNamespace()}
	gvk := desiredObj.GetObjectKind().GroupVersionKind()
	foundObj := &unstructured.Unstructured{}
	foundObj.SetGroupVersionKind(desiredObj.GetObjectKind().GroupVersionKind())
	if err := k8sClient.Get(ctx, objLookupKey, foundObj); err != nil {
		return err
	}

	desiredObjContent, foundObjContent := desiredObj.Object, foundObj.Object
	if !apiequality.Semantic.DeepDerivative(desiredObjContent, foundObjContent) {
		desiredObjYaml, err := yaml.Marshal(desiredObjContent)
		if err != nil {
			return err
		}
		foundObjYaml, err := yaml.Marshal(foundObjContent)
		if err != nil {
			return err
		}

		return fmt.Errorf("desired and found %s(%s) are not equal, difference:\n%v\n desired: \n%v\n",
			gvk, objLookupKey, diff.Diff(string(desiredObjYaml), string(foundObjYaml)), string(desiredObjYaml))
	}

	return nil
}
