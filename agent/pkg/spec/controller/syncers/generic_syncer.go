package syncers

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/controller/rbac"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/controller/workers"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/base"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
	helper "github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// genericBundleSyncer syncs objects spec from received bundles.
type genericBundleSyncer struct {
	log                          logr.Logger
	workerPool                   *workers.WorkerPool
	bundleProcessingWaitingGroup sync.WaitGroup
	enforceHohRbac               bool
}

func NewGenericSyncer(workerPool *workers.WorkerPool, config *config.AgentConfig) *genericBundleSyncer {
	return &genericBundleSyncer{
		log:                          ctrl.Log.WithName("generic-bundle-syncer"),
		workerPool:                   workerPool,
		bundleProcessingWaitingGroup: sync.WaitGroup{},
		enforceHohRbac:               config.SpecEnforceHohRbac,
	}
}

func (syncer *genericBundleSyncer) Sync(payload []byte) error {
	genericBundle := &base.SpecGenericBundle{}
	if err := json.Unmarshal(payload, genericBundle); err != nil {
		return err
	}

	syncer.bundleProcessingWaitingGroup.Add(len(genericBundle.Objects) + len(genericBundle.DeletedObjects))
	syncer.syncObjects(genericBundle.Objects)
	syncer.syncDeletedObjects(genericBundle.DeletedObjects)
	syncer.bundleProcessingWaitingGroup.Wait()
	return nil
}

func (syncer *genericBundleSyncer) syncObjects(bundleObjects []*unstructured.Unstructured) {
	for _, bundleObject := range bundleObjects {
		if !syncer.enforceHohRbac { // if rbac not enforced, use controller's identity.
			bundleObject = syncer.anonymize(bundleObject) // anonymize removes the user identity from the obj if exists
		}

		syncer.workerPool.Submit(workers.NewJob(bundleObject, func(ctx context.Context,
			k8sClient client.Client, obj interface{},
		) {
			defer syncer.bundleProcessingWaitingGroup.Done()

			unstructuredObject, _ := obj.(*unstructured.Unstructured)

			if !syncer.enforceHohRbac { // if rbac not enforced, create missing namespaces.
				if err := utils.CreateNamespaceIfNotExist(ctx, k8sClient,
					unstructuredObject.GetNamespace()); err != nil {
					syncer.log.Error(err, "failed to create namespace",
						"namespace", unstructuredObject.GetNamespace())
					return
				}
			}

			// Deprecated: skip the "bindingOverrides" from the placementbinding
			// Reference: https://github.com/open-cluster-management-io/governance-policy-propagator/pull/110
			delete(unstructuredObject.Object, "bindingOverrides")

			// Deprecated: skip the "spec.decisionStrategy" and "spec.spreadPolicy" from the placement
			// Reference:
			//   "spec.spreadPolicy": https://github.com/open-cluster-management-io/api/pull/225
			//   "spec.decisionStrategy": https://github.com/open-cluster-management-io/api/pull/242
			spec, ok := unstructuredObject.Object["spec"]
			if ok {
				specMap := spec.(map[string]interface{})
				delete(specMap, "decisionStrategy")
				delete(specMap, "spreadPolicy")
				unstructuredObject.Object["spec"] = specMap
			}

			delete(unstructuredObject.Object, "status")
			err := helper.UpdateObject(ctx, k8sClient, unstructuredObject)
			if err != nil {
				syncer.log.Error(err, "failed to update object", "name", unstructuredObject.GetName(),
					"namespace", unstructuredObject.GetNamespace(), "kind", unstructuredObject.GetKind())
				return
			}
			syncer.log.V(2).Info("object updated", "name", unstructuredObject.GetName(), "namespace",
				unstructuredObject.GetNamespace(), "kind", unstructuredObject.GetKind())
		}))
	}
}

func (syncer *genericBundleSyncer) syncDeletedObjects(deletedObjects []*unstructured.Unstructured) {
	for _, deletedBundleObj := range deletedObjects {
		if !syncer.enforceHohRbac { // if rbac not enforced, use controller's identity.
			deletedBundleObj = syncer.anonymize(deletedBundleObj) // anonymize removes the user identity from the obj if exists
		}

		syncer.workerPool.Submit(workers.NewJob(deletedBundleObj, func(ctx context.Context,
			k8sClient client.Client, obj interface{},
		) {
			defer syncer.bundleProcessingWaitingGroup.Done()

			unstructuredObject, _ := obj.(*unstructured.Unstructured)

			// syncer.deleteObject(ctx, k8sClient, obj.(*unstructured.Unstructured))
			if deleted, err := helper.DeleteObject(ctx, k8sClient, unstructuredObject); err != nil {
				syncer.log.Error(err, "failed to delete object", "name",
					unstructuredObject.GetName(), "namespace",
					unstructuredObject.GetNamespace(), "kind", unstructuredObject.GetKind())
			} else if deleted {
				syncer.log.Info("object deleted", "name", unstructuredObject.GetName(),
					"namespace", unstructuredObject.GetNamespace(), "kind", unstructuredObject.GetKind())
			}
		}))
	}
}

func (syncer *genericBundleSyncer) anonymize(obj *unstructured.Unstructured) *unstructured.Unstructured {
	annotations := obj.GetAnnotations()
	delete(annotations, rbac.UserIdentityAnnotation)
	delete(annotations, rbac.UserGroupsAnnotation)
	obj.SetAnnotations(annotations)
	return obj
}
