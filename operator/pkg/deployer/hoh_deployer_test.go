package deployer_test

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	yamlserializer "k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/types"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/deployer"
)

//go:embed testdata
var manifestsFS embed.FS
var decoder = yamlserializer.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

func loadObjects(manifestsFS embed.FS, dir string) ([]*unstructured.Unstructured, error) {
	var unstructuredObjs []*unstructured.Unstructured
	var files []string
	err := fs.WalkDir(manifestsFS, dir, func(file string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		files = append(files, file)
		return nil
	})
	if err != nil {
		return unstructuredObjs, err
	}

	if len(files) == 0 {
		return unstructuredObjs, fmt.Errorf("no manifests found")
	}

	for _, file := range files {
		content, err := manifestsFS.ReadFile(file)
		if err != nil {
			return unstructuredObjs, err
		}

		if len(content) == 0 {
			continue
		}

		// decode YAML manifest into unstructured.Unstructured
		unstrObj := &unstructured.Unstructured{}
		_, _, err = decoder.Decode(content, nil, unstrObj)
		if err != nil {
			return unstructuredObjs, err
		}

		unstructuredObjs = append(unstructuredObjs, unstrObj)
	}

	return unstructuredObjs, nil
}

var _ = Describe("Deployer", Ordered, func() {
	var (
		hohdeployer    deployer.Deployer
		ctx            context.Context
		cancel         context.CancelFunc
		typedObjects   []*unstructured.Unstructured
		untypedObjects []*unstructured.Unstructured
	)

	BeforeAll(func() {
		hohdeployer = deployer.NewHoHDeployer(k8sClient)
		Expect(hohdeployer).NotTo(BeNil())
		ctx, cancel = context.WithCancel(context.TODO())
	})

	AfterAll(func() {
		cancel()
	})

	BeforeEach(func() {
		By("load typed objects")
		typedObjects, err := loadObjects(manifestsFS, "testdata/typedv1")
		Expect(err).NotTo(HaveOccurred())

		By("create typed objects")
		for _, obj := range typedObjects {
			Expect(hohdeployer.Deploy(obj)).NotTo(HaveOccurred())
		}

		By("check typed objects are created")
		for _, obj := range typedObjects {
			unsObj := &unstructured.Unstructured{}
			unsObj.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
			err := k8sClient.Get(ctx, types.NamespacedName{
				Namespace: obj.GetNamespace(),
				Name:      obj.GetName(),
			}, unsObj)
			Expect(err).NotTo(HaveOccurred())
		}

		By("load untyped objects")
		untypedObjects, err := loadObjects(manifestsFS, "testdata/untypedv1")
		Expect(err).NotTo(HaveOccurred())

		By("create untyped objects")
		for _, obj := range untypedObjects {
			Expect(hohdeployer.Deploy(obj)).NotTo(HaveOccurred())
		}

		By("check untyped objects are created")
		for _, obj := range untypedObjects {
			unsObj := &unstructured.Unstructured{}
			unsObj.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
			err := k8sClient.Get(ctx, types.NamespacedName{
				Namespace: obj.GetNamespace(),
				Name:      obj.GetName(),
			}, unsObj)
			Expect(err).NotTo(HaveOccurred())
		}
	})

	AfterEach(func() {
		By("delete typed objects")
		for _, obj := range typedObjects {
			_ = k8sClient.Delete(ctx, obj)
			Expect(k8sClient.Delete(ctx, obj)).NotTo(HaveOccurred())
		}

		By("check typed objects are deleted")
		for _, obj := range typedObjects {
			unsObj := &unstructured.Unstructured{}
			unsObj.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
			err := k8sClient.Get(ctx, types.NamespacedName{
				Namespace: obj.GetNamespace(),
				Name:      obj.GetName(),
			}, unsObj)
			Expect(err).To(HaveOccurred())
			Expect(errors.IsNotFound(err)).Should(BeTrue())
		}

		By("delete untyped objects")
		for _, obj := range untypedObjects {
			_ = k8sClient.Delete(ctx, obj)
			Expect(k8sClient.Delete(ctx, obj)).NotTo(HaveOccurred())
		}

		By("check untyped objects are deleted")
		for _, obj := range untypedObjects {
			unsObj := &unstructured.Unstructured{}
			unsObj.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
			err := k8sClient.Get(ctx, types.NamespacedName{
				Namespace: obj.GetNamespace(),
				Name:      obj.GetName(),
			}, unsObj)
			Expect(err).To(HaveOccurred())
			Expect(errors.IsNotFound(err)).Should(BeTrue())
		}
	})

	Context("with typed objects", func() {
		It("should be updated when new labels are added", func() {
			By("load typed objects")
			typedObjects, err := loadObjects(manifestsFS, "testdata/typedv1")
			Expect(err).NotTo(HaveOccurred())

			By("update objects by adding new label")
			for _, obj := range typedObjects {
				labels := obj.GetLabels()
				if len(labels) == 0 {
					labels = map[string]string{}
				}
				labels["foo"] = "bar"
				obj.SetLabels(labels)
				Expect(hohdeployer.Deploy(obj)).NotTo(HaveOccurred())
			}

			By("check the labels of objects with deploy function are updated")
			for _, obj := range typedObjects {
				unsObj := &unstructured.Unstructured{}
				unsObj.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
				err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: obj.GetNamespace(),
					Name:      obj.GetName(),
				}, unsObj)
				Expect(err).NotTo(HaveOccurred())
				Expect(unsObj.GetLabels()).Should(HaveKeyWithValue("foo", "bar"))
			}
		})
	})

	Context("with typed objects", func() {
		It("should be updated when new annotations are added", func() {
			By("load typed objects")
			typedObjects, err := loadObjects(manifestsFS, "testdata/typedv1")
			Expect(err).NotTo(HaveOccurred())

			By("update objects by adding new annotation")
			for _, obj := range typedObjects {
				annotations := obj.GetAnnotations()
				if len(annotations) == 0 {
					annotations = map[string]string{}
				}
				annotations["foo"] = "bar"
				obj.SetAnnotations(annotations)
				Expect(hohdeployer.Deploy(obj)).NotTo(HaveOccurred())
			}

			By("check the annotations of objects with deploy function are updated")
			for _, obj := range typedObjects {
				unsObj := &unstructured.Unstructured{}
				unsObj.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
				err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: obj.GetNamespace(),
					Name:      obj.GetName(),
				}, unsObj)
				Expect(err).NotTo(HaveOccurred())
				Expect(unsObj.GetAnnotations()).Should(HaveKeyWithValue("foo", "bar"))
			}
		})
	})

	Context("with typed objects", func() {
		It("should be updated when new v2 are deployed", func() {
			By("load typed objects of new version")
			typedObjects, err := loadObjects(manifestsFS, "testdata/typedv2")
			Expect(err).NotTo(HaveOccurred())

			By("update objects content from new version")
			for _, obj := range typedObjects {
				annotations := obj.GetAnnotations()
				if len(annotations) == 0 {
					annotations = map[string]string{}
				}
				annotations["foo"] = "bar"
				obj.SetAnnotations(annotations)
				Expect(hohdeployer.Deploy(obj)).NotTo(HaveOccurred())
			}

			By("check objects from new version are updated")
			for _, obj := range typedObjects {
				// if resource has annotation skip-creation-if-exist: true, then it will not be updated
				metadata, ok := obj.Object["metadata"].(map[string]interface{})
				if ok {
					annotations, ok := metadata["annotations"].(map[string]interface{})
					if ok && annotations != nil && annotations["skip-creation-if-exist"] != nil {
						if strings.ToLower(annotations["skip-creation-if-exist"].(string)) == "true" {
							continue
						}
					}
				}
				unsObj := &unstructured.Unstructured{}
				unsObj.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
				err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: obj.GetNamespace(),
					Name:      obj.GetName(),
				}, unsObj)
				Expect(err).NotTo(HaveOccurred())
				if obj.GetObjectKind().GroupVersionKind().Kind != "Secret" {
					Expect(apiequality.Semantic.DeepDerivative(obj, unsObj)).Should(BeTrue())
				}
			}
		})
	})

	Context("with untyped objects", func() {
		It("should be updated when new labels are added", func() {
			By("load untyped objects")
			untypedObjects, err := loadObjects(manifestsFS, "testdata/untypedv1")
			Expect(err).NotTo(HaveOccurred())

			By("update objects by adding new label")
			for _, obj := range untypedObjects {
				labels := obj.GetLabels()
				if len(labels) == 0 {
					labels = map[string]string{}
				}
				labels["foo"] = "bar"
				obj.SetLabels(labels)
				Expect(hohdeployer.Deploy(obj)).NotTo(HaveOccurred())
			}

			By("check the labels of objects with deploy function are updated")
			for _, obj := range untypedObjects {
				unsObj := &unstructured.Unstructured{}
				unsObj.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
				err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: obj.GetNamespace(),
					Name:      obj.GetName(),
				}, unsObj)
				Expect(err).NotTo(HaveOccurred())
				Expect(unsObj.GetLabels()).Should(HaveKeyWithValue("foo", "bar"))
			}
		})
	})

	Context("with untyped objects", func() {
		It("should be updated when new annotations are added", func() {
			By("load untyped objects")
			untypedObjects, err := loadObjects(manifestsFS, "testdata/untypedv1")
			Expect(err).NotTo(HaveOccurred())

			By("update objects by adding new annotation")
			for _, obj := range untypedObjects {
				annotations := obj.GetAnnotations()
				if len(annotations) == 0 {
					annotations = map[string]string{}
				}
				annotations["foo"] = "bar"
				obj.SetAnnotations(annotations)
				Expect(hohdeployer.Deploy(obj)).NotTo(HaveOccurred())
			}

			By("check the annotations of objects with deploy function are updated")
			for _, obj := range untypedObjects {
				unsObj := &unstructured.Unstructured{}
				unsObj.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
				err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: obj.GetNamespace(),
					Name:      obj.GetName(),
				}, unsObj)
				Expect(err).NotTo(HaveOccurred())
				Expect(unsObj.GetAnnotations()).Should(HaveKeyWithValue("foo", "bar"))
			}
		})
	})

	Context("with untyped objects", func() {
		It("should be updated when new v2 are deployed", func() {
			By("load untyped objects of new version")
			untypedObjects, err := loadObjects(manifestsFS, "testdata/untypedv2")
			Expect(err).NotTo(HaveOccurred())

			By("update objects content from new version")
			for _, obj := range untypedObjects {
				annotations := obj.GetAnnotations()
				if len(annotations) == 0 {
					annotations = map[string]string{}
				}
				annotations["foo"] = "bar"
				obj.SetAnnotations(annotations)
				Expect(hohdeployer.Deploy(obj)).NotTo(HaveOccurred())
			}

			By("check objects from new version are updated")
			for _, obj := range untypedObjects {
				// if resource has annotation skip-creation-if-exist: true, then it will not be updated
				metadata, ok := obj.Object["metadata"].(map[string]interface{})
				if ok {
					annotations, ok := metadata["annotations"].(map[string]interface{})
					if ok && annotations != nil && annotations["skip-creation-if-exist"] != nil {
						if strings.ToLower(annotations["skip-creation-if-exist"].(string)) == "true" {
							continue
						}
					}
				}
				unsObj := &unstructured.Unstructured{}
				unsObj.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
				err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: obj.GetNamespace(),
					Name:      obj.GetName(),
				}, unsObj)
				Expect(err).NotTo(HaveOccurred())
				Expect(apiequality.Semantic.DeepDerivative(obj, unsObj)).Should(BeTrue())
			}
		})
	})
})
