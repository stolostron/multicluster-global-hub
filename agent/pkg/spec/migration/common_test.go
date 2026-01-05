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
				PauseAnnotation: "true",
			},
		},
		{
			name:        "Skip adding pause annotation to ImageClusterInstall (not in ZTPClusterResourceGVKs)",
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
				PauseAnnotation:       "true",
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
				PauseAnnotation: "true",
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

				// Verify annotations were added only to resources in ZTPClusterResourceGVKs
				for _, obj := range c.initObjects {
					u := obj.(*unstructured.Unstructured)
					resource := &unstructured.Unstructured{}
					resource.SetGroupVersionKind(u.GroupVersionKind())

					err := fakeClient.Get(ctx, client.ObjectKeyFromObject(u), resource)
					assert.Nil(t, err)

					annotations := resource.GetAnnotations()
					// Only check expected annotations for resources in ZTPClusterResourceGVKs
					isInZTPList := false
					for _, gvk := range ZTPClusterResourceGVKs {
						if gvk == u.GroupVersionKind() {
							isInZTPList = true
							break
						}
					}

					if isInZTPList && c.expectedValue != nil {
						for key, expectedVal := range c.expectedValue {
							assert.Equal(t, expectedVal, annotations[key])
						}
					} else if !isInZTPList && c.expectedValue != nil {
						// For resources not in ZTPClusterResourceGVKs, annotation should not be added
						if annotations != nil {
							_, hasKey := annotations[PauseAnnotation]
							assert.False(t, hasKey, "Annotation should not be added to %s", u.GetKind())
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
								PauseAnnotation: "true",
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
			name:        "Skip removal of pause annotation from ImageClusterInstall (not in ZTPClusterResourceGVKs)",
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
								PauseAnnotation: "true",
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
								PauseAnnotation:      "true",
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
								PauseAnnotation: "true",
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
								PauseAnnotation: "true",
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
			shouldHaveKey: false, // Will be checked per-resource based on ZTPClusterResourceGVKs
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

				// Verify annotations were removed only from resources in ZTPClusterResourceGVKs
				for _, obj := range c.initObjects {
					u := obj.(*unstructured.Unstructured)
					resource := &unstructured.Unstructured{}
					resource.SetGroupVersionKind(u.GroupVersionKind())

					err := fakeClient.Get(ctx, client.ObjectKeyFromObject(u), resource)
					assert.Nil(t, err)

					// Check if resource is in ZTPClusterResourceGVKs
					isInZTPList := false
					for _, gvk := range ZTPClusterResourceGVKs {
						if gvk == u.GroupVersionKind() {
							isInZTPList = true
							break
						}
					}

					annotations := resource.GetAnnotations()
					if annotations != nil {
						_, hasKey := annotations[PauseAnnotation]
						if isInZTPList {
							// For resources in ZTPClusterResourceGVKs, annotation should be removed
							assert.Equal(t, c.shouldHaveKey, hasKey)
						} else {
							// For resources not in ZTPClusterResourceGVKs, annotation should remain
							// Check original object to see if it had the annotation
							origAnnotations := obj.(*unstructured.Unstructured).GetAnnotations()
							if origAnnotations != nil {
								_, origHasKey := origAnnotations[PauseAnnotation]
								assert.Equal(t, origHasKey, hasKey, "Annotation state should not change for %s", u.GetKind())
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
			name:        "Skip removal of /deprovision finalizers from ImageClusterInstall (not in ZTPClusterResourceGVKs)",
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
			expectedFinalizers: nil, // Will be checked per-resource based on ZTPClusterResourceGVKs
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

				// Verify only /deprovision finalizers were removed from resources in ZTPClusterResourceGVKs
				for _, obj := range c.initObjects {
					u := obj.(*unstructured.Unstructured)
					resource := &unstructured.Unstructured{}
					resource.SetGroupVersionKind(u.GroupVersionKind())

					err := fakeClient.Get(ctx, client.ObjectKeyFromObject(u), resource)
					assert.Nil(t, err)

					// Check if resource is in ZTPClusterResourceGVKs
					isInZTPList := false
					for _, gvk := range ZTPClusterResourceGVKs {
						if gvk == u.GroupVersionKind() {
							isInZTPList = true
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
						// For test cases with multiple resources, check based on ZTPClusterResourceGVKs
						if isInZTPList {
							// For resources in ZTPClusterResourceGVKs, /deprovision finalizers should be removed
							for _, f := range finalizers {
								assert.NotContains(t, f, "/deprovision", "Resource in ZTPClusterResourceGVKs should not have /deprovision finalizer")
							}
						} else {
							// For resources not in ZTPClusterResourceGVKs, finalizers should remain unchanged
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

func TestZTPClusterResourceGVKs(t *testing.T) {
	// Verify that ZTPClusterResourceGVKs contains expected GVKs
	// Note: ImageClusterInstall was removed from this list
	expectedGVKs := []schema.GroupVersionKind{
		{
			Group:   "hive.openshift.io",
			Version: "v1",
			Kind:    "ClusterDeployment",
		},
	}

	assert.Equal(t, len(expectedGVKs), len(ZTPClusterResourceGVKs), "ZTPClusterResourceGVKs should contain expected number of GVKs")

	for i, expected := range expectedGVKs {
		assert.Equal(t, expected.Group, ZTPClusterResourceGVKs[i].Group)
		assert.Equal(t, expected.Version, ZTPClusterResourceGVKs[i].Version)
		assert.Equal(t, expected.Kind, ZTPClusterResourceGVKs[i].Kind)
	}
}

func TestPauseAnnotation(t *testing.T) {
	// Verify the pause annotation constant
	assert.Equal(t, "hive.openshift.io/reconcile-pause", PauseAnnotation)
}
