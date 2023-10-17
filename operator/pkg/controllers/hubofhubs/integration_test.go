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
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	"gopkg.in/yaml.v2"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	clusterv1beta2 "open-cluster-management.io/api/cluster/v1beta2"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	chnv1 "open-cluster-management.io/multicloud-operators-channel/pkg/apis/apps/v1"
	placementrulesv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	applicationv1beta1 "sigs.k8s.io/application/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/condition"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/hubofhubs"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	commonutils "github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// +kubebuilder:docs-gen:collapse=Imports

//go:embed manifests
var testFS embed.FS

// Define utility constants for object names and testing timeouts/durations and intervals.
const (
	MGHName              = "test-mgh"
	StorageSecretName    = operatorconstants.GHStorageSecretName
	TransportSecretName  = operatorconstants.GHTransportSecretName
	kafkaCACert          = "foobar"
	kafkaClientCert      = "foobar"
	KafkaClientKey       = "foobar"
	kafkaBootstrapServer = "https://test-kafka.example.com"
	datasourceSecretName = "multicluster-global-hub-grafana-datasources"

	timeout  = time.Second * 15
	duration = time.Second * 10
	interval = time.Millisecond * 250
)

var _ = Describe("MulticlusterGlobalHub controller", Ordered, func() {
	var storageSecret *corev1.Secret
	BeforeAll(func() {
		storageSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      StorageSecretName,
				Namespace: config.GetDefaultNamespace(),
			},
			Data: map[string][]byte{
				"database_uri": []byte(testPostgres.URI),
				"ca.crt":       []byte(""),
			},
			Type: corev1.SecretTypeOpaque,
		}
		Expect(k8sClient.Create(ctx, storageSecret)).Should(Succeed())
	})

	Context("When create MGH instance with invalid large scale data layer type", func() {
		It("Should not add finalizer to MGH instance and not deploy anything", func() {
			ctx := context.Background()
			// create a testing MGH instance with invalid large scale data layer setting
			By("By creating a new MGH instance with invalid large scale data layer setting")
			mgh := &globalhubv1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      MGHName,
					Namespace: config.GetDefaultNamespace(),
				},
				Spec: globalhubv1alpha4.MulticlusterGlobalHubSpec{},
			}
			Expect(k8sClient.Create(ctx, mgh)).Should(Succeed())

			// after creating this MGH instance, check that the MGH instance's Spec fields are failed with default values.
			mghLookupKey := types.NamespacedName{Namespace: config.GetDefaultNamespace(), Name: MGHName}
			createdMGH := &globalhubv1alpha4.MulticlusterGlobalHub{}

			// get this newly created MGH instance, given that creation may not immediately happen.
			Eventually(func() bool {
				err := k8sClient.Get(ctx, mghLookupKey, createdMGH)
				return err == nil
			}, timeout, interval).Should(BeTrue())

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
			mgh := &globalhubv1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      MGHName,
					Namespace: config.GetDefaultNamespace(),
					Annotations: map[string]string{
						operatorconstants.AnnotationImageOverridesCM: "noexisting-cm",
					},
				},
				Spec: globalhubv1alpha4.MulticlusterGlobalHubSpec{
					DataLayer: globalhubv1alpha4.DataLayerConfig{},
				},
			}
			Expect(k8sClient.Create(ctx, mgh)).Should(Succeed())

			// after creating this MGH instance, check that the MGH instance's Spec fields are failed with default values.
			mghLookupKey := types.NamespacedName{Namespace: config.GetDefaultNamespace(), Name: MGHName}
			createdMGH := &globalhubv1alpha4.MulticlusterGlobalHub{}

			// get this newly created MGH instance, given that creation may not immediately happen.
			Eventually(func() bool {
				err := k8sClient.Get(ctx, mghLookupKey, createdMGH)
				return err == nil
			}, timeout, interval).Should(BeTrue())

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
		mgh := &globalhubv1alpha4.MulticlusterGlobalHub{
			ObjectMeta: metav1.ObjectMeta{
				Name:      MGHName,
				Namespace: config.GetDefaultNamespace(),
			},
			Spec: globalhubv1alpha4.MulticlusterGlobalHubSpec{
				DataLayer: globalhubv1alpha4.DataLayerConfig{
					Kafka: globalhubv1alpha4.KafkaConfig{},
					Postgres: globalhubv1alpha4.PostgresConfig{
						Retention: "1y",
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
			By("By creating a fake transport secret")
			Expect(k8sClient.Create(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      TransportSecretName,
					Namespace: config.GetDefaultNamespace(),
				},
				Data: map[string][]byte{
					"ca.crt":           []byte(kafkaCACert),
					"client.crt":       []byte(kafkaClientCert),
					"client.key":       []byte(KafkaClientKey),
					"bootstrap_server": []byte(kafkaBootstrapServer),
				},
				Type: corev1.SecretTypeOpaque,
			})).Should(Succeed())

			By("By creating a new MCH instance")
			mch := &mchv1.MultiClusterHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multiclusterhub",
					Namespace: config.GetDefaultNamespace(),
				},
				Spec: mchv1.MultiClusterHubSpec{
					Overrides: &mchv1.Overrides{
						Components: []mchv1.ComponentConfig{
							{
								Name:    "app-lifecycle",
								Enabled: true,
							},
							{
								Name:    "grc",
								Enabled: true,
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, mch)).Should(Succeed())

			By("By creating a new MGH instance")
			mgh.SetNamespace(config.GetDefaultNamespace())
			Expect(k8sClient.Create(ctx, mgh)).Should(Succeed())

			createdMGH := &globalhubv1alpha4.MulticlusterGlobalHub{}
			// get this newly created MGH instance, given that creation may not immediately happen.
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(mgh), createdMGH)
			}, timeout, interval).Should(Succeed())

			// make sure the default values are filled
			// Expect(createdMGH.Spec.AggregationLevel).Should(Equal(globalhubv1alpha4.Full))
			// Expect(createdMGH.Spec.EnableLocalPolicies).Should(Equal(true))

			Eventually(func() error {
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mgh), createdMGH)).Should(Succeed())
				if condition.GetConditionStatus(createdMGH,
					condition.CONDITION_TYPE_DATABASE_INIT) !=
					condition.CONDITION_STATUS_TRUE {
					return fmt.Errorf("the database init condition is not set to true")
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
			imagePullPolicy := corev1.PullAlways
			if mgh.Spec.ImagePullPolicy != "" {
				imagePullPolicy = mgh.Spec.ImagePullPolicy
			}

			// dataRetention should at least be 1 month, otherwise it will deleted the current month partitions and records
			months, err := commonutils.ParseRetentionMonth(mgh.Spec.DataLayer.Postgres.Retention)
			Expect(err).NotTo(HaveOccurred())
			if months < 1 {
				months = 1
			}
			managerObjects, err = hohRenderer.Render("manifests/manager", "", func(
				profile string,
			) (interface{}, error) {
				return struct {
					Image                  string
					Replicas               int32
					ProxyImage             string
					ImagePullPolicy        string
					ImagePullSecret        string
					ProxySessionSecret     string
					DatabaseURL            string
					PostgresCACert         string
					KafkaCACert            string
					KafkaClientCert        string
					KafkaClientKey         string
					KafkaBootstrapServer   string
					TransportType          string
					TransportFormat        string
					MessageCompressionType string
					Namespace              string
					LeaseDuration          string
					RenewDeadline          string
					RetryPeriod            string
					SchedulerInterval      string
					SkipAuth               bool
					NodeSelector           map[string]string
					Tolerations            []corev1.Toleration
					RetentionMonth         int
					StatisticLogInterval   string
					EnableGlobalResource   bool
					LaunchJobNames         string
					LogLevel               string
				}{
					Image:                  config.GetImage(config.GlobalHubManagerImageKey),
					Replicas:               2,
					ProxyImage:             config.GetImage(config.OauthProxyImageKey),
					ImagePullPolicy:        string(imagePullPolicy),
					ImagePullSecret:        mgh.Spec.ImagePullSecret,
					ProxySessionSecret:     "testing",
					DatabaseURL:            base64.StdEncoding.EncodeToString([]byte(testPostgres.URI)),
					PostgresCACert:         base64.StdEncoding.EncodeToString([]byte("")),
					KafkaCACert:            base64.RawStdEncoding.EncodeToString([]byte(kafkaCACert)),
					KafkaClientCert:        base64.RawStdEncoding.EncodeToString([]byte(kafkaClientCert)),
					KafkaClientKey:         base64.RawStdEncoding.EncodeToString([]byte(KafkaClientKey)),
					KafkaBootstrapServer:   kafkaBootstrapServer,
					MessageCompressionType: string(operatorconstants.GzipCompressType),
					TransportType:          string(transport.Kafka),
					TransportFormat:        string(globalhubv1alpha4.CloudEvents),
					Namespace:              config.GetDefaultNamespace(),
					LeaseDuration:          "137",
					RenewDeadline:          "107",
					RetryPeriod:            "26",
					SkipAuth:               config.SkipAuth(mgh),
					SchedulerInterval:      config.GetSchedulerInterval(mgh),
					NodeSelector:           map[string]string{"foo": "bar"},
					Tolerations: []corev1.Toleration{
						{
							Key:      "dedicated",
							Operator: corev1.TolerationOpEqual,
							Effect:   corev1.TaintEffectNoSchedule,
							Value:    "infra",
						},
					},
					RetentionMonth:       months,
					StatisticLogInterval: config.GetStatisticLogInterval(),
					EnableGlobalResource: true,
					LaunchJobNames:       config.GetLaunchJobNames(mgh),
					LogLevel:             "info",
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
			// generate datasource secret: must before the grafana objects
			mghReconciler.GenerateGrafanaDataSourceSecret(ctx, mgh)
			Expect(err).NotTo(HaveOccurred())

			grafanaObjects, err = hohRenderer.Render("manifests/grafana", "", func(profile string) (interface{}, error) {
				return struct {
					Namespace            string
					Replicas             int32
					SessionSecret        string
					ProxyImage           string
					GrafanaImage         string
					DatasourceSecretName string
					ImagePullPolicy      string
					ImagePullSecret      string
					NodeSelector         map[string]string
					Tolerations          []corev1.Toleration
					LogLevel             string
				}{
					Namespace:            config.GetDefaultNamespace(),
					Replicas:             2,
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
					LogLevel: "info",
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

			By("Should create the local-cluster instance")
			localCluster := &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      operatorconstants.LocalClusterName,
					Namespace: config.GetDefaultNamespace(),
				},
			}
			Expect(k8sClient.Create(ctx, localCluster)).Should(Succeed())

			By("Should create a managed hub without this annotation")
			mh1 := &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mh1",
					Namespace: config.GetDefaultNamespace(),
				},
			}
			Expect(k8sClient.Create(ctx, mh1)).Should(Succeed())

			By("Should create a managed hub with this annotation, but the value is false")
			mh2 := &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mh2",
					Namespace: config.GetDefaultNamespace(),
					Annotations: map[string]string{
						"foo": "bar",
						operatorconstants.AnnotationONMulticlusterHub: "false",
					},
				},
			}
			Expect(k8sClient.Create(ctx, mh2)).Should(Succeed())

			By("checking the annotation in the managed hub is added")
			Eventually(func() error {
				clusters := &clusterv1.ManagedClusterList{}
				if err := k8sClient.List(ctx, clusters, &client.ListOptions{}); err != nil {
					return err
				}

				for _, managedHub := range clusters.Items {
					if managedHub.Name == operatorconstants.LocalClusterName {
						continue
					}
					annotations := managedHub.GetAnnotations()
					if val, ok := annotations[operatorconstants.AnnotationONMulticlusterHub]; ok {
						if val != "true" {
							return fmt.Errorf("the annotation(%s) value is not true", operatorconstants.AnnotationONMulticlusterHub)
						}
					}
				}
				return nil
			}, timeout, interval).Should(Succeed())

			By("By checking the kafkaBootstrapServer")
			createdMGH := &globalhubv1alpha4.MulticlusterGlobalHub{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mgh), createdMGH)).Should(Succeed())
			kafkaConnection, err := mghReconciler.GenerateKafkaConnectionFromGHTransportSecret(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(kafkaConnection.BootstrapServer).To(Equal(kafkaBootstrapServer))

			By("By checking the kafka secret is deleted")
			Expect(k8sClient.Delete(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      TransportSecretName,
					Namespace: config.GetDefaultNamespace(),
				},
			})).Should(Succeed())
			Eventually(func() error {
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      TransportSecretName,
					Namespace: config.GetDefaultNamespace(),
				}, &corev1.Secret{}); err != nil {
					if errors.IsNotFound(err) {
						return nil
					}
					return err
				}
				return fmt.Errorf("should not find the secret")
			}, timeout, interval).ShouldNot(HaveOccurred())
			Eventually(func() error {
				_, err = mghReconciler.GenerateKafkaConnectionFromGHTransportSecret(ctx)
				return err
			}, timeout, interval).Should(HaveOccurred())
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

			By("checking the annotation in the managed hub is deleted")
			Eventually(func() error {
				clusters := &clusterv1.ManagedClusterList{}
				if err := k8sClient.List(ctx, clusters, &client.ListOptions{}); err != nil {
					return err
				}

				for _, managedHub := range clusters.Items {
					if managedHub.Name == operatorconstants.LocalClusterName {
						continue
					}
					annotations := managedHub.GetAnnotations()
					if _, ok := annotations[operatorconstants.AnnotationONMulticlusterHub]; ok {
						return fmt.Errorf("the annotation(%s) should be deleted", operatorconstants.AnnotationONMulticlusterHub)
					}
				}
				return nil
			}, timeout, interval).Should(Succeed())
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

	Context("Reconcile the Postgres and kafka resources", func() {
		mcgh := &globalhubv1alpha4.MulticlusterGlobalHub{}
		It("Should create the MGH instance", func() {
			mcgh = &globalhubv1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      MGHName,
					Namespace: config.GetDefaultNamespace(),
					Annotations: map[string]string{
						operatorconstants.AnnotationMGHInstallCrunchyOperator: "true",
					},
				},
				Spec: globalhubv1alpha4.MulticlusterGlobalHubSpec{},
			}
			Expect(k8sClient.Create(ctx, mcgh)).Should(Succeed())
		})

		It("Should get the postgres connection", func() {
			Eventually(func() bool {
				mghReconciler.MiddlewareConfig.PgConnection = nil
				mghReconciler.MiddlewareConfig.KafkaConnection = nil
				mghReconciler.ReconcileMiddleware(ctx, mcgh)
				// has multicluster-global-hub-storage secret
				if mghReconciler.MiddlewareConfig.PgConnection == nil {
					return false
				}
				return true
				// Expect(mghReconciler.MiddlewareConfig.PgConnection).ShouldNot(BeNil())
				// // no multicluster-global-hub-transport secret
				// Expect(mghReconciler.MiddlewareConfig.KafkaConnection).Should(BeNil())
			}, timeout, interval).Should(BeTrue())

		})

		It("Should create the postgres resources", func() {
			Expect(mghReconciler.EnsureCrunchyPostgresSubscription(ctx, mcgh)).Should(Succeed())
			Expect(mghReconciler.EnsureCrunchyPostgres(ctx)).Should(Succeed())
			_, err := mghReconciler.WaitForPostgresReady(ctx)
			// postgres cannot be ready in envtest
			Expect(err).Should(HaveOccurred())
		})
		It("Should create the kafka resources", func() {
			Expect(mghReconciler.EnsureKafkaSubscription(ctx, mcgh)).Should(Succeed())
			Expect(mghReconciler.EnsureKafkaResources(ctx, mcgh)).Should(Succeed())
			_, err := mghReconciler.WaitForKafkaClusterReady(ctx)
			// postgres cannot be ready in envtest
			Expect(err).Should(HaveOccurred())
		})

		It("Should not get the postgres and kafka connection", func() {
			mghReconciler.MiddlewareConfig.KafkaConnection = nil

			Expect(k8sClient.Delete(ctx, storageSecret)).Should(Succeed())
			Eventually(func() error {
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      StorageSecretName,
					Namespace: config.GetDefaultNamespace(),
				}, &corev1.Secret{}); err != nil {
					if errors.IsNotFound(err) {
						return nil
					}
					return err
				}
				return fmt.Errorf("should not find the secret")
			}, timeout, interval).ShouldNot(HaveOccurred())
			mghReconciler.ReconcileMiddleware(ctx, mcgh)
			// no multicluster-global-hub-transport secret
			Expect(mghReconciler.MiddlewareConfig.KafkaConnection).Should(BeNil())
		})

		It("Should delete the MGH instance", func() {
			Expect(k8sClient.Delete(ctx, mcgh)).Should(Succeed())
		})
	})

	Context("Reconcile the Postgres database", Ordered, func() {
		mcgh := &globalhubv1alpha4.MulticlusterGlobalHub{}
		It("Should create the MGH instance", func() {
			mcgh = &globalhubv1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      MGHName,
					Namespace: config.GetDefaultNamespace(),
				},
				Spec: globalhubv1alpha4.MulticlusterGlobalHubSpec{},
			}
			Expect(k8sClient.Create(ctx, mcgh)).Should(Succeed())
		})

		It("Should get the postgres connection", func() {
			mghReconciler.MiddlewareConfig.PgConnection = nil
			err := mghReconciler.InitPostgresByStatefulset(ctx, mcgh)
			if err != nil {
				fmt.Println("InitPostgresBystatefuleset Error", err.Error())
			}
			Expect(mghReconciler.MiddlewareConfig.PgConnection).ShouldNot(BeNil())
			Expect(k8sClient.Delete(ctx, mcgh)).Should(Succeed())
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
