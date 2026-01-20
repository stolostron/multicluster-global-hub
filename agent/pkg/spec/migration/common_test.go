// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package migration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestAddPauseAnnotations(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()

	cases := []struct {
		name          string
		clusterName   string
		initObjects   []client.Object
		expectedError bool
		expectedValue map[string]string
	}{
		{
			name:        "Successfully add pause annotation to ClusterDeployment",
			clusterName: "cluster1",
			initObjects: []client.Object{
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "hive.openshift.io/v1",
						"kind":       "ClusterDeployment",
						"metadata": map[string]interface{}{
							"name":      "cluster1",
							"namespace": "cluster1",
						},
						"spec": map[string]interface{}{
							"clusterName": "cluster1",
						},
					},
				},
			},
			expectedError: false,
			expectedValue: map[string]string{
				HivePauseAnnotation: "true",
			},
		},
		{
			name:        "Skip adding pause annotation to ImageClusterInstall (not in ZTPClusterResources)",
			clusterName: "cluster1",
			initObjects: []client.Object{
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "extensions.hive.openshift.io/v1alpha1",
						"kind":       "ImageClusterInstall",
						"metadata": map[string]interface{}{
							"name":      "cluster1",
							"namespace": "cluster1",
						},
						"spec": map[string]interface{}{
							"clusterDeploymentRef": map[string]interface{}{
								"name": "cluster1",
							},
						},
					},
				},
			},
			expectedError: false,
			expectedValue: nil, // No annotation should be added since ImageClusterInstall is not in the list
		},
		{
			name:        "Successfully add pause annotation to resource with existing annotations",
			clusterName: "cluster1",
			initObjects: []client.Object{
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "hive.openshift.io/v1",
						"kind":       "ClusterDeployment",
						"metadata": map[string]interface{}{
							"name":      "cluster1",
							"namespace": "cluster1",
							"annotations": map[string]interface{}{
								"existing-annotation": "value",
							},
						},
						"spec": map[string]interface{}{
							"clusterName": "cluster1",
						},
					},
				},
			},
			expectedError: false,
			expectedValue: map[string]string{
				"existing-annotation": "value",
				HivePauseAnnotation:   "true",
			},
		},
		{
			name:        "Skip adding annotation when resource not found",
			clusterName: "cluster1",
			initObjects: []client.Object{},
			// No error expected, should just skip
			expectedError: false,
		},
		{
			name:        "Add pause annotation to ClusterDeployment but not ImageClusterInstall",
			clusterName: "cluster1",
			initObjects: []client.Object{
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "hive.openshift.io/v1",
						"kind":       "ClusterDeployment",
						"metadata": map[string]interface{}{
							"name":      "cluster1",
							"namespace": "cluster1",
						},
						"spec": map[string]interface{}{
							"clusterName": "cluster1",
						},
					},
				},
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "extensions.hive.openshift.io/v1alpha1",
						"kind":       "ImageClusterInstall",
						"metadata": map[string]interface{}{
							"name":      "cluster1",
							"namespace": "cluster1",
						},
						"spec": map[string]interface{}{
							"clusterDeploymentRef": map[string]interface{}{
								"name": "cluster1",
							},
						},
					},
				},
			},
			expectedError: false,
			expectedValue: map[string]string{
				HivePauseAnnotation: "true",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(c.initObjects...).Build()

			err := AddPauseAnnotations(ctx, fakeClient, c.clusterName)

			if c.expectedError {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)

				// Verify annotations were added only to resources in ZTPClusterResources
				for _, obj := range c.initObjects {
					u := obj.(*unstructured.Unstructured)
					resource := &unstructured.Unstructured{}
					resource.SetGroupVersionKind(u.GroupVersionKind())

					err := fakeClient.Get(ctx, client.ObjectKeyFromObject(u), resource)
					assert.Nil(t, err)

					annotations := resource.GetAnnotations()
					// Only check expected annotations for resources in ZTPClusterResources
					var matchedResource *ZTPResourceConfig
					for i := range ZTPClusterResources {
						if ZTPClusterResources[i].GVK == u.GroupVersionKind() {
							matchedResource = &ZTPClusterResources[i]
							break
						}
					}

					if matchedResource != nil && c.expectedValue != nil {
						for key, expectedVal := range c.expectedValue {
							assert.Equal(t, expectedVal, annotations[key])
						}
					} else if matchedResource == nil && c.expectedValue != nil {
						// For resources not in ZTPClusterResources, annotation should not be added
						if annotations != nil {
							_, hasHiveKey := annotations[HivePauseAnnotation]
							_, hasMetal3Key := annotations[Metal3PauseAnnotation]
							assert.False(t, hasHiveKey || hasMetal3Key, "Annotation should not be added to %s", u.GetKind())
						}
					}
				}
			}
		})
	}
}

func TestRemovePauseAnnotations(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()

	cases := []struct {
		name          string
		clusterName   string
		initObjects   []client.Object
		expectedError bool
		shouldHaveKey bool
	}{
		{
			name:        "Successfully remove pause annotation from ClusterDeployment",
			clusterName: "cluster1",
			initObjects: []client.Object{
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "hive.openshift.io/v1",
						"kind":       "ClusterDeployment",
						"metadata": map[string]interface{}{
							"name":      "cluster1",
							"namespace": "cluster1",
							"annotations": map[string]interface{}{
								HivePauseAnnotation: "true",
							},
						},
						"spec": map[string]interface{}{
							"clusterName": "cluster1",
						},
					},
				},
			},
			expectedError: false,
			shouldHaveKey: false,
		},
		{
			name:        "Skip removal of pause annotation from ImageClusterInstall (not in ZTPResourceConfigGVKs)",
			clusterName: "cluster1",
			initObjects: []client.Object{
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "extensions.hive.openshift.io/v1alpha1",
						"kind":       "ImageClusterInstall",
						"metadata": map[string]interface{}{
							"name":      "cluster1",
							"namespace": "cluster1",
							"annotations": map[string]interface{}{
								HivePauseAnnotation: "true",
							},
						},
						"spec": map[string]interface{}{
							"clusterDeploymentRef": map[string]interface{}{
								"name": "cluster1",
							},
						},
					},
				},
			},
			expectedError: false,
			shouldHaveKey: true, // Annotation should remain since ImageClusterInstall is not in the list
		},
		{
			name:        "Successfully remove pause annotation while keeping other annotations",
			clusterName: "cluster1",
			initObjects: []client.Object{
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "hive.openshift.io/v1",
						"kind":       "ClusterDeployment",
						"metadata": map[string]interface{}{
							"name":      "cluster1",
							"namespace": "cluster1",
							"annotations": map[string]interface{}{
								HivePauseAnnotation:  "true",
								"other-annotation":   "value",
								"another-annotation": "value2",
							},
						},
						"spec": map[string]interface{}{
							"clusterName": "cluster1",
						},
					},
				},
			},
			expectedError: false,
			shouldHaveKey: false,
		},
		{
			name:        "Skip removal when resource not found",
			clusterName: "cluster1",
			initObjects: []client.Object{},
			// No error expected, should just skip
			expectedError: false,
		},
		{
			name:        "Skip removal when resource has no pause annotation",
			clusterName: "cluster1",
			initObjects: []client.Object{
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "hive.openshift.io/v1",
						"kind":       "ClusterDeployment",
						"metadata": map[string]interface{}{
							"name":      "cluster1",
							"namespace": "cluster1",
						},
						"spec": map[string]interface{}{
							"clusterName": "cluster1",
						},
					},
				},
			},
			expectedError: false,
		},
		{
			name:        "Remove pause annotation from ClusterDeployment but not ImageClusterInstall",
			clusterName: "cluster1",
			initObjects: []client.Object{
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "hive.openshift.io/v1",
						"kind":       "ClusterDeployment",
						"metadata": map[string]interface{}{
							"name":      "cluster1",
							"namespace": "cluster1",
							"annotations": map[string]interface{}{
								HivePauseAnnotation: "true",
							},
						},
						"spec": map[string]interface{}{
							"clusterName": "cluster1",
						},
					},
				},
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "extensions.hive.openshift.io/v1alpha1",
						"kind":       "ImageClusterInstall",
						"metadata": map[string]interface{}{
							"name":      "cluster1",
							"namespace": "cluster1",
							"annotations": map[string]interface{}{
								HivePauseAnnotation: "true",
							},
						},
						"spec": map[string]interface{}{
							"clusterDeploymentRef": map[string]interface{}{
								"name": "cluster1",
							},
						},
					},
				},
			},
			expectedError: false,
			shouldHaveKey: false, // Will be checked per-resource based on ZTPResourceConfigGVKs
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(c.initObjects...).Build()

			err := RemovePauseAnnotations(ctx, fakeClient, c.clusterName)

			if c.expectedError {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)

				// Verify annotations were removed only from resources in ZTPClusterResources
				for _, obj := range c.initObjects {
					u := obj.(*unstructured.Unstructured)
					resource := &unstructured.Unstructured{}
					resource.SetGroupVersionKind(u.GroupVersionKind())

					err := fakeClient.Get(ctx, client.ObjectKeyFromObject(u), resource)
					assert.Nil(t, err)

					// Check if resource is in ZTPClusterResources and get its pause annotation
					var matchedResource *ZTPResourceConfig
					for i := range ZTPClusterResources {
						if ZTPClusterResources[i].GVK == u.GroupVersionKind() {
							matchedResource = &ZTPClusterResources[i]
							break
						}
					}

					annotations := resource.GetAnnotations()
					if annotations != nil {
						if matchedResource != nil {
							// For resources in ZTPClusterResources, check the specific pause annotation
							_, hasKey := annotations[matchedResource.PauseAnnotation]
							// For resources in ZTPClusterResources, annotation should be removed
							assert.Equal(t, c.shouldHaveKey, hasKey)
						} else {
							// For resources not in ZTPClusterResources, annotation should remain
							// Check original object to see if it had the annotation
							origAnnotations := obj.(*unstructured.Unstructured).GetAnnotations()
							if origAnnotations != nil {
								_, origHasHive := origAnnotations[HivePauseAnnotation]
								_, hasHive := annotations[HivePauseAnnotation]
								assert.Equal(t, origHasHive, hasHive, "Hive annotation state should not change for %s", u.GetKind())
							}
						}
					}
				}
			}
		})
	}
}

func TestRemoveDeprovisionFinalizers(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()

	cases := []struct {
		name               string
		clusterName        string
		initObjects        []client.Object
		expectedError      bool
		expectedFinalizers []string
	}{
		{
			name:        "Successfully remove /deprovision finalizers from ClusterDeployment",
			clusterName: "cluster1",
			initObjects: []client.Object{
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "hive.openshift.io/v1",
						"kind":       "ClusterDeployment",
						"metadata": map[string]interface{}{
							"name":       "cluster1",
							"namespace":  "cluster1",
							"finalizers": []interface{}{"hive.openshift.io/deprovision", "other-finalizer"},
						},
						"spec": map[string]interface{}{
							"clusterName": "cluster1",
						},
					},
				},
			},
			expectedError:      false,
			expectedFinalizers: []string{"other-finalizer"},
		},
		{
			name:        "Skip removal of /deprovision finalizers from ImageClusterInstall (not in ZTPResourceConfigGVKs)",
			clusterName: "cluster1",
			initObjects: []client.Object{
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "extensions.hive.openshift.io/v1alpha1",
						"kind":       "ImageClusterInstall",
						"metadata": map[string]interface{}{
							"name":       "cluster1",
							"namespace":  "cluster1",
							"finalizers": []interface{}{"imageclusterinstall.agent-install.openshift.io/deprovision"},
						},
						"spec": map[string]interface{}{
							"clusterDeploymentRef": map[string]interface{}{
								"name": "cluster1",
							},
						},
					},
				},
			},
			expectedError:      false,
			expectedFinalizers: []string{"imageclusterinstall.agent-install.openshift.io/deprovision"}, // Should remain since not in the list
		},
		{
			name:        "Skip removal when resource not found",
			clusterName: "cluster1",
			initObjects: []client.Object{},
			// No error expected, should just skip
			expectedError: false,
		},
		{
			name:        "Skip removal when resource has no finalizers",
			clusterName: "cluster1",
			initObjects: []client.Object{
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "hive.openshift.io/v1",
						"kind":       "ClusterDeployment",
						"metadata": map[string]interface{}{
							"name":      "cluster1",
							"namespace": "cluster1",
						},
						"spec": map[string]interface{}{
							"clusterName": "cluster1",
						},
					},
				},
			},
			expectedError: false,
		},
		{
			name:        "Keep non-deprovision finalizers",
			clusterName: "cluster1",
			initObjects: []client.Object{
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "hive.openshift.io/v1",
						"kind":       "ClusterDeployment",
						"metadata": map[string]interface{}{
							"name":       "cluster1",
							"namespace":  "cluster1",
							"finalizers": []interface{}{"finalizer1", "finalizer2"},
						},
						"spec": map[string]interface{}{
							"clusterName": "cluster1",
						},
					},
				},
			},
			expectedError:      false,
			expectedFinalizers: []string{"finalizer1", "finalizer2"},
		},
		{
			name:        "Remove /deprovision finalizers from ClusterDeployment but not ImageClusterInstall",
			clusterName: "cluster1",
			initObjects: []client.Object{
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "hive.openshift.io/v1",
						"kind":       "ClusterDeployment",
						"metadata": map[string]interface{}{
							"name":       "cluster1",
							"namespace":  "cluster1",
							"finalizers": []interface{}{"hive.openshift.io/deprovision"},
						},
						"spec": map[string]interface{}{
							"clusterName": "cluster1",
						},
					},
				},
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "extensions.hive.openshift.io/v1alpha1",
						"kind":       "ImageClusterInstall",
						"metadata": map[string]interface{}{
							"name":       "cluster1",
							"namespace":  "cluster1",
							"finalizers": []interface{}{"imageclusterinstall.agent-install.openshift.io/deprovision", "keep-this"},
						},
						"spec": map[string]interface{}{
							"clusterDeploymentRef": map[string]interface{}{
								"name": "cluster1",
							},
						},
					},
				},
			},
			expectedError:      false,
			expectedFinalizers: nil, // Will be checked per-resource based on ZTPResourceConfigGVKs
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(c.initObjects...).Build()

			err := RemoveDeprovisionFinalizers(ctx, fakeClient, c.clusterName)

			if c.expectedError {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)

				// Verify only /deprovision finalizers were removed from resources in ZTPClusterResources
				for _, obj := range c.initObjects {
					u := obj.(*unstructured.Unstructured)
					resource := &unstructured.Unstructured{}
					resource.SetGroupVersionKind(u.GroupVersionKind())

					err := fakeClient.Get(ctx, client.ObjectKeyFromObject(u), resource)
					assert.Nil(t, err)

					// Check if resource is in ZTPClusterResources
					var matchedResource *ZTPResourceConfig
					for i := range ZTPClusterResources {
						if ZTPClusterResources[i].GVK == u.GroupVersionKind() {
							matchedResource = &ZTPClusterResources[i]
							break
						}
					}

					finalizers := resource.GetFinalizers()
					if c.expectedFinalizers != nil {
						// For test cases with single resource type, use expectedFinalizers
						if len(c.initObjects) == 1 {
							assert.Equal(t, c.expectedFinalizers, finalizers)
						}
					} else {
						// For test cases with multiple resources, check based on ZTPClusterResources
						if matchedResource != nil {
							// For resources in ZTPClusterResources, /deprovision finalizers should be removed
							for _, f := range finalizers {
								assert.NotContains(t, f, "/deprovision", "Resource in ZTPClusterResources should not have /deprovision finalizer")
							}
						} else {
							// For resources not in ZTPClusterResources, finalizers should remain unchanged
							origFinalizers := obj.(*unstructured.Unstructured).GetFinalizers()
							assert.Equal(t, origFinalizers, finalizers, "Finalizers should not change for %s", u.GetKind())
						}
					}
				}
			}
		})
	}
}

func TestRemoveDeprovisionFinalizersHelper(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()

	cases := []struct {
		name               string
		initObject         client.Object
		expectedError      bool
		expectedFinalizers []string
	}{
		{
			name: "Successfully remove only /deprovision finalizers from resource",
			initObject: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":       "test-cm",
						"namespace":  "test-ns",
						"finalizers": []interface{}{"hive.openshift.io/deprovision", "keep-this-finalizer"},
					},
					"data": map[string]interface{}{
						"key": "value",
					},
				},
			},
			expectedError:      false,
			expectedFinalizers: []string{"keep-this-finalizer"},
		},
		{
			name: "Keep all finalizers when none have /deprovision suffix",
			initObject: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":       "test-cm",
						"namespace":  "test-ns",
						"finalizers": []interface{}{"finalizer1", "finalizer2"},
					},
					"data": map[string]interface{}{
						"key": "value",
					},
				},
			},
			expectedError:      false,
			expectedFinalizers: []string{"finalizer1", "finalizer2"},
		},
		{
			name: "Skip removal when resource has no finalizers",
			initObject: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "test-cm",
						"namespace": "test-ns",
					},
					"data": map[string]interface{}{
						"key": "value",
					},
				},
			},
			expectedError:      false,
			expectedFinalizers: nil,
		},
		{
			name:          "Return nil when object is nil",
			initObject:    nil,
			expectedError: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var fakeClient client.Client
			if c.initObject != nil {
				fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(c.initObject).Build()
			} else {
				fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			}

			err := removeDeprovisionFinalizers(ctx, fakeClient, c.initObject)

			if c.expectedError {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)

				// Verify only /deprovision finalizers were removed if object exists
				if c.initObject != nil {
					resource := &unstructured.Unstructured{}
					u := c.initObject.(*unstructured.Unstructured)
					resource.SetGroupVersionKind(u.GroupVersionKind())

					err := fakeClient.Get(ctx, client.ObjectKeyFromObject(c.initObject), resource)
					assert.Nil(t, err)

					finalizers := resource.GetFinalizers()
					assert.Equal(t, c.expectedFinalizers, finalizers)
				}
			}
		})
	}
}

func TestZTPClusterResources(t *testing.T) {
	// Verify that ZTPClusterResources contains expected resources
	// This test validates the number of configured ZTP resources and their properties
	// to ensure all necessary resource types (ClusterDeployment, BareMetalHost, DataImage)
	// are properly configured with their pause annotations and finalizer suffixes
	expectedResources := []ZTPResourceConfig{
		{
			GVK: schema.GroupVersionKind{
				Group:   "hive.openshift.io",
				Version: "v1",
				Kind:    "ClusterDeployment",
			},
			PauseAnnotation: HivePauseAnnotation,
			FinalizerSuffix: "/deprovision",
		},
		{
			GVK: schema.GroupVersionKind{
				Group:   "metal3.io",
				Version: "v1alpha1",
				Kind:    "BareMetalHost",
			},
			PauseAnnotation: Metal3PauseAnnotation,
			FinalizerSuffix: ".metal3.io",
		},
		{
			GVK: schema.GroupVersionKind{
				Group:   "metal3.io",
				Version: "v1alpha1",
				Kind:    "DataImage",
			},
			PauseAnnotation: Metal3PauseAnnotation,
			FinalizerSuffix: ".metal3.io",
		},
	}

	assert.Equal(t, len(expectedResources), len(ZTPClusterResources), "ZTPClusterResources should contain expected number of resources")

	for i, expected := range expectedResources {
		assert.Equal(t, expected.GVK.Group, ZTPClusterResources[i].GVK.Group)
		assert.Equal(t, expected.GVK.Version, ZTPClusterResources[i].GVK.Version)
		assert.Equal(t, expected.GVK.Kind, ZTPClusterResources[i].GVK.Kind)
		assert.Equal(t, expected.PauseAnnotation, ZTPClusterResources[i].PauseAnnotation)
		assert.Equal(t, expected.FinalizerSuffix, ZTPClusterResources[i].FinalizerSuffix)
	}
}

func TestPauseAnnotations(t *testing.T) {
	// Verify the pause annotation constants
	assert.Equal(t, "hive.openshift.io/reconcile-pause", HivePauseAnnotation)
	assert.Equal(t, "baremetalhost.metal3.io/paused", Metal3PauseAnnotation)
}

func TestFilterByLabelKey(t *testing.T) {
	cases := []struct {
		name          string
		items         []unstructured.Unstructured
		labelKey      string
		expectedCount int
		expectedNames []string
		description   string
	}{
		{
			name:     "Filter resources with matching label key",
			labelKey: "test-label",
			items: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata": map[string]interface{}{
							"name":      "cm1",
							"namespace": "default",
							"labels": map[string]interface{}{
								"test-label": "value1",
							},
						},
					},
				},
				{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata": map[string]interface{}{
							"name":      "cm2",
							"namespace": "default",
							"labels": map[string]interface{}{
								"other-label": "value2",
							},
						},
					},
				},
				{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata": map[string]interface{}{
							"name":      "cm3",
							"namespace": "default",
							"labels": map[string]interface{}{
								"test-label": "value3",
							},
						},
					},
				},
			},
			expectedCount: 2,
			expectedNames: []string{"cm1", "cm3"},
			description:   "Should filter only resources with test-label key",
		},
		{
			name:     "Return empty list when no resources match label key",
			labelKey: "non-existent-label",
			items: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata": map[string]interface{}{
							"name":      "cm1",
							"namespace": "default",
							"labels": map[string]interface{}{
								"other-label": "value1",
							},
						},
					},
				},
			},
			expectedCount: 0,
			expectedNames: []string{},
			description:   "Should return empty list when no resources have the label key",
		},
		{
			name:     "Return empty list when resources have no labels",
			labelKey: "test-label",
			items: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata": map[string]interface{}{
							"name":      "cm1",
							"namespace": "default",
						},
					},
				},
			},
			expectedCount: 0,
			expectedNames: []string{},
			description:   "Should return empty list when resources have no labels",
		},
		{
			name:          "Handle empty input list",
			labelKey:      "test-label",
			items:         []unstructured.Unstructured{},
			expectedCount: 0,
			expectedNames: []string{},
			description:   "Should return empty list when input is empty",
		},
		{
			name:     "Filter resources with label key regardless of value",
			labelKey: "preserve",
			items: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "Secret",
						"metadata": map[string]interface{}{
							"name":      "secret1",
							"namespace": "default",
							"labels": map[string]interface{}{
								"preserve": "true",
							},
						},
					},
				},
				{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "Secret",
						"metadata": map[string]interface{}{
							"name":      "secret2",
							"namespace": "default",
							"labels": map[string]interface{}{
								"preserve": "",
							},
						},
					},
				},
				{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "Secret",
						"metadata": map[string]interface{}{
							"name":      "secret3",
							"namespace": "default",
							"labels": map[string]interface{}{
								"other": "value",
							},
						},
					},
				},
			},
			expectedCount: 2,
			expectedNames: []string{"secret1", "secret2"},
			description:   "Should filter based on key presence regardless of value",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			filtered := filterByLabelKey(c.items, c.labelKey)

			assert.Equal(t, c.expectedCount, len(filtered), c.description)

			if c.expectedCount > 0 {
				actualNames := make([]string, len(filtered))
				for i, item := range filtered {
					actualNames[i] = item.GetName()
				}
				assert.ElementsMatch(t, c.expectedNames, actualNames, "Should match expected resource names")
			}
		})
	}
}

func TestHasLabelKey(t *testing.T) {
	cases := []struct {
		name        string
		item        *unstructured.Unstructured
		labelKey    string
		expected    bool
		description string
	}{
		{
			name:     "Resource has the label key",
			labelKey: "test-label",
			item: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "cm1",
						"namespace": "default",
						"labels": map[string]interface{}{
							"test-label": "value",
						},
					},
				},
			},
			expected:    true,
			description: "Should return true when resource has the label key",
		},
		{
			name:     "Resource does not have the label key",
			labelKey: "test-label",
			item: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "cm1",
						"namespace": "default",
						"labels": map[string]interface{}{
							"other-label": "value",
						},
					},
				},
			},
			expected:    false,
			description: "Should return false when resource does not have the label key",
		},
		{
			name:     "Resource has no labels",
			labelKey: "test-label",
			item: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "cm1",
						"namespace": "default",
					},
				},
			},
			expected:    false,
			description: "Should return false when resource has no labels",
		},
		{
			name:     "Resource has label with empty value",
			labelKey: "test-label",
			item: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "cm1",
						"namespace": "default",
						"labels": map[string]interface{}{
							"test-label": "",
						},
					},
				},
			},
			expected:    true,
			description: "Should return true when label key exists even with empty value",
		},
		{
			name:     "Resource labels map is nil",
			labelKey: "test-label",
			item: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "cm1",
						"namespace": "default",
						"labels":    nil,
					},
				},
			},
			expected:    false,
			description: "Should return false when labels map is nil",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := hasLabelKey(c.item, c.labelKey)
			assert.Equal(t, c.expected, result, c.description)
		})
	}
}

func TestListAndFilterResourcesByLabelKey(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()

	cases := []struct {
		name            string
		clusterName     string
		migrateResource MigrationResource
		initObjects     []client.Object
		expectedCount   int
		expectedNames   []string
		description     string
	}{
		{
			name:        "Filter Secrets by label key",
			clusterName: "cluster1",
			migrateResource: MigrationResource{
				gvk: schema.GroupVersionKind{
					Group:   "",
					Version: "v1",
					Kind:    "Secret",
				},
				labelKey: "siteconfig.open-cluster-management.io/preserve",
			},
			initObjects: []client.Object{
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "Secret",
						"metadata": map[string]interface{}{
							"name":      "secret1",
							"namespace": "cluster1",
							"labels": map[string]interface{}{
								"siteconfig.open-cluster-management.io/preserve": "true",
							},
						},
					},
				},
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "Secret",
						"metadata": map[string]interface{}{
							"name":      "secret2",
							"namespace": "cluster1",
							"labels": map[string]interface{}{
								"other-label": "value",
							},
						},
					},
				},
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "Secret",
						"metadata": map[string]interface{}{
							"name":      "secret3",
							"namespace": "cluster1",
							"labels": map[string]interface{}{
								"siteconfig.open-cluster-management.io/preserve": "false",
							},
						},
					},
				},
			},
			expectedCount: 2,
			expectedNames: []string{"secret1", "secret3"},
			description:   "Should filter Secrets by label key presence",
		},
		{
			name:        "Filter ConfigMaps by label key",
			clusterName: "cluster1",
			migrateResource: MigrationResource{
				gvk: schema.GroupVersionKind{
					Group:   "",
					Version: "v1",
					Kind:    "ConfigMap",
				},
				labelKey: "siteconfig.open-cluster-management.io/preserve",
			},
			initObjects: []client.Object{
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata": map[string]interface{}{
							"name":      "cm1",
							"namespace": "cluster1",
							"labels": map[string]interface{}{
								"siteconfig.open-cluster-management.io/preserve": "true",
							},
						},
					},
				},
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata": map[string]interface{}{
							"name":      "cm2",
							"namespace": "cluster1",
						},
					},
				},
			},
			expectedCount: 1,
			expectedNames: []string{"cm1"},
			description:   "Should filter ConfigMaps by label key presence",
		},
		{
			name:        "Return empty when no resources match label key",
			clusterName: "cluster1",
			migrateResource: MigrationResource{
				gvk: schema.GroupVersionKind{
					Group:   "",
					Version: "v1",
					Kind:    "Secret",
				},
				labelKey: "non-existent-label",
			},
			initObjects: []client.Object{
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "Secret",
						"metadata": map[string]interface{}{
							"name":      "secret1",
							"namespace": "cluster1",
							"labels": map[string]interface{}{
								"other-label": "value",
							},
						},
					},
				},
			},
			expectedCount: 0,
			expectedNames: []string{},
			description:   "Should return empty list when no resources have the label key",
		},
		{
			name:        "List all resources when labelKey is not set",
			clusterName: "cluster1",
			migrateResource: MigrationResource{
				gvk: schema.GroupVersionKind{
					Group:   "",
					Version: "v1",
					Kind:    "Secret",
				},
				labelKey: "",
			},
			initObjects: []client.Object{
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "Secret",
						"metadata": map[string]interface{}{
							"name":      "secret1",
							"namespace": "cluster1",
						},
					},
				},
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "Secret",
						"metadata": map[string]interface{}{
							"name":      "secret2",
							"namespace": "cluster1",
						},
					},
				},
			},
			expectedCount: 2,
			expectedNames: []string{"secret1", "secret2"},
			description:   "Should return all resources when labelKey is not set",
		},
		{
			name:        "Filter by both annotation key and label key",
			clusterName: "cluster1",
			migrateResource: MigrationResource{
				gvk: schema.GroupVersionKind{
					Group:   "",
					Version: "v1",
					Kind:    "Secret",
				},
				annotationKey: "test-annotation",
				labelKey:      "test-label",
			},
			initObjects: []client.Object{
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "Secret",
						"metadata": map[string]interface{}{
							"name":      "secret1",
							"namespace": "cluster1",
							"annotations": map[string]interface{}{
								"test-annotation": "value1",
							},
							"labels": map[string]interface{}{
								"test-label": "value1",
							},
						},
					},
				},
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "Secret",
						"metadata": map[string]interface{}{
							"name":      "secret2",
							"namespace": "cluster1",
							"annotations": map[string]interface{}{
								"test-annotation": "value2",
							},
						},
					},
				},
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "Secret",
						"metadata": map[string]interface{}{
							"name":      "secret3",
							"namespace": "cluster1",
							"labels": map[string]interface{}{
								"test-label": "value3",
							},
						},
					},
				},
			},
			expectedCount: 1,
			expectedNames: []string{"secret1"},
			description:   "Should filter by both annotation and label keys (AND operation)",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(c.initObjects...).Build()

			resources, err := listAndFilterResources(ctx, fakeClient, c.clusterName, c.migrateResource)

			assert.Nil(t, err)
			assert.Equal(t, c.expectedCount, len(resources), c.description)

			if c.expectedCount > 0 {
				actualNames := make([]string, len(resources))
				for i, item := range resources {
					actualNames[i] = item.GetName()
				}
				assert.ElementsMatch(t, c.expectedNames, actualNames, "Should match expected resource names")
			}
		})
	}
}
