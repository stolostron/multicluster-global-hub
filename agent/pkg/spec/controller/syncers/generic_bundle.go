package syncers

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/hub-of-hubs/agent/pkg/helper"
	"github.com/stolostron/hub-of-hubs/agent/pkg/spec/bundle"
	"github.com/stolostron/hub-of-hubs/agent/pkg/spec/controller/rbac"
	"github.com/stolostron/hub-of-hubs/agent/pkg/spec/controller/workers"
	consumer "github.com/stolostron/hub-of-hubs/agent/pkg/transport/consumer"
)

// genericBundleSyncer syncs objects spec from received bundles.
type genericBundleSyncer struct {
	log                          logr.Logger
	genericBundleChan            chan *bundle.GenericBundle
	workerPool                   *workers.WorkerPool
	bundleProcessingWaitingGroup sync.WaitGroup
	enforceHohRbac               bool
}

// AddGenericBundleSyncer adds genericBundleSyncer to the manager.
func AddGenericBundleSyncer(log logr.Logger, mgr ctrl.Manager, enforceHohRbac bool,
	consumer consumer.Consumer, workerPool *workers.WorkerPool,
) error {
	if err := mgr.Add(&genericBundleSyncer{
		log:                          log,
		genericBundleChan:            consumer.GetGenericBundleChan(),
		workerPool:                   workerPool,
		bundleProcessingWaitingGroup: sync.WaitGroup{},
		enforceHohRbac:               enforceHohRbac,
	}); err != nil {
		return fmt.Errorf("failed to add generic bundles spec syncer - %w", err)
	}

	return nil
}

// Start function starts bundles spec syncer.
func (syncer *genericBundleSyncer) Start(ctx context.Context) error {
	syncer.log.Info("started bundles syncer...")

	go syncer.sync(ctx)

	<-ctx.Done() // blocking wait for stop event
	syncer.log.Info("stopped bundles syncer")

	return nil
}

func (syncer *genericBundleSyncer) sync(ctx context.Context) {
	for {
		select {
		case <-ctx.Done(): // we have received a signal to stop
			return

		case receivedBundle := <-syncer.genericBundleChan: // handle the bundle
			syncer.bundleProcessingWaitingGroup.Add(len(receivedBundle.Objects) + len(receivedBundle.DeletedObjects))
			syncer.syncObjects(receivedBundle.Objects)
			syncer.syncDeletedObjects(receivedBundle.DeletedObjects)
			syncer.bundleProcessingWaitingGroup.Wait()
		}
	}
}

func (syncer *genericBundleSyncer) syncObjects(bundleObjects []*unstructured.Unstructured) {
	for _, bundleObject := range bundleObjects {
		if !syncer.enforceHohRbac { // if rbac not enforced, use controller's identity.
			bundleObject = syncer.anonymize(bundleObject) // anonymize removes the user identity from the obj if exists
		}

		syncer.workerPool.Submit(workers.NewJob(bundleObject, func(ctx context.Context,
			k8sClient client.Client, obj interface{}) {
			defer syncer.bundleProcessingWaitingGroup.Done()

			unstructuredObject, _ := obj.(*unstructured.Unstructured)

			if !syncer.enforceHohRbac { // if rbac not enforced, create missing namespaces.
				if err := helper.CreateNamespaceIfNotExist(ctx, k8sClient, unstructuredObject.GetNamespace()); err != nil {
					syncer.log.Error(err, "failed to create namespace", "namespace", unstructuredObject.GetNamespace())
					return
				}
			}

			err := helper.UpdateObject(ctx, k8sClient, unstructuredObject)
			if err != nil {
				syncer.log.Error(err, "failed to update object", "name", unstructuredObject.GetName(),
					"namespace", unstructuredObject.GetNamespace(), "kind", unstructuredObject.GetKind())
				return
			}
			syncer.log.Info("object updated", "name", unstructuredObject.GetName(), "namespace",
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
			k8sClient client.Client, obj interface{}) {
			defer syncer.bundleProcessingWaitingGroup.Done()

			unstructuredObject, _ := obj.(*unstructured.Unstructured)

			// syncer.deleteObject(ctx, k8sClient, obj.(*unstructured.Unstructured))
			if deleted, err := helper.DeleteObject(ctx, k8sClient, unstructuredObject); err != nil {
				syncer.log.Error(err, "failed to delete object", "name", unstructuredObject.GetName(), "namespace",
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
