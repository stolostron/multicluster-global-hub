package syncers

import (
	"context"
	"encoding/json"
	"sync"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/rbac"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/workers"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/spec"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// genericBundleSyncer syncs objects spec from received bundles.
type genericBundleSyncer struct {
	log                          *zap.SugaredLogger
	workerPool                   *workers.WorkerPool
	bundleProcessingWaitingGroup sync.WaitGroup
	enforceHohRbac               bool
}

func NewGenericSyncer(workerPool *workers.WorkerPool, config *configs.AgentConfig) *genericBundleSyncer {
	return &genericBundleSyncer{
		log:                          logger.DefaultZapLogger(),
		workerPool:                   workerPool,
		bundleProcessingWaitingGroup: sync.WaitGroup{},
		enforceHohRbac:               config.SpecEnforceHohRbac,
	}
}

func (syncer *genericBundleSyncer) Sync(ctx context.Context, evt *cloudevents.Event) error {
	payload := evt.Data()
	genericBundle := &spec.GenericSpecBundle{}
	if err := json.Unmarshal(payload, genericBundle); err != nil {
		return err
	}

	syncer.bundleProcessingWaitingGroup.Add(len(genericBundle.Objects) + len(genericBundle.DeletedObjects))
	syncer.syncObjects(genericBundle.Objects)
	syncer.syncDeletedObjects(genericBundle.DeletedObjects)
	syncer.bundleProcessingWaitingGroup.Wait()
	return nil
}

func (s *genericBundleSyncer) syncObjects(bundleObjects []*unstructured.Unstructured) {
	for _, bundleObject := range bundleObjects {
		if !s.enforceHohRbac { // if rbac not enforced, use controller's identity.
			bundleObject = s.anonymize(bundleObject) // anonymize removes the user identity from the obj if exists
		}

		s.workerPool.Submit(workers.NewJob(bundleObject, func(ctx context.Context,
			k8sClient client.Client, obj interface{},
		) {
			defer s.bundleProcessingWaitingGroup.Done()

			unstructuredObject, _ := obj.(*unstructured.Unstructured)

			if !s.enforceHohRbac { // if rbac not enforced, create missing namespaces.
				if err := utils.CreateNamespaceIfNotExist(ctx, k8sClient,
					unstructuredObject.GetNamespace()); err != nil {
					s.log.Error(err, "failed to create namespace", unstructuredObject.GetNamespace())
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
			err := utils.UpdateObject(ctx, k8sClient, unstructuredObject)
			if err != nil {
				s.log.Error(err, "failed to update object", "name", unstructuredObject.GetName(),
					"namespace", unstructuredObject.GetNamespace(), "kind", unstructuredObject.GetKind())
				return
			}
			s.log.Debug("object updated", "name", unstructuredObject.GetName(), "namespace",
				unstructuredObject.GetNamespace(), "kind", unstructuredObject.GetKind())
		}))
	}
}

func (s *genericBundleSyncer) syncDeletedObjects(deletedObjects []*unstructured.Unstructured) {
	for _, deletedBundleObj := range deletedObjects {
		if !s.enforceHohRbac { // if rbac not enforced, use controller's identity.
			deletedBundleObj = s.anonymize(deletedBundleObj) // anonymize removes the user identity from the obj if exists
		}

		s.workerPool.Submit(workers.NewJob(deletedBundleObj, func(ctx context.Context,
			k8sClient client.Client, obj interface{},
		) {
			defer s.bundleProcessingWaitingGroup.Done()

			unstructuredObject, _ := obj.(*unstructured.Unstructured)

			// syncer.deleteObject(ctx, k8sClient, obj.(*unstructured.Unstructured))
			if deleted, err := utils.DeleteObject(ctx, k8sClient, unstructuredObject); err != nil {
				s.log.Error("failed to delete object",
					"error", err,
					"name", unstructuredObject.GetName(),
					"namespace", unstructuredObject.GetNamespace(),
					"kind", unstructuredObject.GetKind())
			} else if deleted {
				s.log.Infow("object deleted", "name", unstructuredObject.GetName(),
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
