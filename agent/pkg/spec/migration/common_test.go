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
			name:        "Successfully add pause annotation to ImageClusterInstall",
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
			expectedValue: map[string]string{
				PauseAnnotation: "true",
			},
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
			name:        "Add pause annotation to both ClusterDeployment and ImageClusterInstall",
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

				// Verify annotations were added if objects exist
				for _, obj := range c.initObjects {
					u := obj.(*unstructured.Unstructured)
					resource := &unstructured.Unstructured{}
					resource.SetGroupVersionKind(u.GroupVersionKind())

					err := fakeClient.Get(ctx, client.ObjectKeyFromObject(u), resource)
					assert.Nil(t, err)

					annotations := resource.GetAnnotations()
					if c.expectedValue != nil {
						for key, expectedVal := range c.expectedValue {
							assert.Equal(t, expectedVal, annotations[key])
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
			name:        "Successfully remove pause annotation from ImageClusterInstall",
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
			shouldHaveKey: false,
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
			name:        "Remove pause annotation from both resources",
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
			shouldHaveKey: false,
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

				// Verify annotations were removed if objects exist
				for _, obj := range c.initObjects {
					u := obj.(*unstructured.Unstructured)
					resource := &unstructured.Unstructured{}
					resource.SetGroupVersionKind(u.GroupVersionKind())

					err := fakeClient.Get(ctx, client.ObjectKeyFromObject(u), resource)
					assert.Nil(t, err)

					annotations := resource.GetAnnotations()
					if annotations != nil {
						_, hasKey := annotations[PauseAnnotation]
						assert.Equal(t, c.shouldHaveKey, hasKey)
					}
				}
			}
		})
	}
}

func TestRemovePauseResourcesFinalizers(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()

	cases := []struct {
		name                string
		clusterName         string
		initObjects         []client.Object
		expectedError       bool
		shouldHaveFinalizer bool
	}{
		{
			name:        "Successfully remove finalizers from ClusterDeployment",
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
			expectedError:       false,
			shouldHaveFinalizer: false,
		},
		{
			name:        "Successfully remove finalizers from ImageClusterInstall",
			clusterName: "cluster1",
			initObjects: []client.Object{
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "extensions.hive.openshift.io/v1alpha1",
						"kind":       "ImageClusterInstall",
						"metadata": map[string]interface{}{
							"name":       "cluster1",
							"namespace":  "cluster1",
							"finalizers": []interface{}{"finalizer1"},
						},
						"spec": map[string]interface{}{
							"clusterDeploymentRef": map[string]interface{}{
								"name": "cluster1",
							},
						},
					},
				},
			},
			expectedError:       false,
			shouldHaveFinalizer: false,
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
			name:        "Remove finalizers from both resources",
			clusterName: "cluster1",
			initObjects: []client.Object{
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "hive.openshift.io/v1",
						"kind":       "ClusterDeployment",
						"metadata": map[string]interface{}{
							"name":       "cluster1",
							"namespace":  "cluster1",
							"finalizers": []interface{}{"finalizer1"},
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
							"finalizers": []interface{}{"finalizer2", "finalizer3"},
						},
						"spec": map[string]interface{}{
							"clusterDeploymentRef": map[string]interface{}{
								"name": "cluster1",
							},
						},
					},
				},
			},
			expectedError:       false,
			shouldHaveFinalizer: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(c.initObjects...).Build()

			err := RemovePauseResourcesFinalizers(ctx, fakeClient, c.clusterName)

			if c.expectedError {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)

				// Verify finalizers were removed if objects exist
				for _, obj := range c.initObjects {
					u := obj.(*unstructured.Unstructured)
					resource := &unstructured.Unstructured{}
					resource.SetGroupVersionKind(u.GroupVersionKind())

					err := fakeClient.Get(ctx, client.ObjectKeyFromObject(u), resource)
					assert.Nil(t, err)

					finalizers := resource.GetFinalizers()
					if c.shouldHaveFinalizer {
						assert.NotEmpty(t, finalizers)
					} else {
						assert.Empty(t, finalizers)
					}
				}
			}
		})
	}
}

func TestRemoveFinalizers(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()

	cases := []struct {
		name                string
		initObject          client.Object
		expectedError       bool
		shouldHaveFinalizer bool
	}{
		{
			name: "Successfully remove finalizers from resource",
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
			expectedError:       false,
			shouldHaveFinalizer: false,
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
			expectedError: false,
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

			err := removeFinalizers(ctx, fakeClient, c.initObject)

			if c.expectedError {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)

				// Verify finalizers were removed if object exists
				if c.initObject != nil {
					resource := &unstructured.Unstructured{}
					u := c.initObject.(*unstructured.Unstructured)
					resource.SetGroupVersionKind(u.GroupVersionKind())

					err := fakeClient.Get(ctx, client.ObjectKeyFromObject(c.initObject), resource)
					assert.Nil(t, err)

					finalizers := resource.GetFinalizers()
					if c.shouldHaveFinalizer {
						assert.NotEmpty(t, finalizers)
					} else {
						assert.Empty(t, finalizers)
					}
				}
			}
		})
	}
}

func TestPauseResources(t *testing.T) {
	// Verify that PauseResources contains expected GVKs
	expectedGVKs := []schema.GroupVersionKind{
		{
			Group:   "hive.openshift.io",
			Version: "v1",
			Kind:    "ClusterDeployment",
		},
		{
			Group:   "extensions.hive.openshift.io",
			Version: "v1alpha1",
			Kind:    "ImageClusterInstall",
		},
	}

	assert.Equal(t, len(expectedGVKs), len(PauseResources), "PauseResources should contain expected number of GVKs")

	for i, expected := range expectedGVKs {
		assert.Equal(t, expected.Group, PauseResources[i].Group)
		assert.Equal(t, expected.Version, PauseResources[i].Version)
		assert.Equal(t, expected.Kind, PauseResources[i].Kind)
	}
}

func TestPauseAnnotation(t *testing.T) {
	// Verify the pause annotation constant
	assert.Equal(t, "hive.openshift.io/reconcile-pause", PauseAnnotation)
}
