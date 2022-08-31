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

package hubofhubs

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/kylelemons/godebug/diff"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	operatorv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/apis/operator/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
	commonconstants "github.com/stolostron/multicluster-global-hub/pkg/constants"
)

// +kubebuilder:docs-gen:collapse=Imports

var _ = Describe("MulticlusterGlobalHub controller", func() {
	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		MGHNamespace         = "default"
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
			By("By creating a new MGH instance")
			ctx := context.Background()
			By("By creating fake storage secret and transport secret")
			storageSecret := &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Secret",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      StorageSecretName,
					Namespace: MGHNamespace,
				},
				Data: map[string][]byte{
					"database_uri": []byte("postgres://postgres:testpwd@hoh-primary.hoh-postgres.svc:/hoh"),
				},
				Type: corev1.SecretTypeOpaque,
			}
			Expect(k8sClient.Create(ctx, storageSecret)).Should(Succeed())

			transportSecret := &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Secret",
				},
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

			mgh := &operatorv1alpha1.MulticlusterGlobalHub{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "operator.open-cluster-management.io/v1alpha1",
					Kind:       "MulticlusterGlobalHub",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      MGHName,
					Namespace: MGHNamespace,
					Annotations: map[string]string{
						constants.AnnotationMGHSkipDBInit:                        "true",
						commonconstants.GlobalHubSkipConsoleInstallAnnotationKey: "true",
					},
				},
				Spec: operatorv1alpha1.MulticlusterGlobalHubSpec{
					Storage: corev1.LocalObjectReference{
						Name: StorageSecretName,
					},
					Transport: corev1.LocalObjectReference{
						Name: TransportSecretName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, mgh)).Should(Succeed())

			// 	After creating this MGH instance, check that the MGH instance's Spec fields are failed with default values.
			mghLookupKey := types.NamespacedName{Namespace: MGHNamespace, Name: MGHName}
			createdMGH := &operatorv1alpha1.MulticlusterGlobalHub{}

			// get this newly created MGH instance, given that creation may not immediately happen.
			Eventually(func() bool {
				err := k8sClient.Get(ctx, mghLookupKey, createdMGH)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			// make sure the default values are filled
			Expect(createdMGH.Spec.AggregationLevel).Should(Equal(operatorv1alpha1.Full))
			Expect(createdMGH.Spec.EnableLocalPolicies).Should(Equal(true))

			// create hoh render for testing
			hohRenderer := renderer.NewHoHRenderer(fs)

			checkResource := func(ctx context.Context, k8sClient client.Client, unsObj *unstructured.Unstructured,
			) error {
				objLookupKey := types.NamespacedName{Name: unsObj.GetName(), Namespace: unsObj.GetNamespace()}
				gvk := unsObj.GetObjectKind().GroupVersionKind()
				foundObj := &unstructured.Unstructured{}
				foundObj.SetGroupVersionKind(unsObj.GetObjectKind().GroupVersionKind())
				if err := k8sClient.Get(ctx, objLookupKey, foundObj); err != nil {
					return err
				}

				// delete metadata before compare
				unsObjContent, foundObjContent := unsObj.Object, foundObj.Object
				delete(unsObjContent, "metadata")
				delete(foundObjContent, "metadata")
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

				fmt.Printf("desired and found %s(%s) are equal\n", gvk, objLookupKey)
				return nil
			}

			By("By checking the multicluster-global-hub-manager resources are created as expected")
			managerObjects, err := hohRenderer.Render("manifests/manager", func(
				component string,
			) (interface{}, error) {
				return struct {
					Image                string
					DBSecret             string
					KafkaCA              string
					KafkaBootstrapServer string
					Namespace            string
				}{
					Image:                config.GetImage("multicluster_global_hub_manager"),
					DBSecret:             mgh.Spec.Storage.Name,
					KafkaCA:              base64.RawStdEncoding.EncodeToString([]byte(kafkaCA)),
					KafkaBootstrapServer: kafkaBootstrapServer,
					Namespace:            MGHNamespace,
				}, nil
			})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() bool {
				for _, unsObj := range managerObjects {
					err := checkResource(ctx, k8sClient, unsObj)
					if err != nil {
						fmt.Printf("failed to check manager resource: %v\n", err)
						return false
					}
				}
				return true
			}, timeout, interval).Should(BeTrue())

			By("By checking the multicluster-global-hub-rbac resources are created as expected")
			hohRBACObjects, err := hohRenderer.Render("manifests/rbac", func(
				component string,
			) (interface{}, error) {
				return struct {
					Image     string
					Namespace string
				}{
					Image:     config.GetImage("multicluster_global_hub_rbac"),
					Namespace: MGHNamespace,
				}, nil
			})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() bool {
				for _, unsObj := range hohRBACObjects {
					if unsObj.GetName() == "opa-data" {
						// skip opa-data secret check
						continue
					}
					err := checkResource(ctx, k8sClient, unsObj)
					if err != nil {
						fmt.Printf("failed to check rbac resource: %v\n", err)
						return false
					}
				}
				return true
			}, timeout, interval).Should(BeTrue())

			By("By checking the placement related resources are created as expected")
			placementObjects, err := hohRenderer.Render("manifests/placement", func(
				component string,
			) (interface{}, error) {
				return struct {
					Namespace string
				}{
					Namespace: MGHNamespace,
				}, nil
			})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() bool {
				for _, unsObj := range placementObjects {
					err := checkResource(ctx, k8sClient, unsObj)
					if err != nil {
						fmt.Printf("failed to check placement related resource: %v\n", err)
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
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			By("By deleting the multicluster-global-hub-config configmap")
			Expect(k8sClient.Delete(ctx, hohConfig)).Should(Succeed())

			Eventually(func() bool {
				hohConfig := &corev1.ConfigMap{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: constants.HOHSystemNamespace,
					Name:      constants.HOHConfigName,
				}, hohConfig)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())
		})
	})
})
