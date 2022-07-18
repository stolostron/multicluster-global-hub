package applier

import (
	"context"
	"embed"
	"io/fs"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
)

//go:embed manifests
var manifestFS embed.FS

var decUnstructured = yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

func ApplyManifestsForVersion(ctx context.Context, hubVersion string, dyn dynamic.Interface, mapper *restmapper.DeferredDiscoveryRESTMapper, log logr.Logger) error {
	err := fs.WalkDir(manifestFS, "manifests/"+hubVersion, func(file string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		obj := &unstructured.Unstructured{}
		if !d.IsDir() {
			b, err := manifestFS.ReadFile(file)
			if err != nil {
				return err
			}
			_, gvk, err := decUnstructured.Decode(b, nil, obj)
			if err != nil {
				return err
			}
			mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
			if err != nil {
				return err
			}
			var dr dynamic.ResourceInterface
			if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
				// for namespace scoped resources
				dr = dyn.Resource(mapping.Resource).Namespace(obj.GetNamespace())
			} else {
				// for cluster-wide resources
				dr = dyn.Resource(mapping.Resource)
			}

			if err := applyDynamicResource(ctx, dr, obj, log); err != nil {
				return err
			}
		}
		return nil
	})

	return err
}

func applyDynamicResource(ctx context.Context, dr dynamic.ResourceInterface, desired *unstructured.Unstructured, log logr.Logger) error {
	existingObj, err := dr.Get(ctx, desired.GetName(), metav1.GetOptions{})
	if errors.IsNotFound(err) {
		log.Info("creating unstructured object", "name", desired.GetName(), "namespace", desired.GetNamespace())
		_, err := dr.Create(ctx, desired, metav1.CreateOptions{})
		if err != nil {
			return err
		}
		return nil
	}
	if err != nil {
		return err
	}

	toUpdate, modified, err := ensureGenericSpec(existingObj, desired)
	if err != nil {
		return err
	}

	if !modified {
		return nil
	}

	log.Info("desired unstructured object changed, updating it", desired.GetNamespace(), desired.GetName())
	_, err = dr.Update(ctx, toUpdate, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	return nil
}

func ensureGenericSpec(existing, desired *unstructured.Unstructured) (*unstructured.Unstructured, bool, error) {
	desiredCopy := desired.DeepCopy()
	desiredSpec, _, err := unstructured.NestedMap(desiredCopy.UnstructuredContent(), "spec")
	if err != nil {
		return nil, false, err
	}
	existingSpec, _, err := unstructured.NestedMap(existing.UnstructuredContent(), "spec")
	if err != nil {
		return nil, false, err
	}

	if equality.Semantic.DeepEqual(existingSpec, desiredSpec) {
		return existing, false, nil
	}

	existingCopy := existing.DeepCopy()
	if err := unstructured.SetNestedMap(existingCopy.UnstructuredContent(), desiredSpec, "spec"); err != nil {
		return nil, true, err
	}

	return existingCopy, true, nil
}
